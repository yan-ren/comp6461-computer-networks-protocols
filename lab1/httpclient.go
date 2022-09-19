package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/url"
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

func (c *HttpClient) Get(inputUrl string) {
	u, err := url.Parse(inputUrl)
	if err != nil {
		panic(err)
	}
	con, err := net.Dial("tcp", u.Host+":80")
	c.checkError(err)

	req := "GET " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n" + HttpRequestHeaderEnd

	_, err = con.Write([]byte(req))
	c.checkError(err)

	res, err := ioutil.ReadAll(con)
	c.checkError(err)

	fmt.Println(string(res))
}

func (c *HttpClient) Post(inputUrl string, data string) {
	u, err := url.Parse(inputUrl)
	if err != nil {
		panic(err)
	}
	con, err := net.Dial("tcp", u.Host+":80")
	c.checkError(err)

	req := "POST " + inputUrl + " " + c.Proto + "\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(data)) + "\r\n" +
		"Content-Type: application/json" + HttpRequestHeaderEnd +
		data

	_, err = con.Write([]byte(req))
	c.checkError(err)

	res, err := ioutil.ReadAll(con)
	c.checkError(err)

	fmt.Println(string(res))
}

func (c *HttpClient) checkError(err error) {

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	client := HttpClient{Proto: "HTTP/1.0"}
	// client.Get("http://httpbin.org/get?course=networking&assignment=1")

	client.Post("http://httpbin.org/post", "{\"Assignment\": 1}")
	// flag.Parse()

	// values := flag.Args()

	// if len(values) == 0 {
	// 	fmt.Println(HelpMsg)
	// 	flag.PrintDefaults()
	// 	os.Exit(1)
	// }

	// for _, word := range values {
	// 	if word == "help" {
	// 		if contains(values, "get") {
	// 			fmt.Println(HelpGetMsg)
	// 			os.Exit(1)
	// 		} else if contains(values, "post") {
	// 			fmt.Println(HelpPostMsg)
	// 			os.Exit(1)
	// 		} else {
	// 			fmt.Println(HelpMsg)
	// 			flag.PrintDefaults()
	// 			os.Exit(1)
	// 		}
	// 	}
	// }
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
