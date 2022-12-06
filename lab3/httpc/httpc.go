package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"lab3/lib"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

const HelpMsg = `
httpc help
httpc is a curl-like application but supports HTTP protocol only.
Usage:
	httpc command [arguments]
The commands are:
	get     executes a HTTP GET request and prints the response.
	post	executes a HTTP POST request and prints the response.
	help	prints this screen.
Use "httpc help [command]" for more information about a command.`

const HelpGetMsg = `
httpc help get

usage: httpc get [-v] [-h key:value] URL

Get executes a HTTP GET request for a given URL.
	-v				Prints the detail of the response such as protocol, status, and headers
	-h key:value	Associates headers to HTTP request with the format 'key:value'`

const HelpPostMsg = `
httpc help post

usage: httpc post [-v] [-h key:value] [-d inline-data] [-f file] URL

Post executes a HTTP POST request for a given URL with inline data or from file.
	-v				Prints the detail of the response such as protocol, status, and headers.
	-h key:value	Associates headers to HTTP Request with the format 'key:value'
	-d string		Associates an inline data to the body HTTP POST request.
	-f file			Associates the content of a file to the body HTTP POST.`

const HttpRequestHeaderEnd = "\r\n\r\n"

const (
	DefaultTimeOut = 1 * time.Second
	UDPBufferSize  = 2048
	WindowSize     = 1024
	Router         = "127.0.0.1:3000"
	Client         = "127.0.0.1:55341"
)

var handshakeSYNACKChannel = make(chan *lib.Packet, 1)

type httpHeaderFlags []string

func (h *httpHeaderFlags) String() string {
	return fmt.Sprint(*h)
}

func (h *httpHeaderFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

type HttpClient struct {
	proto      string
	verbose    bool
	seqNum     uint32
	clientConn *net.UDPConn
	routerAddr *net.UDPAddr
	serverAddr *net.UDPAddr

	receiverWindow []*lib.Packet
	senderWindow   []*lib.WindowElement

	deliveryMessage chan bool
	logger          *log.Logger
}

func initHttpClient(verbose bool) *HttpClient {
	httpClient := &HttpClient{
		proto:           "HTTP/1.0",
		verbose:         verbose,
		seqNum:          0,
		receiverWindow:  make([]*lib.Packet, WindowSize),
		senderWindow:    make([]*lib.WindowElement, WindowSize),
		deliveryMessage: make(chan bool, 1),
	}
	logf, err := os.OpenFile("httpc.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v", err)
		panic(err)
	}
	httpClient.logger = log.New(io.MultiWriter(logf, os.Stderr), "", log.Ltime|log.Lmicroseconds)

	return httpClient
}

// checkForDeliver writes to readyToDeliver channel when packets in receiver window is ready to be delivered
func (c *HttpClient) checkForDeliver() {
	deliverSeqNum := -1

	for {
		if deliverSeqNum == -1 {
			for i := 0; i < len(c.receiverWindow); i++ {
				if c.receiverWindow[i] != nil && c.receiverWindow[i].Type == lib.DELIVER {
					deliverSeqNum = i
					break
				}
			}
		} else {
			readyToDeliver := true
			for i := deliverSeqNum; i > 0; i-- {
				if c.receiverWindow[i] == nil {
					readyToDeliver = false
				}
			}

			if readyToDeliver {
				c.deliveryMessage <- true
				return
			}
		}
	}
}

// establishConnection send syn to establish tcp three-way handshaking
func (c *HttpClient) establishConnection() {
	for {
		// Send SYN
		_, err := c.clientConn.WriteToUDP(lib.Packet{Type: lib.SYN, SeqNum: c.seqNum, ToAddr: c.serverAddr}.Raw(), c.routerAddr)
		c.logger.Printf("[handshake] syn to %s\n", c.serverAddr)
		if err != nil {
			c.logger.Printf("[establishConnection][error] %s\n", err)
		}

		// Listen on channel and a timeout channel
		select {
		case res := <-handshakeSYNACKChannel:
			c.logger.Printf("[handshake] syn-ack from %s, connection established\n", res.FromAddr)
			c.seqNum++
			return
		case <-time.After(DefaultTimeOut):
			c.logger.Println("[handshake] syn time out, resent syn")
		}
	}
}

func (c *HttpClient) get(inputUrl string, headers httpHeaderFlags) (*http.Response, []byte, error) {
	u, err := url.Parse(inputUrl)
	if err != nil {
		c.logger.Fatalf("[get][error] %s\n", err)
	}
	// make http GET request
	req := "GET " + inputUrl + " " + c.proto + "\r\n" +
		"Host: " + u.Host + "\r\n"
	for _, header := range headers {
		req += header + "\r\n"
	}
	req += HttpRequestHeaderEnd
	// mock http request object
	httpReq, _ := http.NewRequest("GET", inputUrl, nil)
	// send
	rawRes := c.send(u, []byte(req))
	httpRes, err := c.paresHttpResponse(rawRes, httpReq)

	return httpRes, rawRes, err
}

// getPackets concates all packets' payload in receiver window and return as []byte
func (c *HttpClient) getPackets() []byte {
	payload := []byte{}

	for i := 1; i < len(c.receiverWindow); i++ {
		if c.receiverWindow[i].Type == lib.DATA {
			payload = append(payload, c.receiverWindow[i].Payload...)
		}
		if c.receiverWindow[i].Type == lib.DELIVER {
			break
		}
	}

	return payload
}

// paresHttpResponse parses http response from raw bytes
func (c *HttpClient) paresHttpResponse(rawRes []byte, httpReq *http.Request) (*http.Response, error) {
	httpRes, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rawRes)), httpReq)
	return httpRes, err
}

func (c *HttpClient) post(inputUrl string, headers httpHeaderFlags, data string) (*http.Response, []byte, error) {
	u, err := url.Parse(inputUrl)
	if err != nil {
		c.logger.Fatalf("[post][error] %s\n", err)
	}

	req := "POST " + inputUrl + " " + c.proto + "\r\n" +
		"Host: " + u.Host + "\r\n"

	for _, header := range headers {
		req += header + "\r\n"
	}
	req += "Content-Length: " + strconv.Itoa(len(data)) + HttpRequestHeaderEnd + data
	// mock http request object
	mockHttpReq, _ := http.NewRequest("GET", inputUrl, nil)
	// send
	rawRes := c.send(u, []byte(req))
	httpRes, err := c.paresHttpResponse(rawRes, mockHttpReq)

	return httpRes, rawRes, err
}

// receive function receives all incomming packets
func (c *HttpClient) receive() {

	for {
		buffer := make([]byte, UDPBufferSize)
		n, _, err := c.clientConn.ReadFromUDP(buffer)
		if err != nil {
			c.logger.Println(err)
		}
		p, err := lib.ParsePacket(buffer[:n])
		if err != nil {
			c.logger.Println(err)
		}
		// Response to handshake
		if p.Type == lib.SYNACK {
			c.clientConn.WriteToUDP(lib.Packet{Type: lib.ACK, SeqNum: p.SeqNum, ToAddr: c.serverAddr}.Raw(), c.routerAddr)
			c.logger.Printf("[receive][handshake] ack to %s, packet %s\n", p.FromAddr, p)
			if len(handshakeSYNACKChannel) == 0 {
				handshakeSYNACKChannel <- p
			}
		}
		// ACK
		if p.Type == lib.ACK {
			if len(c.senderWindow[p.SeqNum].Ack) == 0 && c.senderWindow[p.SeqNum].Timer != nil {
				c.senderWindow[p.SeqNum].Timer.Stop()
				c.senderWindow[p.SeqNum].Ack <- p
				// c.logger.Printf("[receive] ack from %s, packet %s\n", p.FromAddr, p)
			}
		}
		// DATA, DELIVER
		if p.Type == lib.DATA || p.Type == lib.DELIVER {
			// send ACK
			c.clientConn.WriteToUDP(lib.Packet{Type: lib.ACK, SeqNum: p.SeqNum, ToAddr: p.FromAddr}.Raw(), c.routerAddr)
			// c.logger.Printf("[receive] ack to %s, packet %s\n", p.FromAddr, p)
			// save packet in receiver window
			if c.receiverWindow[p.SeqNum] == nil {
				c.receiverWindow[p.SeqNum] = p
			}
		}
	}
}

// retransmission iterate through sender window and check for timeout
func (c *HttpClient) retransmission() {
	for {
		for i := 0; i < int(c.seqNum); i++ {
			if c.senderWindow[i] != nil && c.senderWindow[i].Packet != nil && len(c.senderWindow[i].Ack) == 0 && len(c.senderWindow[i].Timer.C) != 0 {
				// resend
				_, err := c.clientConn.WriteToUDP(c.senderWindow[i].Packet.Raw(), c.routerAddr)
				c.logger.Printf("[retransmission][timeout] to %s, packet %s\n", c.senderWindow[i].Packet.ToAddr, c.senderWindow[i].Packet)

				if err != nil {
					c.logger.Printf("[retransmission][error] %s\n", err)
				}
				// reset timer
				<-c.senderWindow[i].Timer.C
				c.senderWindow[i].Timer.Reset(DefaultTimeOut)
			}
		}
	}
}

// send synchronous for req as packets and block until all packets being acknowledged
func (c *HttpClient) send(u *url.URL, req []byte) []byte {
	// send req as packets
	startByte := 0
	for startByte < len(req) {
		var payload []byte
		if startByte+lib.MaxPayload >= len(req) {
			payload = req[startByte:]
		} else {
			payload = req[startByte : startByte+lib.MaxPayload]
		}
		startByte += lib.MaxPayload
		packet := lib.Packet{Type: lib.DATA, SeqNum: c.seqNum, ToAddr: c.serverAddr, Payload: payload}
		c.sendPacket(&packet)
	}
	// send delivery notice
	c.sendPacket(&lib.Packet{Type: lib.DELIVER, SeqNum: c.seqNum, ToAddr: c.serverAddr})
	c.logger.Println("[send] all response packets sent, wait for response...")

	// wait for response and parse
	<-c.deliveryMessage
	// get response
	rawResp := c.getPackets()
	c.logger.Printf("[deliver_packets] deliver combined packets: %s\n", string(rawResp))

	return rawResp
}

// sendPacket create new element in sender window and send packet
func (c *HttpClient) sendPacket(packet *lib.Packet) {
	_, err := c.clientConn.WriteToUDP(packet.Raw(), c.routerAddr)
	if err != nil {
		c.logger.Printf("[sendPacket][error] %s\n", err)
	}

	c.senderWindow[c.seqNum] = lib.NewWindowElement(packet, DefaultTimeOut)
	c.seqNum++
}

func (c *HttpClient) validateUrl(inputUrl string) bool {
	_, err := url.ParseRequestURI(inputUrl)
	if err != nil {
		c.logger.Fatalf("[validateUrl][error] %s\n", err)
	}

	return true
}

// processHttpResponse writes to file or write to logger
func (c *HttpClient) processHttpResponse(httpRes *http.Response, rawRes []byte, responseLog string) {
	if httpRes.StatusCode == http.StatusOK {
		// parse response body
		bodyBytes, err := io.ReadAll(httpRes.Body)
		if err != nil {
			c.logger.Fatalf("[paresHttpResponse][error] %s\n", err)
		}
		bodyString := string(bodyBytes)

		// write to response file or write to logger
		if responseLog != "" {
			err := os.WriteFile(responseLog, bodyBytes, 0644)
			if err != nil {
				c.logger.Fatalf("[paresHttpResponse][error] %s\n", err)
			}
		} else {
			c.logger.Printf("[paresHttpResponse] %s\n", bodyString)
		}
	} else {
		c.logger.Printf("[paresHttpResponse] %s\n", string(rawRes))
	}
}

func (c *HttpClient) HandleGetRequest(url string, headers httpHeaderFlags, outputPath string) {
	if c.validateUrl(url) {
		if c.verbose {
			c.logger.Println("[HandleGetRequest] received url: " + url)
			c.logger.Println("[HandleGetRequest] received headers: " + headers.String())
		}
		httpRes, resRaw, err := c.get(url, headers)
		if err != nil {
			c.logger.Printf("[HandleGetRequest][error] response: %s, error: %s\n", resRaw, err)
		} else {
			c.processHttpResponse(httpRes, resRaw, outputPath)
		}
	} else {
		c.logger.Printf("[HandleGetRequest][error] invalid request url: %s\n", url)
	}
}

func (c *HttpClient) HandlePostRequest(url string, headers httpHeaderFlags, data string, inputFilePath string, outputPath string) {
	if data != "" && inputFilePath != "" {
		log.Fatal("cannot use [-d] and [-f] at same time")
	}

	if c.validateUrl(url) {
		if c.verbose {
			c.logger.Println("[HandlePostRequest] received url: " + url)
			c.logger.Println("[HandlePostRequest] received headers: " + headers.String())
		}

		if data == "" && inputFilePath != "" {
			data = loadDataFile(inputFilePath)
		}
		httpRes, resRaw, err := c.post(url, headers, data)
		if err != nil {
			c.logger.Printf("[HandlePostRequest][error] response: %s, error: %s\n", resRaw, err)
		} else {
			c.processHttpResponse(httpRes, resRaw, outputPath)
		}
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func loadDataFile(file string) string {
	content, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}

	return string(content)
}

func main() {
	customSet := flag.NewFlagSet("", flag.ExitOnError)
	verbose := customSet.Bool("v", false, "verbose mode")
	data := customSet.String("d", "", "inline data")
	file := customSet.String("f", "", "file")
	output := customSet.String("o", "", "output file name")

	var headers httpHeaderFlags
	customSet.Var(&headers, "h", "http headers")

	flag.Parse()
	customSet.Parse(os.Args[2:]) // skip the fisrt command line argument (get|post)

	commandLineInput := flag.Args()
	args := customSet.Args()
	requestUrl := args[len(args)-1]

	// Initialize http client
	httpClient := initHttpClient(*verbose)

	// UDP Setup
	// Client connection
	clientAddr, err := net.ResolveUDPAddr("udp", Client)
	if err != nil {
		httpClient.logger.Fatalln(err)
	}
	httpClient.clientConn, err = net.ListenUDP("udp", clientAddr)
	if err != nil {
		httpClient.logger.Fatalln(err)
	}
	defer httpClient.clientConn.Close()

	// Host server address
	u, err := url.Parse(requestUrl)
	if err != nil {
		httpClient.logger.Fatalln(err)
	}
	httpClient.serverAddr, err = net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		httpClient.logger.Fatalln(err)
	}

	// Router address
	httpClient.routerAddr, err = net.ResolveUDPAddr("udp", Router)
	if err != nil {
		httpClient.logger.Fatalln(err)
	}

	// Start receiving packets
	go httpClient.receive()

	// Start retransmission routine
	go httpClient.retransmission()

	// Start delivery checking
	go httpClient.checkForDeliver()

	// Establish connection three-way handshake
	httpClient.establishConnection()

	if len(commandLineInput) == 0 {
		fmt.Println(HelpMsg)
		flag.PrintDefaults()
		os.Exit(1)
	}

	for _, word := range commandLineInput {
		if word == "help" {
			if contains(commandLineInput, "get") {
				fmt.Println(HelpGetMsg)
				os.Exit(1)
			} else if contains(commandLineInput, "post") {
				fmt.Println(HelpPostMsg)
				os.Exit(1)
			} else {
				fmt.Println(HelpMsg)
				flag.PrintDefaults()
				os.Exit(1)
			}
		}

		if word == "get" {
			httpClient.HandleGetRequest(requestUrl, headers, *output)
		}

		if word == "post" {
			httpClient.HandlePostRequest(requestUrl, headers, *data, *file, *output)
		}

		// block, in case any ack to server is lost and need resend
		for {

		}
	}
}
