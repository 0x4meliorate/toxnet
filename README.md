# Toxnet
<p align="center">
  <img src="https://i.imgur.com/sMbpdVJ.gif" width="650" height="auto">
</p>


#### Description
Toxnet is a proof-of-concept [E2EE](https://en.wikipedia.org/wiki/End-to-end_encryption) [P2P](https://en.wikipedia.org/wiki/Peer-to-peer) [C2](https://en.wikipedia.org/wiki/Command_and_control).   
Thanks to [xbee](https://github.com/xbee) and the developers of [TokTok](https://github.com/TokTok) this project was simple to create.  
The Toxnet code has been commented and is very easy to understand.  
This project works by setting up a C2 and having it act as a relay for incoming and outgoing messages.  

You can find Tox bootstrap servers here: [nodes](https://nodes.tox.chat).  
Just update the server interface in the client and server.  


__Warning: Only use this software according to your current legislation. Misuse of this software can raise legal and ethical issues which I don't support nor can be held responsible for.__

C2 is written in Go and uses [go-toxcore-c](https://github.com/TokTok/go-toxcore-c).  
Client is written in C and uses [c-toxcore](https://github.com/TokTok/c-toxcore).

How to use
==========

#### Debian
##### Server
* `sudo apt install libsodium-dev libtoxcore-dev golang -y`
* `go get github.com/TokTok/go-toxcore-c`
* Download [qTox](https://qtox.github.io/)
* Place your Tox ID (from qTox) into "server.go" in the "admins" array on line 15. [Example](https://i.imgur.com/N4mq8Jf.png)
* `go run server.go`
* Upon starting the C2, this will present a "TOX-ID", add this Tox ID using qTox.
* Once the server has accepted the incoming friend request and establishes a connection, send "help" as a message to the server.
* This will send instructions on how to use Toxnet. [Example](https://i.imgur.com/EgDxnDi.png)
##### Client
* Get the "TOX-ID" and "PUB-KEY" under the Toxnet banner on the server.
* Edit client.c and change `c2id = "TOX-ID"` and `c2pub="PUB-KEY"`). [Example](https://i.imgur.com/N4mq8Jf.png)
* Then compile using the command below.
* `gcc -o client client.c -I tox/include -I deps/file2str -lpthread -Bstatic /usr/lib/x86_64-linux-gnu/libtoxcore.a /usr/lib/x86_64-linux-gnu/libsodium.a -lrt -Bdynamic -lc -lm -lgcc -ldl -pthread`
<img src="https://i.imgur.com/3IG44mm.png" alt="address" width="750" height="auto">


#### Windows
* Coming Soon



