# Toxnet

Tox botnet that uses the golang bindings for libtoxcore.  
https://github.com/TokTok/go-toxcore-c

![ToxNet](https://i.imgur.com/eoDjqMb.png?raw=true)


This project was created in two days and it was my first time using Golang.  
However, the code has been commented and is easy to understand.  
The way Toxnet works is by having a server that auto accepts incoming clients friend requests.  
The client send outgoing messages every 30 seconds to the server tox address, which allows the server to know which clients are currently online/active.  
Then each online machine is appended to an array, which is then called when you use the list online clients command.  
Just type help in the server console.

How to use
==========
* sudo apt install libtoxcore
* go get github.com/TokTok/go-toxcore-c
* go run server.go - C2
* go run client.go - Bot
