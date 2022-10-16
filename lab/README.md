## Usage Examples

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