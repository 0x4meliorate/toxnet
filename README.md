# Toxnet

Tox botnet that uses the golang bindings for libtoxcore.  
This project is still in alpha and is still a little buggy.  
https://github.com/TokTok/go-toxcore-c

![ToxNet](https://i.imgur.com/eoDjqMb.png?raw=true)

#### Description
This project was created in two days and it was my first time using Golang.  
However, thanks to [xbee](https://github.com/xbee) and the developers of [TokTok](https://github.com/TokTok) this project was very simple to create.  
The Toxnet code has been commented and is pretty easy to understand.  
The way Toxnet works is by having a server that auto accepts incoming clients friend requests.  
The client sends outgoing messages every 30 seconds to the server tox address, which allows the server to know which clients are currently online/active.  
Then each online machine is appended to an array, which is then called when you use the list online clients command.  
In order to get started just type help in the server console.
You can find Tox bootstrap servers here: [nodes](https://nodes.tox.chat).  
Just update the server interface in the client and server.  


__Warning: Only use this software according to your current legislation. Misuse of this software can raise legal and ethical issues which I don't support nor can be held responsible for.__

How to use
==========
* sudo apt install libtoxcore
* go get github.com/TokTok/go-toxcore-c
* go run server.go
* You will then see the C2 address at the top
* Edit client.go and input the address in client.go
* go run client.go
<img src="https://i.imgur.com/M4rURRO.png" alt="address" width="1000" height="auto">
