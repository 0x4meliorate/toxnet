# Toxnet
<p align="center">
  <img src="https://i.imgur.com/sMbpdVJ.gif" width="650" height="auto">
</p>


#### Description
Toxnet is a proof-of-concept [E2EE](https://en.wikipedia.org/wiki/End-to-end_encryption) [P2P](https://en.wikipedia.org/wiki/Peer-to-peer) [C2](https://en.wikipedia.org/wiki/Command_and_control).   
Thanks to [xbee](https://github.com/xbee) and the developers of [TokTok](https://github.com/TokTok) this project was simple to create.  
The Toxnet code has been commented and is very easy to understand.  
This project works by setting up a C2 and having it act as a relay for incoming and outgoing messages.  
A better explanation and more features are yet to come! It's getting late, I'll do this tomorrow. 😅

__Warning: Only use this software according to your current legislation. Misuse of this software can raise legal and ethical issues which I don't support nor can be held responsible for.__

C2 is written in Go and uses [go-toxcore-c](https://github.com/TokTok/go-toxcore-c).  
Client is written in C and uses [c-toxcore](https://github.com/TokTok/c-toxcore).

Setup
==========
Installation is straight forward on Debian-based Linux distributions:
* `sudo apt install libsodium-dev libtoxcore-dev golang -y`
* `go get github.com/TokTok/go-toxcore-c`
* Download [qTox](https://qtox.github.io/)
* Place your Tox-ID (from qTox) into `net/config.go` in the "Admins" array on line 4. [View](https://github.com/0x4meliorate/toxnet/blob/132b719d250f8a9a0448c09e4f0d882ff047db83/net/config.go#L4)
* Run `go run main.go`
* Upon starting the C2, this will present a "TOX-ID", add this Tox-ID using qTox.
* Once the server has accepted the incoming friend request and establishes a connection, send "help" as a message to the server.
* This will send instructions on how to use Toxnet. [Example](https://i.imgur.com/EgDxnDi.png)

###### Generate Linux payload: `go run main.go -t linux`
