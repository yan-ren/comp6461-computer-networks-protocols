# COMP6461 Computer Networks Protocols

COMP6461 Computer Networks Protocols Fall2022 course labs implemented in Go.

## Usage Examples

## Lab 1
#### General
```
./httpc help
```
#### Get Usage
```
./httpc help get
```
#### Post Usage
```
./httpc help post
```
#### Get with query parameters
```
./httpc get 'http://httpbin.org:80/get?course=networking&assignment=1'
```
#### Get with verbose option
```
./httpc get -v 'http://httpbin.org:80/get?course=networking&assignment=1'
```
#### Get with mutiple headers
```
./httpc get -v -h User-Agent:Concordia-HTTP/1.0 -h Host:httpbin.org 'http://httpbin.org:80/get?course=networking&assignment=1'
```
#### Post with inline data
```
./httpc post -h Content-Type:application/json --d '{"Assignment": 1}' http://httpbin.org:80/post
```
#### Post with data file
```
./httpc post -h Content-Type:application/json --f ./data_file.json http://httpbin.org:80/post
```
#### Optional Task
#### Output response to file
```
./httpc get -o hello.txt 'http://httpbin.org:80/get?course=networking&assignment=1' 
```
#### Redirection
```
./httpc get 'https://httpbin.org:80/redirect-to?url=http%3A%2F%2Fhttpbin.org:80%2Fget%3Fcourse%3Dnetworking%26assignment%3D1'
```
```
./httpc post -h Content-Type:application/json --d '{"Assignment": 1}' 'https://httpbin.org:80/redirect-to?url=http://httpbin.org:80/post'
```
## Lab 2
#### Launch file server, default port: 8007
```
./httpfs -v
```

#### Return a list of the current files in the data directory
```
./httpc get -v 'http://127.0.0.1:8007'
```

#### Returns the content of the file named foo
```
./httpc get -v 'http://127.0.0.1:8007/foo'
```

#### Create or overwrite the file named cool in the data directory
```
./httpc post -v -h Content-Type:application/json --d '{"Assignment": 100}' 'http://127.0.0.1:8007/cool'
```

#### Write to parent directory of current directory should be error
```
./httpc post -v -h Content-Type:application/json --d '{"Assignment": 100}' 'http://127.0.0.1:8007/../cool'
```
## Lab 3
#### Start router
```
./router --port=3000 --drop-rate=0.2 --max-delay=2s --seed=1
```

#### Post long data
```
./httpc post -h Content-Type:application/json --f ./sample2.txt "http://127.0.0.1:8007/sample2.txt"
```