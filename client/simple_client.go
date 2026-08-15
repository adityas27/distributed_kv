package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		address = flag.String("addr", "localhost:5000", "Server address")
		command = flag.String("cmd", "", "Command to execute (leave empty for interactive mode)")
	)
	flag.Parse()

	if *command != "" {
		// Single command mode
		result, err := executeCommand(*address, *command)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result)
		return
	}

	// Interactive mode
	fmt.Printf("Connected to %s\n", *address)
	fmt.Println("Commands: SET, GET, DELETE, PING, STATUS, STATS")
	fmt.Println("Type 'quit' or 'exit' to close")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if strings.ToLower(input) == "quit" || strings.ToLower(input) == "exit" {
			fmt.Println("Goodbye!")
			break
		}

		result, err := executeCommand(*address, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Println(result)
	}
}

func executeCommand(address, cmd string) (string, error) {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmdType := strings.ToUpper(parts[0])

	switch cmdType {
	case "SET":
		if len(parts) < 3 {
			return "", fmt.Errorf("usage: SET <key> <value> [EX <seconds>]")
		}

		key := parts[1]
		value := parts[2]
		var ttl string

		// Check for EX parameter
		if len(parts) >= 5 && strings.ToUpper(parts[3]) == "EX" {
			ttl = parts[4]
		}

		// Send SET command
		if ttl != "" {
			fmt.Fprintf(conn, "SET %s %d EX %s\n", key, len(value), ttl)
		} else {
			fmt.Fprintf(conn, "SET %s %d\n", key, len(value))
		}

		// Send value
		fmt.Fprintln(conn, value)

		// Read response
		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	case "GET":
		if len(parts) != 2 {
			return "", fmt.Errorf("usage: GET <key>")
		}

		fmt.Fprintf(conn, "GET %s\n", parts[1])

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	case "DELETE":
		if len(parts) != 2 {
			return "", fmt.Errorf("usage: DELETE <key>")
		}

		fmt.Fprintf(conn, "DELETE %s\n", parts[1])

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	case "PING":
		fmt.Fprintln(conn, "PING")

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	case "STATUS":
		fmt.Fprintln(conn, "STATUS")

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	case "STATS":
		fmt.Fprintln(conn, "STATS")

		reader := bufio.NewReader(conn)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(response), nil

	default:
		return "", fmt.Errorf("unknown command: %s", cmdType)
	}
}
