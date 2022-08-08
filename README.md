# Toxnet

C2 is written in Go and uses [go-toxcore-c](https://github.com/TokTok/go-toxcore-c).  
Client is written in C and uses [c-toxcore](https://github.com/TokTok/c-toxcore).

![ToxNet](https://i.imgur.com/ySV3ynY.png)

#### Description
Toxnet is a proof-of-concept [e2ee](https://en.wikipedia.org/wiki/End-to-end_encryption) [P2P](https://en.wikipedia.org/wiki/Peer-to-peer) [C2](https://en.wikipedia.org/wiki/Command_and_control).   
Thanks to [xbee](https://github.com/xbee) and the developers of [TokTok](https://github.com/TokTok), this project was very simple to create.  
The Toxnet code has been commented and is very easy to understand.  
This project works by setting up a C2 and having it act as a relay for admins and bots.  
Use a Tox client such as [qTox](https://qtox.github.io/) and place your Tox ID into "server.go" in the "admins" array on line 15.  
Upon starting the C2 (server.go), this will present a "TOX-ID", add this Tox ID using your Tox client.  
Once the server has accepted the incoming friend request and establishes a connection, send "help" as a message to the server.  
This will send instructions on how to use Toxnet.  

You can find Tox bootstrap servers here: [nodes](https://nodes.tox.chat).  
Just update the server interface in the client and server.  


__Warning: Only use this software according to your current legislation. Misuse of this software can raise legal and ethical issues which I don't support nor can be held responsible for.__

How to use
==========

### Debian

* `sudo apt install libsodium-dev libtoxcore-dev golang -y`
* `go get github.com/TokTok/go-toxcore-c`
* `go run server.go`
* You will then see the "TOX-ID" and "PUB-KEY" under the Toxnet banner.
* Edit client.c and put the "TOX-ID" and "PUB-KEY" in client.c.
* `gcc -o client client.c -I tox/include -I deps/file2str -lpthread -Bstatic /usr/lib/x86_64-linux-gnu/libtoxcore.a /usr/lib/x86_64-linux-gnu/libsodium.a -lrt -Bdynamic -lc -lm -lgcc -ldl -pthread`

### Windows
* Coming Soon

<img src="https://imgur.com/HFthlr9.png" alt="address" width="1000" height="auto">
