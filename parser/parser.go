package parser

import (
	"fmt"
	"strings"
)

type Command struct {
	Name  string
	Key   string
	Value string
}

func Parse(line string) (*Command, error) {
	fields := strings.Fields(strings.TrimSpace(line))

	if len(fields) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	switch strings.ToUpper(fields[0]) {

	case "PING":
		if len(fields) != 1 {
			return nil, fmt.Errorf("usage: PING")
		}

		return &Command{
			Name: "PING",
		}, nil

	case "GET":
		if len(fields) != 2 {
			return nil, fmt.Errorf("usage: GET <key>")
		}

		return &Command{
			Name: "GET",
			Key:  fields[1],
		}, nil

	case "SET":
		if len(fields) < 3 {
			return nil, fmt.Errorf("usage: SET <key> <value>")
		}

		return &Command{
			Name:  "SET",
			Key:   fields[1],
			Value: strings.Join(fields[2:], " "),
		}, nil

	case "DELETE":
		if len(fields) != 2 {
			return nil, fmt.Errorf("usage: DELETE <key>")
		}

		return &Command{
			Name: "DELETE",
			Key:  fields[1],
		}, nil

	default:
		return nil, fmt.Errorf("unknown command")
	}
}