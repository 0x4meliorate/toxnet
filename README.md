# Toxnet

Tox botnet that uses the golang bindings for libtoxcore.  
https://github.com/TokTok/go-toxcore-c

![ToxNet](https://i.imgur.com/eoDjqMb.png?raw=true)


This project was created in two days and it was my first time using Golang.  
However, the code has been commented and is easy to understand.  
The way Toxnet works is by having a server that auto accepts incoming clients friend requests.  
The client sends outgoing messages every 30 seconds to the server tox address, which allows the server to know which clients are currently online/active.  
Then each online machine is appended to an array, which is then called when you use the list online clients command.  
In order to get started just type help in the server console.

How to use
==========
* sudo apt install libtoxcore
* go get github.com/TokTok/go-toxcore-c
* go run server.go - C2
* You will then see the C2 address at the top
* Edit client.go and input the address in client.go
* go run client.go - Bot
<img src="https://i.imgur.com/M4rURRO.png" alt="address" width="1000" height="auto">

