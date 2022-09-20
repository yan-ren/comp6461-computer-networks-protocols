package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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

type HttpClient struct {
	Proto   string
	Verbose bool
}

type httpHeaderFlags []string

func (h *httpHeaderFlags) String() string {
	return fmt.Sprint(*h)
}

func (h *httpHeaderFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func (c *HttpClient) HandleGetRequest(url string, headers httpHeaderFlags) *http.Response {
	if c.validateUrl(url) {
		if c.Verbose {
			fmt.Println("received url: " + url)
			fmt.Println("received headers: " + headers.String())
		}
		return c.get(url, headers)
	}

	return nil
}

func (c *HttpClient) HandlePostRequest(url string, headers httpHeaderFlags, data string, file string) *http.Response {
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
		return c.post(url, headers, data)
	}

	return nil
}

func (c *HttpClient) validateUrl(inputUrl string) bool {
	_, err := url.ParseRequestURI(inputUrl)
	c.checkError(err)

	return true
}

func (c *HttpClient) get(inputUrl string, headers httpHeaderFlags) *http.Response {
	u, err := url.Parse(inputUrl)
	c.checkError(err)

	con, err := net.Dial("tcp", u.Host+":80")
	c.checkError(err)

	req := "GET " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n"
	for _, header := range headers {
		req += header + "\r\n"
	}

	req += HttpRequestHeaderEnd

	// mock
	httpReq, _ := http.NewRequest("GET", inputUrl, nil)

	_, err = con.Write([]byte(req))
	c.checkError(err)

	res, err := ioutil.ReadAll(con)
	c.checkError(err)

	httpRes, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(res)), httpReq)
	c.checkError(err)

	if httpRes.StatusCode == http.StatusOK {
		if c.Verbose {
			fmt.Println(string(res))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				log.Fatal(err)
			}
			bodyString := string(bodyBytes)
			fmt.Println(bodyString)
		}

		return httpRes
	} else {
		fmt.Println(string(res))
	}

	return httpRes
}

func (c *HttpClient) post(inputUrl string, headers httpHeaderFlags, data string) *http.Response {
	u, err := url.Parse(inputUrl)
	c.checkError(err)

	con, err := net.Dial("tcp", u.Host+":80")
	c.checkError(err)

	req := "POST " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n"

	for _, header := range headers {
		req += header + "\r\n"
	}
	req += "Content-Length: " + strconv.Itoa(len(data)) + HttpRequestHeaderEnd + data

	// mock
	httpReq, _ := http.NewRequest("POST", inputUrl, nil)

	_, err = con.Write([]byte(req))
	c.checkError(err)

	res, err := ioutil.ReadAll(con)
	c.checkError(err)

	httpRes, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(res)), httpReq)
	c.checkError(err)

	if httpRes.StatusCode == http.StatusOK {
		if c.Verbose {
			fmt.Println(string(res))
		} else {
			bodyBytes, err := io.ReadAll(httpRes.Body)
			if err != nil {
				log.Fatal(err)
			}
			bodyString := string(bodyBytes)
			fmt.Println(bodyString)
		}

		return httpRes
	} else {
		fmt.Println(string(res))
	}

	return httpRes
}

func (c *HttpClient) checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	// client.Get("http://httpbin.org/get?course=networking&assignment=1")
	// client.Post("http://httpbin.org/post", "{\"Assignment\": 1}")

	customSet := flag.NewFlagSet("", flag.ExitOnError)
	verbose := customSet.Bool("v", false, "verbose mode")
	data := customSet.String("d", "", "inline data")
	file := customSet.String("f", "", "file")

	var headers httpHeaderFlags
	customSet.Var(&headers, "h", "http headers")

	flag.Parse()
	customSet.Parse(os.Args[2:]) // skip the fisrt command line argument (get|post)

	commandLineInput := flag.Args()
	args := customSet.Args()

	client := HttpClient{Proto: "HTTP/1.0", Verbose: *verbose}

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
			client.HandleGetRequest(args[len(args)-1], headers)
			os.Exit(1)
		}

		if word == "post" {
			client.HandlePostRequest(args[len(args)-1], headers, *data, *file)
			os.Exit(1)
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
