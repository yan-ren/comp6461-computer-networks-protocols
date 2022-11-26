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
	SeqNum         = 1
	WindowSize     = 1024
	DefaultTimeOut = 1 * time.Second
	BufferSize     = 2048
	Router         = "127.0.0.1:3000"
	Client         = "127.0.0.1:55341"
)

var handshakeAckChannel = make(chan *lib.Packet, 1)

type HttpClient struct {
	Proto         string
	Verbose       bool
	ClientConn    *net.UDPConn
	RouterAddress *net.UDPAddr
	ServerAddress *net.UDPAddr
	ReciveWindow  []lib.WindowElement
}

type httpHeaderFlags []string

func (h *httpHeaderFlags) String() string {
	return fmt.Sprint(*h)
}

func (h *httpHeaderFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func (c *HttpClient) HandleGetRequest(url string, headers httpHeaderFlags, output string) *http.Response {
	if c.validateUrl(url) {
		if c.Verbose {
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
		if c.Verbose {
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
	checkError(err)

	return true
}

/*
Establish tcp three-way handshaking
*/
func (c *HttpClient) establish() {
	// Send SYN
	_, err := c.ClientConn.WriteToUDP(lib.Packet{Type: lib.SYN, SeqNum: 0, ToAddr: c.ServerAddress}.Raw(), c.RouterAddress)
	checkError(err)

	// Listen on channel and a timeout channel
	select {
	case res := <-handshakeAckChannel:
		fmt.Printf("[handshake] syn-ack from %s\n", res.FromAddr)
	case <-time.After(DefaultTimeOut):
		fmt.Println("[handshake] syn time out, resent syn")
		c.establish()
		return
	}
}

func (c *HttpClient) receive() {
	buffer := make([]byte, BufferSize)

	for {
		n, _, err := c.ClientConn.ReadFromUDP(buffer)
		checkError(err)
		p, err := lib.ParsePacket(buffer[:n])
		checkError(err)
		// Response to handshake
		if p.Type == lib.SYNACK {
			c.ClientConn.WriteToUDP(lib.Packet{Type: lib.ACK, SeqNum: 0, ToAddr: c.ServerAddress}.Raw(), c.RouterAddress)
			if len(handshakeAckChannel) == 0 {
				handshakeAckChannel <- p
			}
		}
		// DATA
		if p.Type == lib.DATA && c.ReciveWindow[p.SeqNum].Packet != nil {
			c.ReciveWindow[p.SeqNum].Packet = p
		}
	}
}

func (c *HttpClient) send(u *url.URL, req []byte) (*http.Response, []byte) {
	// Router connection
	router, err := net.ResolveUDPAddr("udp", Router)
	checkError(err)
	client, err := net.DialUDP("udp", nil, router)
	checkError(err)

	// Host server
	server, err := net.ResolveUDPAddr("udp", u.Host)
	checkError(err)

	fmt.Printf("The UDP server is %s\n", client.RemoteAddr().String())
	defer client.Close()

	for {
		packet := lib.Packet{Type: lib.DATA, SeqNum: 1, ToAddr: server, Payload: req}
		_, err = client.Write(packet.Raw())
		checkError(err)

		buffer := make([]byte, 1024)
		n, _, err := client.ReadFromUDP(buffer)
		checkError(err)
		fmt.Printf("Reply: %s\n", string(buffer[0:n]))
	}

	return nil, nil
}

func (c *HttpClient) get(inputUrl string, headers httpHeaderFlags, output string) *http.Response {
	u, err := url.Parse(inputUrl)
	checkError(err)

	req := "GET " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n"
	for _, header := range headers {
		req += header + "\r\n"
	}
	req += HttpRequestHeaderEnd

	httpRes, rawRes := c.send(u, []byte(req))

	if httpRes.StatusCode == http.StatusOK {
		if c.Verbose {
			fmt.Println(string(rawRes))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				log.Fatal(err)
			}
			bodyString := string(bodyBytes)
			if output != "" {
				err := os.WriteFile(output, bodyBytes, 0644)
				checkError(err)
			} else {
				fmt.Println(bodyString)
			}
		}
	} else if httpRes.StatusCode == http.StatusMovedPermanently || httpRes.StatusCode == http.StatusFound {
		redirectUrl := httpRes.Header.Get("Location")
		if redirectUrl != "" {
			fmt.Printf("redirect to %s\n", redirectUrl)
			c.get(redirectUrl, nil, output)
		} else {
			fmt.Printf("redirct error: missing Location in header: %s\n", httpRes.Header)
		}
	} else {
		fmt.Println(string(rawRes))
	}

	return httpRes
}

func (c *HttpClient) post(inputUrl string, headers httpHeaderFlags, data string, output string) *http.Response {
	u, err := url.Parse(inputUrl)
	checkError(err)

	req := "POST " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n"

	for _, header := range headers {
		req += header + "\r\n"
	}
	req += "Content-Length: " + strconv.Itoa(len(data)) + HttpRequestHeaderEnd + data

	httpRes, rawRes := c.send(u, []byte(req))

	if httpRes.StatusCode == http.StatusOK {
		if c.Verbose {
			fmt.Println(string(rawRes))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				log.Fatal(err)
			}
			bodyString := string(bodyBytes)
			if output != "" {
				err := os.WriteFile(output, bodyBytes, 0644)
				checkError(err)
			} else {
				fmt.Println(bodyString)
			}
		}
	} else if httpRes.StatusCode == http.StatusMovedPermanently || httpRes.StatusCode == http.StatusFound {
		redirectUrl := httpRes.Header.Get("Location")
		if redirectUrl != "" {
			fmt.Printf("redirect to %s\n", redirectUrl)
			c.post(redirectUrl, headers, data, output)
		} else {
			fmt.Printf("redirct error: missing Location in header: %s\n", httpRes.Header)
		}
	} else {
		fmt.Println(string(rawRes))
	}

	return httpRes
}

func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	customSet := flag.NewFlagSet("", flag.ExitOnError)
	verbose := customSet.Bool("v", false, "verbose mode")
	// data := customSet.String("d", "", "inline data")
	// file := customSet.String("f", "", "file")
	// output := customSet.String("o", "", "output file name")

	var headers httpHeaderFlags
	customSet.Var(&headers, "h", "http headers")

	flag.Parse()
	customSet.Parse(os.Args[2:]) // skip the fisrt command line argument (get|post)

	commandLineInput := flag.Args()
	args := customSet.Args()
	requestUrl := args[len(args)-1]

	// Initialize http client
	httpClient := HttpClient{Proto: "HTTP/1.0", Verbose: *verbose, ReciveWindow: make([]lib.WindowElement, WindowSize)}

	// UDP Setup
	// Client connection
	clientAddr, err := net.ResolveUDPAddr("udp", Client)
	checkError(err)
	httpClient.ClientConn, err = net.ListenUDP("udp", clientAddr)
	defer httpClient.ClientConn.Close()
	checkError(err)

	// Host server address
	u, err := url.Parse(requestUrl)
	checkError(err)
	httpClient.ServerAddress, err = net.ResolveUDPAddr("udp", u.Host)
	checkError(err)

	// Router address
	httpClient.RouterAddress, err = net.ResolveUDPAddr("udp", Router)
	checkError(err)

	// Start receiver window
	go httpClient.receive()

	// Establish connection three-way handshake
	httpClient.establish()

	if len(commandLineInput) == 0 {
		fmt.Println(HelpMsg)
		flag.PrintDefaults()
		os.Exit(1)
	}

	for {

	}
	// for _, word := range commandLineInput {
	// 	if word == "help" {
	// 		if contains(commandLineInput, "get") {
	// 			fmt.Println(HelpGetMsg)
	// 			os.Exit(1)
	// 		} else if contains(commandLineInput, "post") {
	// 			fmt.Println(HelpPostMsg)
	// 			os.Exit(1)
	// 		} else {
	// 			fmt.Println(HelpMsg)
	// 			flag.PrintDefaults()
	// 			os.Exit(1)
	// 		}
	// 	}

	// 	if word == "get" {
	// 		httpClient.HandleGetRequest(args[len(args)-1], headers, *output)
	// 		os.Exit(1)
	// 	}

	// 	if word == "post" {
	// 		httpClient.HandlePostRequest(args[len(args)-1], headers, *data, *file, *output)
	// 		os.Exit(1)
	// 	}
	// }
}

func sendPacket(packet lib.Packet) {
	panic("unimplemented")
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
