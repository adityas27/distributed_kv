package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {
	ln, err := net.Listen("tcp", ":9000")
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Println("Listening on :9000")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("Client connected:", conn.RemoteAddr())
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Printf("[%s] %s\n", conn.RemoteAddr(), msg)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("read error:", err)
	}

	fmt.Println("Client disconnected:", conn.RemoteAddr())
}