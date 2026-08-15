package parser

import (
	"fmt"
	"strconv"
	"strings"
)

type Command struct {
	Name        string
	Key         string
	Value       string
	ValueLength int
	TTL         int
}

func Parse(line string) (*Command, error) {
	fields := strings.Fields(strings.TrimSpace(line))

	if len(fields) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	switch strings.ToUpper(fields[0]) {

	case "PING":
		return &Command{Name: "PING"}, nil

	case "STATUS":
		return &Command{Name: "STATUS"}, nil

	case "STATS":
		return &Command{Name: "STATS"}, nil

	case "GET":
		if len(fields) != 2 {
			return nil, fmt.Errorf("usage: GET <key>")
		}

		return &Command{
			Name: "GET",
			Key:  fields[1],
		}, nil

	case "DELETE":
		if len(fields) != 2 {
			return nil, fmt.Errorf("usage: DELETE <key>")
		}

		return &Command{
			Name: "DELETE",
			Key:  fields[1],
		}, nil

	case "REPLICA":
		// REPLICA command contains the full command line after REPLICA
		// We store it in Key field and parse it in the handler
		if len(fields) < 2 {
			return nil, fmt.Errorf("usage: REPLICA <command>")
		}

		// Join the rest as a single command
		replicaCmd := strings.Join(fields[1:], " ")
		
		// Parse the actual command
		actualFields := fields[1:]
		
		if strings.ToUpper(actualFields[0]) == "SET" {
			if len(actualFields) < 3 {
				return nil, fmt.Errorf("invalid replica SET command")
			}
			
			valueLength, err := strconv.Atoi(actualFields[2])
			if err != nil || valueLength < 0 {
				return nil, fmt.Errorf("invalid value length")
			}
			
			cmd := &Command{
				Name:        "REPLICA",
				Key:         replicaCmd,
				ValueLength: valueLength,
			}
			
			// Parse TTL if present
			for i := 3; i < len(actualFields)-1; i++ {
				if strings.ToUpper(actualFields[i]) == "EX" {
					ttl, err := strconv.Atoi(actualFields[i+1])
					if err == nil {
						cmd.TTL = ttl
					}
					break
				}
			}
			
			return cmd, nil
		} else if strings.ToUpper(actualFields[0]) == "DELETE" {
			return &Command{
				Name: "REPLICA",
				Key:  replicaCmd,
			}, nil
		}
		
		return nil, fmt.Errorf("unsupported replica command")

	case "SET":

		if len(fields) < 3 {
			return nil, fmt.Errorf("usage: SET <key> <value-length> [EX seconds]")
		}

		valueLength, err := strconv.Atoi(fields[2])
		if err != nil || valueLength < 0 {
			return nil, fmt.Errorf("invalid value length")
		}

		cmd := &Command{
			Name:        "SET",
			Key:         fields[1],
			ValueLength: valueLength,
		}

		if len(fields) != 3 && len(fields) != 5 {
			return nil, fmt.Errorf("usage: SET <key> <value-length> [EX seconds]")
		}

		if len(fields) == 5 {
			if strings.ToUpper(fields[3]) != "EX" {
				return nil, fmt.Errorf("expected EX")
			}

			ttl, err := strconv.Atoi(fields[4])
			if err != nil {
				return nil, fmt.Errorf("invalid ttl")
			}

			cmd.TTL = ttl
		}

		return cmd, nil

	default:
		return nil, fmt.Errorf("unknown command")
	}
}
