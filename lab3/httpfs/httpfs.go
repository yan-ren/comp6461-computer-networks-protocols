package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"lab3/lib"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	BufferSize           = 2048
	CRLF                 = "\r\n"
	HttpRequestHeaderEnd = "\r\n\r\n"
	ReadTimeoutDuration  = 5 * time.Second
	Host                 = "127.0.0.1"
	WindowSize           = 1024
	DefaultTimeOut       = 1 * time.Second
	Router               = "127.0.0.1:3000"
)

var workingDirectory string
var err error
var handshakeSynChannel = make(chan *lib.Packet, 1)
var handshakeAckChannel = make(chan *lib.Packet, 1)

type HttpServer struct {
	seqNum     uint32
	connect    bool
	serverConn *net.UDPConn
	routerAddr *net.UDPAddr
	clientAddr *net.UDPAddr

	reciverWindow []*lib.Packet
	senderWindow  []*lib.WindowElement

	deliveryMessage chan bool
	logger          *log.Logger
	mutex           *sync.Mutex
}

func initHttpServer() *HttpServer {
	httpServer := &HttpServer{reciverWindow: make([]*lib.Packet, WindowSize), senderWindow: make([]*lib.WindowElement, WindowSize), connect: false, mutex: &sync.Mutex{}}
	httpServer.deliveryMessage = make(chan bool, 1)
	logf, err := os.OpenFile("server.log."+fmt.Sprint(time.Now().Unix()), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v", err)
		panic(err)
	}
	httpServer.logger = log.New(io.MultiWriter(logf, os.Stderr), "", log.Ltime|log.Lmicroseconds)

	return httpServer
}

func (s *HttpServer) reset() {
	s.clientAddr = nil
	s.reciverWindow = make([]*lib.Packet, WindowSize)
	s.seqNum = 0
	s.senderWindow = make([]*lib.WindowElement, WindowSize)
	s.deliveryMessage = make(chan bool, 1)
	handshakeSynChannel = make(chan *lib.Packet, 1)
	handshakeAckChannel = make(chan *lib.Packet, 1)
	go s.checkForDeliver()
}

// receive function receives all incomming packets
func (s *HttpServer) receive() {
	buffer := make([]byte, BufferSize)

	for {
		n, _, err := s.serverConn.ReadFromUDP(buffer)
		if err != nil {
			s.logger.Println(err)
		}
		p, err := lib.ParsePacket(buffer[:n])
		if err != nil {
			s.logger.Println(err)
		}

		// when connection is not established, only receive the handshake packet
		if !s.isConnected() {
			// Response to handshake
			if p.Type == lib.SYN && len(handshakeSynChannel) == 0 {
				handshakeSynChannel <- p
			}
			if p.Type == lib.ACK && p.SeqNum == 0 && len(handshakeAckChannel) == 0 {
				handshakeAckChannel <- p
			}
		}
		// DATA
		if p.Type == lib.DATA || p.Type == lib.DELIVER {
			// send ACK
			s.serverConn.WriteToUDP(lib.Packet{Type: lib.ACK, SeqNum: p.SeqNum, ToAddr: p.FromAddr}.Raw(), s.routerAddr)
			s.forward(p)
		}
	}
}

func (s *HttpServer) forward(packet *lib.Packet) {
	if s.reciverWindow[packet.SeqNum] == nil {
		s.reciverWindow[packet.SeqNum] = packet
	}
}

func (s *HttpServer) checkForDeliver() {
	deliverSeqNum := -1

	for {
		if deliverSeqNum == -1 {
			for i := 0; i < len(s.reciverWindow); i++ {
				if s.reciverWindow[i] != nil && s.reciverWindow[i].Type == lib.DELIVER {
					deliverSeqNum = i
					break
				}
			}
		} else {
			readyToDeliver := true
			for i := deliverSeqNum; i > 0; i-- {
				if s.reciverWindow[i] == nil {
					readyToDeliver = false
				}
			}

			if readyToDeliver {
				s.deliveryMessage <- true
				return
			}
		}
	}
}

// retransmission iterate through sender window and check for timeout
func (s *HttpServer) retransmission() {
	for {
		if s.isConnected() {
			for i := 0; i < int(s.seqNum); i++ {
				if s.senderWindow[i] != nil && s.senderWindow[i].Packet != nil && len(s.senderWindow[i].Ack) == 0 && len(s.senderWindow[i].Timer.C) != 0 {
					// resend
					_, err := s.serverConn.WriteToUDP(s.senderWindow[i].Packet.Raw(), s.routerAddr)
					if err != nil {
						s.logger.Printf("[retransmission][error] %s\n", err)
					}
					// reset timer
					<-s.senderWindow[i].Timer.C
					s.senderWindow[i].Timer.Reset(DefaultTimeOut)
				}
			}
		}
	}
}

func (s *HttpServer) send(req []byte) (*http.Response, []byte) {
	// // send req in packets
	// startByte := 0
	// for startByte < len(req) {
	// 	var payload []byte
	// 	if startByte+lib.MaxPayload >= len(req) {
	// 		payload = req[startByte:]
	// 	} else {
	// 		payload = req[startByte : startByte+lib.MaxPayload]
	// 	}
	// 	startByte += lib.MaxPayload
	// 	packet := lib.Packet{Type: lib.DATA, SeqNum: c.seqNum, ToAddr: c.serverAddr, Payload: payload}
	// 	c.sendPacket(&packet)
	// }

	// // wait for response and parse
	// for {

	// }
	return nil, nil
}

// sendPacket create new element in sender window and send packet
func (s *HttpServer) sendPacket(packet *lib.Packet) {
	_, err := s.serverConn.WriteToUDP(packet.Raw(), s.routerAddr)
	if err != nil {
		s.logger.Printf("[sendPacket][error] %s\n", err)
	}

	s.senderWindow[s.seqNum] = lib.NewWindowElement(packet, DefaultTimeOut)
	s.seqNum++
}

// concate all packet payload in recever window and return as []byte
func (s *HttpServer) getPackets() []byte {
	payload := []byte{}

	for i := 1; i < len(s.reciverWindow); i++ {
		if s.reciverWindow[i].Type == lib.DATA {
			payload = append(payload, s.reciverWindow[i].Payload...)
		}
		if s.reciverWindow[i].Type == lib.DELIVER {
			break
		}
	}

	return payload
}

/*
Establish tcp three-way handshaking
*/
func (s *HttpServer) establishConnection() {
	// Listen on SYN
	packet := <-handshakeSynChannel
	s.logger.Printf("[handshake] syn from %s\n", packet.FromAddr)
	if s.clientAddr == nil {
		s.clientAddr = packet.FromAddr
	}

	for {
		// Send SYN-ACK
		_, err = s.serverConn.WriteToUDP(lib.Packet{Type: lib.SYNACK, SeqNum: s.seqNum, ToAddr: s.clientAddr}.Raw(), s.routerAddr)
		if err != nil {
			s.logger.Println(err)
		}
		// Listen on channel and a timeout channel
		select {
		case res := <-handshakeAckChannel:
			s.logger.Printf("[handshake] ack from %s, connection established\n", res.FromAddr)
			s.connected()
			return
		case <-time.After(DefaultTimeOut):
			s.logger.Println("[handshake] ack time out, resent syn-ack")
		}
	}
}

func (s *HttpServer) isConnected() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.connect
}

func (s *HttpServer) connected() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.connect = true
}

func (s *HttpServer) disconnected() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.connect = false
}

type fileListResponse struct {
	Files []string `json:"files"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func makeResponse(httpCode int, contentType string, contentDisposition string, body string) []byte {
	res := fmt.Sprintf("HTTP/1.1 %d %s\r\n", httpCode, http.StatusText(httpCode))
	res += "Date: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n"

	if body != "" {
		res += "Content-Type: " + contentType + "\r\n"
		if contentDisposition != "" {
			res += "Content-Disposition: " + contentDisposition + "\r\n"
		}
		res += "Content-Length: " + fmt.Sprint(len(body)) + HttpRequestHeaderEnd + body
	} else {
		res += HttpRequestHeaderEnd
	}

	return []byte(res)
}

func getErrorResponse(httpStatusCode int, err error) []byte {
	fmt.Println("[error]: " + err.Error())
	b, _ := json.Marshal(errorResponse{Error: err.Error()})
	res := makeResponse(httpStatusCode, "application/json", "", string(b))
	return res
}

func handleGet(req *http.Request, verbose bool) ([]byte, error) {
	if verbose {
		fmt.Printf("[debug] handle GET request: %s, path: %s\n", req.URL, req.URL.Path)
	}
	if req.URL.Path == "" {
		var respBody fileListResponse
		respBody.Files = make([]string, 0)

		err := filepath.WalkDir(workingDirectory,
			func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() {
					rel, _ := filepath.Rel(workingDirectory, path)
					respBody.Files = append(respBody.Files, rel)
				}
				return nil
			})
		if err != nil {
			return getErrorResponse(http.StatusInternalServerError, err), err
		}

		b, _ := json.Marshal(respBody)
		res := makeResponse(http.StatusOK, "application/json", "", string(b))
		return res, nil
	} else {
		file, err := ioutil.ReadFile(workingDirectory + "/" + req.URL.Path[1:])
		if err != nil {
			if os.IsNotExist(err) {
				return getErrorResponse(http.StatusNotFound, err), err
			}
			return getErrorResponse(http.StatusInternalServerError, err), err
		}
		res := makeResponse(http.StatusOK, "text/plain", "attachment; filename="+req.URL.Path[1:], string(file))
		return res, nil
	}
}

func handlePost(req *http.Request, verbose bool) ([]byte, error) {
	if verbose {
		fmt.Printf("[debug] handle POST request: %s, path: %s\n", req.URL, req.URL.Path)
	}
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return getErrorResponse(http.StatusInternalServerError, err), err
	}

	if !verifyFilePath(req.URL.Path[1:]) {
		return getErrorResponse(http.StatusInternalServerError, fmt.Errorf("invalid file path: %s", req.URL.Path[1:])), err
	}

	f, err := os.Create(workingDirectory + "/" + req.URL.Path[1:])

	if err != nil {
		return getErrorResponse(http.StatusInternalServerError, err), err
	}

	defer f.Close()

	_, err = f.Write(b)

	if err != nil {
		return getErrorResponse(http.StatusInternalServerError, err), err
	}

	res := makeResponse(http.StatusCreated, "", "", "")
	return res, nil
}

func verifyFilePath(s string) bool {
	if strings.Contains(s, "..") {
		return false
	}

	return true
}

func parseHttpRequest(req []byte) (*http.Request, error) {
	var request *http.Request
	tmp := string(req)

	if strings.Contains(tmp, HttpRequestHeaderEnd) {
		header := tmp[:strings.Index(tmp, HttpRequestHeaderEnd)]
		// parse header
		reader := bufio.NewReader(strings.NewReader(header + HttpRequestHeaderEnd))
		req, err := http.ReadRequest(reader)
		request = req
		if err != nil {
			return request, err
		}
	}

	if request.Header.Get("Content-Length") == strconv.Itoa(len(tmp)-strings.Index(tmp, HttpRequestHeaderEnd)-len(HttpRequestHeaderEnd)) {
		// parse body
		body := ioutil.NopCloser(strings.NewReader(tmp[strings.Index(tmp, HttpRequestHeaderEnd)+len(HttpRequestHeaderEnd):]))
		request.Body = body
	}

	return request, nil
}

func handleRequest(req []byte, verbose bool) ([]byte, error) {

	request, err := parseHttpRequest(req)
	if err != nil {
		return nil, err
	}

	if request.Method == http.MethodGet {
		response, err := handleGet(request, verbose)
		return response, err
	}

	if request.Method == http.MethodPost {
		response, err := handlePost(request, verbose)
		return response, err
	}

	return nil, errors.New("Unsupported request " + string(req))
}

func main() {
	// parse args
	verbose := flag.Bool("v", false, "verbose mode")
	directory := flag.String("d", "", "specifies the directiory that the server will use to read/write")
	port := flag.Int("p", 8007, "echo server port")
	flag.Parse()

	workingDirectory, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to use working directory %s\n", err)
		return
	}
	if *directory != "" {
		if _, err := os.Stat(workingDirectory + *directory); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "failed to use working directory %s\n", err)
			return
		}
		workingDirectory += *directory
	}

	// Initialize http server
	httpServer := initHttpServer()

	// UDP setup
	// Initialize host url and port
	s, err := net.ResolveUDPAddr("udp", Host+":"+fmt.Sprint(*port))
	if err != nil {
		httpServer.logger.Fatalln(err)
	}
	httpServer.serverConn, err = net.ListenUDP("udp", s)
	if err != nil {
		httpServer.logger.Fatalln(err)
	}

	// Resolve Router address
	httpServer.routerAddr, err = net.ResolveUDPAddr("udp", Router)
	if err != nil {
		httpServer.logger.Fatalln(err)
	}

	defer httpServer.serverConn.Close()
	httpServer.logger.Println("file server is listening on", s.AddrPort(), " working dir:", *directory)

	// Start receiving packets
	go httpServer.receive()

	// Start retransmission routinue
	go httpServer.retransmission()

	// Start delivery checking
	go httpServer.checkForDeliver()

	for {
		httpServer.establishConnection()

		// receive request
		<-httpServer.deliveryMessage
		request := httpServer.getPackets()
		httpServer.logger.Printf("[debug] receive request:\n%s", string(request))

		// prepare response
		response, err := handleRequest(request, *verbose)
		if err != nil {
			httpServer.logger.Println(err)
		}
		httpServer.logger.Printf("[debug] send response:\n%s", string(response))
		// send response

		// httpServer.send(response)
		// fmt.Println(string(response))

		// close connection
		httpServer.disconnected()
		// reset
		httpServer.reset()
	}
}
