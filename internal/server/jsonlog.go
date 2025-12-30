// jsonlog.go - Structured JSON logging for production environments
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Logger provides structured JSON logging
type Logger struct {
	output     io.Writer
	minLevel   LogLevel
	enableJSON bool
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Level     LogLevel               `json:"level"`
	Time      string                 `json:"time"`
	Message   string                 `json:"msg"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
}

var (
	// DefaultLogger is the global logger instance
	DefaultLogger *Logger
)

func init() {
	// Initialize with default logger (JSON enabled in production)
	enableJSON := os.Getenv("SFD_LOG_FORMAT") == "json"
	if enableJSON || os.Getenv("SFD_ENV") == "production" {
		enableJSON = true
	}

	DefaultLogger = &Logger{
		output:     os.Stdout,
		minLevel:   getLogLevel(),
		enableJSON: enableJSON,
	}
}

// getLogLevel returns the configured log level from environment
func getLogLevel() LogLevel {
	level := os.Getenv("SFD_LOG_LEVEL")
	switch level {
	case "debug":
		return LogLevelDebug
	case "info":
		return LogLevelInfo
	case "warn":
		return LogLevelWarn
	case "error":
		return LogLevelError
	default:
		return LogLevelInfo
	}
}

// shouldLog checks if a message at the given level should be logged
func (l *Logger) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
	}

	return levels[level] >= levels[l.minLevel]
}

// getCaller returns the file and line number of the caller
func getCaller(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}

	// Shorten file path
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			file = file[i+1:]
			break
		}
	}

	return fmt.Sprintf("%s:%d", file, line)
}

// log writes a log entry
func (l *Logger) log(level LogLevel, msg string, fields map[string]interface{}, err error) {
	if !l.shouldLog(level) {
		return
	}

	entry := LogEntry{
		Level:   level,
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: msg,
		Fields:  fields,
		Caller:  getCaller(3),
	}

	if err != nil {
		entry.Error = err.Error()
	}

	if l.enableJSON {
		data, _ := json.Marshal(entry)
		fmt.Fprintln(l.output, string(data))
	} else {
		// Plain text format for development
		fmt.Fprintf(l.output, "[%s] %s %s", entry.Level, entry.Time, entry.Message)
		if len(entry.Fields) > 0 {
			for k, v := range entry.Fields {
				fmt.Fprintf(l.output, " %s=%v", k, v)
			}
		}
		if entry.Error != "" {
			fmt.Fprintf(l.output, " error=%s", entry.Error)
		}
		fmt.Fprintln(l.output)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields map[string]interface{}) {
	l.log(LogLevelDebug, msg, fields, nil)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields map[string]interface{}) {
	l.log(LogLevelInfo, msg, fields, nil)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields map[string]interface{}) {
	l.log(LogLevelWarn, msg, fields, nil)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields map[string]interface{}, err error) {
	l.log(LogLevelError, msg, fields, err)
}

// Global logging functions

// Debug logs a debug message
func Debug(msg string, fields map[string]interface{}) {
	DefaultLogger.Debug(msg, fields)
}

// Info logs an info message
func Info(msg string, fields map[string]interface{}) {
	DefaultLogger.Info(msg, fields)
}

// Warn logs a warning message
func Warn(msg string, fields map[string]interface{}) {
	DefaultLogger.Warn(msg, fields)
}

// Error logs an error message
func Error(msg string, fields map[string]interface{}, err error) {
	DefaultLogger.Error(msg, fields, err)
}
