package main

type command struct {
	src string
	str string
	dat []byte
}

var cmdChannel chan *command = make(chan *command, 100)

