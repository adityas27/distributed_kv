package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"

	"tcp_test/parser"
)

type Server struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewServer() *Server {
	return &Server{
		store: make(map[string]string),
	}
}

func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	fmt.Println("Listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Println("Client connected:", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := scanner.Text()

		cmd, err := parser.Parse(line)
		if err != nil {
			fmt.Fprintln(conn, "ERROR", err.Error())
			continue
		}

		response := s.execute(cmd)

		_, err = fmt.Fprintln(conn, response)
		if err != nil {
			return
		}
	}
}

func (s *Server) execute(cmd *parser.Command) string {
	switch cmd.Name {

	case "PING":
		return "PONG"

	case "SET":
		s.mu.Lock()
		s.store[cmd.Key] = cmd.Value
		s.mu.Unlock()

		return "OK"

	case "GET":
		s.mu.RLock()
		value, ok := s.store[cmd.Key]
		s.mu.RUnlock()

		if !ok {
			return "NULL"
		}

		return value

	case "DELETE":
		s.mu.Lock()
		delete(s.store, cmd.Key)
		s.mu.Unlock()

		return "OK"

	default:
		return "ERROR unknown command"
	}
}

func main() {
	server := NewServer()

	if err := server.Start(":9000"); err != nil {
		panic(err)
	}
}