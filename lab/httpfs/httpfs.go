package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultBufferSize = 1024
const CRLF = "\r\n"
const HttpRequestHeaderEnd = "\r\n\r\n"
const ReadTimeoutDuration = 5 * time.Second

type fileListResponse struct {
	Files []string `json:"files"`
}

func main() {
	verbose := flag.Bool("v", false, "verbose mode")
	// directory := flag.String("d", "", "specifies the directiory that the server will use to read/write")
	port := flag.Int("p", 8007, "echo server port")
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen on %d\n", *port)
		return
	}
	defer listener.Close()

	fmt.Println("echo server is listening on", listener.Addr())
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error occured during accept connection %v\n", err)
			continue
		}
		go handleConn(conn, *verbose)
	}
}

func handleGet(conn net.Conn, req *http.Request, verbose bool) {
	if verbose {
		fmt.Printf("[debug] handle GET request: %s, path: %s\n", req.URL, req.URL.Path)
	}
	if req.URL.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
		}
		var respBody fileListResponse
		respBody.Files = make([]string, 0)

		files, err := ioutil.ReadDir(cwd)
		if err != nil {
			fmt.Println(err)
		}
		for _, file := range files {
			if !file.IsDir() {
				respBody.Files = append(respBody.Files, file.Name())
			}
		}
		b, _ := json.Marshal(respBody)

		// HTTP/1.1 200 OK
		// Date: Sun, 16 Oct 2022 05:09:56 GMT
		// Content-Type: application/json
		// Content-Length: 282
		res := "HTTP/1.1 200 OK" + "\r\n" +
			"Date: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n" +
			"Content-Type: application/json" + "\r\n" +
			"Content-Length: " + fmt.Sprint(len(string(b))) + HttpRequestHeaderEnd +
			string(b)

		if _, we := conn.Write([]byte(res)); we != nil {
			fmt.Fprintf(os.Stderr, "write error %v\n", we)
		}
	}
}

func handlePost(con net.Conn, req *http.Request, verbose bool) {
	fmt.Println("post method:")
	b, err := io.ReadAll(req.Body)
	// b, err := ioutil.ReadAll(resp.Body)  Go.1.15 and earlier
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(b))
}

//echo reads data and sends back what it received until the channel is closed
func handleConn(conn net.Conn, verbose bool) {
	defer func() {
		fmt.Printf("closing connection %v\n", conn.RemoteAddr())
		conn.Close()
	}()

	fmt.Printf("new connection from %v\n", conn.RemoteAddr())

	buf := make([]byte, DefaultBufferSize)
	tmp := ""
	var request *http.Request

	for {
		conn.SetReadDeadline(time.Now().Add(ReadTimeoutDuration))

		n, re := conn.Read(buf)
		if n > 0 {
			tmp += string(buf[:n])
			if strings.Contains(tmp, HttpRequestHeaderEnd) {
				header := tmp[:strings.Index(tmp, HttpRequestHeaderEnd)]
				// parse header
				reader := bufio.NewReader(strings.NewReader(header + HttpRequestHeaderEnd))
				req, err := http.ReadRequest(reader)
				request = req
				if err != nil {
					fmt.Println(err)
					return
				}
			}
			if request != nil && request.Header.Get("Content-Length") == "" {
				break
			} else if request != nil && request.Header.Get("Content-Length") == strconv.Itoa(len(tmp)-strings.Index(tmp, HttpRequestHeaderEnd)-len(HttpRequestHeaderEnd)) {
				// parse body
				body := ioutil.NopCloser(strings.NewReader(tmp[strings.Index(tmp, HttpRequestHeaderEnd)+len(HttpRequestHeaderEnd):]))
				request.Body = body
				break
			}
		}
		if re == io.EOF {
			return
		}
		if re != nil {
			fmt.Fprintf(os.Stderr, "read error %v\n", re)
			return
		}
	}

	// fmt.Println(req.Method)

	if request.Method == http.MethodGet {
		handleGet(conn, request, verbose)
	}

	if request.Method == http.MethodPost {
		handlePost(conn, request, verbose)
	}
}
