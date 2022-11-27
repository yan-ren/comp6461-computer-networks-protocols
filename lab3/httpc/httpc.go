package main

import (
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
	DefaultTimeOut   = 1 * time.Second
	UDPBufferSize    = 2048
	SenderWindowSize = 1024
	Router           = "127.0.0.1:3000"
	Client           = "127.0.0.1:55341"
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

	senderWindow []*lib.WindowElement

	deliveryMessage chan bool
	logger          *log.Logger
}

func initHttpClient(verbose bool) *HttpClient {
	httpClient := &HttpClient{proto: "HTTP/1.0", verbose: verbose, seqNum: 0, senderWindow: make([]*lib.WindowElement, SenderWindowSize)}
	httpClient.deliveryMessage = make(chan bool, 1)
	logf, err := os.OpenFile("client.log."+fmt.Sprint(time.Now().Unix()), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0660)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file: %v", err)
		panic(err)
	}
	httpClient.logger = log.New(io.MultiWriter(logf, os.Stderr), "", log.Ltime|log.Lmicroseconds)

	return httpClient
}

func (c *HttpClient) HandleGetRequest(url string, headers httpHeaderFlags, output string) *http.Response {
	if c.validateUrl(url) {
		if c.verbose {
			fmt.Println("received url: " + url)
			fmt.Println("received headers: " + headers.String())
		}
		return c.get(url, headers, output)
	}

	return nil
}

func (c *HttpClient) HandlePostRequest(url string, headers httpHeaderFlags, data string, file string, output string) *http.Response {
	if data != "" && file != "" {
		log.Fatal("cannot use [-d] and [-f] at same time")
	}

	if c.validateUrl(url) {
		if c.verbose {
			fmt.Println("received url: " + url)
			fmt.Println("received headers: " + headers.String())
		}

		if data == "" && file != "" {
			data = loadDataFile(file)
		}
		return c.post(url, headers, data, output)
	}

	return nil
}

func (c *HttpClient) validateUrl(inputUrl string) bool {
	_, err := url.ParseRequestURI(inputUrl)
	if err != nil {
		c.logger.Fatalf("[validateUrl][error] %s\n", err)
	}

	return true
}

/*
Establish tcp three-way handshaking
*/
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

// retransmission iterate through sender window and check for timeout
func (c *HttpClient) retransmission() {
	for {
		for i := 0; i < int(c.seqNum); i++ {
			if c.senderWindow[i] != nil && c.senderWindow[i].Packet != nil && len(c.senderWindow[i].Ack) == 0 && len(c.senderWindow[i].Timer.C) != 0 {
				// resend
				_, err := c.clientConn.WriteToUDP(c.senderWindow[i].Packet.Raw(), c.routerAddr)
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

// receive function receives all incomming packets
func (c *HttpClient) receive() {
	buffer := make([]byte, UDPBufferSize)

	for {
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
			if len(handshakeSYNACKChannel) == 0 {
				handshakeSYNACKChannel <- p
			}
		}
		// ACK
		if p.Type == lib.ACK {
			if len(c.senderWindow[p.SeqNum].Ack) == 0 {
				c.senderWindow[p.SeqNum].Timer.Stop()
				c.senderWindow[p.SeqNum].Ack <- p
				c.logger.Printf("[debug] ack from %s, packet %s\n", p.FromAddr, p)
			}
		}
		// DATA
		if p.Type == lib.DATA && c.senderWindow[p.SeqNum].Packet != nil {
			c.senderWindow[p.SeqNum].Packet = p
		}
	}
}

func (c *HttpClient) send(u *url.URL, req []byte) (*http.Response, []byte) {
	// send req in packets
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

	// wait for response and parse
	<-c.deliveryMessage

	return nil, nil
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

func (c *HttpClient) get(inputUrl string, headers httpHeaderFlags, output string) *http.Response {
	u, err := url.Parse(inputUrl)
	if err != nil {
		c.logger.Fatalf("[get][error] %s\n", err)
	}

	req := "GET " + inputUrl + " " + c.proto + "\r\n" +
		"Host: " + u.Host + "\r\n"
	for _, header := range headers {
		req += header + "\r\n"
	}
	req += HttpRequestHeaderEnd

	httpRes, rawRes := c.send(u, []byte(req))

	if httpRes.StatusCode == http.StatusOK {
		if c.verbose {
			fmt.Println(string(rawRes))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				c.logger.Fatalf("[get][error] %s\n", err)
			}
			bodyString := string(bodyBytes)
			if output != "" {
				err := os.WriteFile(output, bodyBytes, 0644)
				if err != nil {
					c.logger.Fatalf("[get][error] %s\n", err)
				}
			} else {
				fmt.Println(bodyString)
			}
		}
	} else if httpRes.StatusCode == http.StatusMovedPermanently || httpRes.StatusCode == http.StatusFound {
		redirectUrl := httpRes.Header.Get("Location")
		if redirectUrl != "" {
			c.logger.Printf("redirect to %s\n", redirectUrl)
			c.get(redirectUrl, nil, output)
		} else {
			c.logger.Printf("redirct error: missing Location in header: %s\n", httpRes.Header)
		}
	} else {
		c.logger.Println(string(rawRes))
	}

	return httpRes
}

func (c *HttpClient) post(inputUrl string, headers httpHeaderFlags, data string, output string) *http.Response {
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

	httpRes, rawRes := c.send(u, []byte(req))

	if httpRes.StatusCode == http.StatusOK {
		if c.verbose {
			fmt.Println(string(rawRes))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				c.logger.Fatalf("[post][error] %s\n", err)
			}
			bodyString := string(bodyBytes)
			if output != "" {
				err := os.WriteFile(output, bodyBytes, 0644)
				if err != nil {
					c.logger.Fatalf("[post][error] %s\n", err)
				}
			} else {
				fmt.Println(bodyString)
			}
		}
	} else if httpRes.StatusCode == http.StatusMovedPermanently || httpRes.StatusCode == http.StatusFound {
		redirectUrl := httpRes.Header.Get("Location")
		if redirectUrl != "" {
			c.logger.Printf("redirect to %s\n", redirectUrl)
			c.post(redirectUrl, headers, data, output)
		} else {
			c.logger.Printf("redirct error: missing Location in header: %s\n", httpRes.Header)
		}
	} else {
		c.logger.Println(string(rawRes))
	}

	return httpRes
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

	// Start retransmission routinue
	go httpClient.retransmission()

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
			os.Exit(1)
		}

		if word == "post" {
			httpClient.HandlePostRequest(requestUrl, headers, *data, *file, *output)
			os.Exit(1)
		}
	}
}
