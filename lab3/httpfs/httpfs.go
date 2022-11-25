package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"lab3/lib"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBufferSize    = 2048
	CRLF                 = "\r\n"
	HttpRequestHeaderEnd = "\r\n\r\n"
	ReadTimeoutDuration  = 5 * time.Second
	Host                 = "127.0.0.1"
)

var workingDirectory string
var err error

type fileListResponse struct {
	Files []string `json:"files"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	// parse args
	// verbose := flag.Bool("v", false, "verbose mode")
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

	// udp setup
	s, err := net.ResolveUDPAddr("udp", Host+":"+fmt.Sprint(*port))
	if err != nil {
		fmt.Println(err)
		return
	}

	connection, err := net.ListenUDP("udp", s)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer connection.Close()
	buffer := make([]byte, 1024)

	fmt.Println("file server is listening on", s.AddrPort(), " working dir:", *directory)
	for {
		n, fromAddr, err := connection.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("failed to receive message:", err)
			return
		}

		p, err := lib.ParsePacket(fromAddr, buffer[:n])
		if err != nil {
			fmt.Println("invalid packet:", err)
			continue
		}

		process(connection, *p)
	}
}

func process(conn *net.UDPConn, p lib.Packet) {
	fmt.Printf("receive packet %s", p)
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

func sendResponse(conn net.Conn, res []byte) {
	if _, we := conn.Write(res); we != nil {
		fmt.Fprintf(os.Stderr, "write error %v\n", we)
	}
}

func handleError(conn net.Conn, httpStatusCode int, err error) {
	fmt.Println("[error]: " + err.Error())
	b, _ := json.Marshal(errorResponse{Error: err.Error()})
	res := makeResponse(httpStatusCode, "application/json", "", string(b))
	sendResponse(conn, res)
}

func handleGet(conn net.Conn, req *http.Request, verbose bool) {
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
			handleError(conn, http.StatusInternalServerError, err)
			return
		}

		b, _ := json.Marshal(respBody)
		res := makeResponse(http.StatusOK, "application/json", "", string(b))
		sendResponse(conn, res)
	} else {
		file, err := ioutil.ReadFile(workingDirectory + "/" + req.URL.Path[1:])
		if err != nil {
			if os.IsNotExist(err) {
				handleError(conn, http.StatusNotFound, err)
			} else {
				handleError(conn, http.StatusInternalServerError, err)
			}
			return
		}
		res := makeResponse(http.StatusOK, "text/plain", "attachment; filename="+req.URL.Path[1:], string(file))
		sendResponse(conn, res)
	}
}

func handlePost(conn net.Conn, req *http.Request, verbose bool) {
	if verbose {
		fmt.Printf("[debug] handle POST request: %s, path: %s\n", req.URL, req.URL.Path)
	}
	b, err := io.ReadAll(req.Body)
	if err != nil {
		handleError(conn, http.StatusInternalServerError, err)
		return
	}

	if !verifyFilePath(req.URL.Path[1:]) {
		handleError(conn, http.StatusInternalServerError, fmt.Errorf("invalid file path: %s", req.URL.Path[1:]))
		return
	}

	f, err := os.Create(workingDirectory + "/" + req.URL.Path[1:])

	if err != nil {
		handleError(conn, http.StatusInternalServerError, err)
		return
	}

	defer f.Close()

	_, err = f.Write(b)

	if err != nil {
		handleError(conn, http.StatusInternalServerError, err)
		return
	}

	res := makeResponse(http.StatusCreated, "", "", "")
	sendResponse(conn, res)
}

func verifyFilePath(s string) bool {
	if strings.Contains(s, "..") {
		return false
	}

	return true
}

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

	if request.Method == http.MethodGet {
		handleGet(conn, request, verbose)
	}

	if request.Method == http.MethodPost {
		handlePost(conn, request, verbose)
	}
}
