package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

const DefaultBufferSize = 1024
const CRLF = "\r\n"
const HttpRequestHeaderEnd = "\r\n\r\n"

func main() {
	port := flag.Int("port", 8007, "echo server port")
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
		go handleConn(conn)
	}
}

//echo reads data and sends back what it received until the channel is closed
func handleConn(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("new connection from %v\n", conn.RemoteAddr())

	buf := make([]byte, DefaultBufferSize)
	tmp := ""
	header := ""

	for {
		n, re := conn.Read(buf)
		if n > 0 {
			tmp += string(buf[:n])
			if strings.Contains(tmp, HttpRequestHeaderEnd) {
				header = tmp[:strings.Index(tmp, HttpRequestHeaderEnd)]
			}
			reader := bufio.NewReader(strings.NewReader(header + HttpRequestHeaderEnd))
			req, err := http.ReadRequest(reader)

			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(req.Method)
			fmt.Println(req.Header)

			if _, we := conn.Write([]byte("HTTP/1.1 200 OK" + HttpRequestHeaderEnd + "received!!")); we != nil {
				fmt.Fprintf(os.Stderr, "write error %v\n", we)
			}

			break
			// if _, we := conn.Write(buf[:n]); we != nil {
			// 	fmt.Fprintf(os.Stderr, "write error %v\n", we)
			// 	break
			// }
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			fmt.Fprintf(os.Stderr, "read error %v\n", re)
			break
		}
	}
}
