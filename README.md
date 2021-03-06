# Toxnet

C2 is written in Go and uses [go-toxcore-c](https://github.com/TokTok/go-toxcore-c).  
Client is written in C and uses [c-toxcore](https://github.com/TokTok/c-toxcore).

![ToxNet](https://i.imgur.com/ySV3ynY.png)

#### Description
This project is a proof-of-concept.   
Thanks to [xbee](https://github.com/xbee) and the developers of [TokTok](https://github.com/TokTok) this project was very simple to create.  
The Toxnet code has been commented and is pretty easy to understand.  
The way Toxnet works is by having a server that auto accepts incoming clients friend requests.   
Then uses the FriendGetConnectionStatus function to determine online status.   
Messages are executed as shell commands and the output is sent back to the C2.   
In order to get started just type help in the server console.  
You can find Tox bootstrap servers here: [nodes](https://nodes.tox.chat).  
Just update the server interface in the client and server.  


__Warning: Only use this software according to your current legislation. Misuse of this software can raise legal and ethical issues which I don't support nor can be held responsible for.__

How to use
==========

### Debian

* `sudo apt install libsodium-dev libtoxcore-dev golang -y`
* `go get github.com/TokTok/go-toxcore-c`
* `go run server.go`
* You will then see the C2 address and public key under the Toxnet banner
* Edit client.c and put the tox address and public key in client.c
* `gcc -o client client.c -I tox/include -I deps/file2str -lpthread -Bstatic /usr/lib/x86_64-linux-gnu/libtoxcore.a /usr/lib/x86_64-linux-gnu/libsodium.a -lrt -Bdynamic -lc -lm -lgcc -ldl -pthread`

### Windows
* Coming Soon

<img src="https://imgur.com/HFthlr9.png" alt="address" width="1000" height="auto">
