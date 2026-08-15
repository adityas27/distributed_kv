package observability

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type Logger struct {
	nodeID      string
	minLevel    LogLevel
	jsonOutput  bool
	fileLogger  *log.Logger
	enableFile  bool
}

type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	NodeID    string                 `json:"node_id"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func NewLogger(nodeID string, minLevel LogLevel, jsonOutput bool) *Logger {
	return &Logger{
		nodeID:     nodeID,
		minLevel:   minLevel,
		jsonOutput: jsonOutput,
		enableFile: false,
	}
}

func (l *Logger) EnableFileLogging(filepath string) error {
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.fileLogger = log.New(file, "", 0)
	l.enableFile = true
	return nil
}

func (l *Logger) log(level LogLevel, message string, fields map[string]interface{}) {
	if level < l.minLevel {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level.String(),
		NodeID:    l.nodeID,
		Message:   message,
		Fields:    fields,
	}

	var output string
	if l.jsonOutput {
		data, _ := json.Marshal(entry)
		output = string(data)
	} else {
		fieldsStr := ""
		if len(fields) > 0 {
			fieldsStr = fmt.Sprintf(" %v", fields)
		}
		output = fmt.Sprintf("[%s] %s [%s] %s%s",
			entry.Timestamp, entry.Level, entry.NodeID, entry.Message, fieldsStr)
	}

	// Write to stdout
	fmt.Println(output)

	// Write to file if enabled
	if l.enableFile && l.fileLogger != nil {
		l.fileLogger.Println(output)
	}
}

func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.log(DEBUG, message, fields)
}

func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log(INFO, message, fields)
}

func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.log(WARN, message, fields)
}

func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.log(ERROR, message, fields)
}

// Convenience methods without fields
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...), nil)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...), nil)
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...), nil)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...), nil)
}
