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
./httpc get 'http://httpbin.org/get?course=networking&assignment=1'
```
#### Get with verbose option
```
./httpc get -v 'http://httpbin.org/get?course=networking&assignment=1'
```
#### Get with mutiple headers
```
./httpc get -v -h User-Agent:Concordia-HTTP/1.0 -h Host:httpbin.org 'http://httpbin.org/get?course=networking&assignment=1'
```
#### Post with inline data
```
./httpc post -h Content-Type:application/json --d '{"Assignment": 1}' http://httpbin.org/post
```
#### Post with data file
```
./httpc post -h Content-Type:application/json --f ./data_file.json http://httpbin.org/post
```