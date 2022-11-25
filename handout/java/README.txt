# UDP Example in Java

## Requirement
1. [Oracle JDK 8](http://www.oracle.com/technetwork/java/javase/downloads/jdk8-downloads-2133151.html)
2. [Apache Maven](https://maven.apache.org/) 

## Usage

1. Compile and package jar 
   mvn package

2. Run the router (see router's README)

3. Run the echo server
   java -cp .:target/udp-echo-1.0-SNAPSHOT-jar-with-dependencies.jar ca.concordia.UDPServer --port 8007

4. Run the echo client
   java -cp .:target/udp-echo-1.0-SNAPSHOT-jar-with-dependencies.jar ca.concordia.UDPClient \
   --router-host localhost \
   --router-port 3000 \
   --server-host localhost \
   --server-port 8007 \
