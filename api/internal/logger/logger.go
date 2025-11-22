package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var (
	currentLevel LogLevel = INFO
	logger       *log.Logger
)

func init() {
	logger = log.New(os.Stdout, "", 0)

	// Set log level from environment
	levelStr := os.Getenv("BFM_LOG_LEVEL")
	switch levelStr {
	case "DEBUG", "debug":
		currentLevel = DEBUG
	case "INFO", "info":
		currentLevel = INFO
	case "WARN", "warn", "WARNING", "warning":
		currentLevel = WARN
	case "ERROR", "error":
		currentLevel = ERROR
	case "FATAL", "fatal":
		currentLevel = FATAL
	default:
		currentLevel = INFO
	}
}

// SetLevel sets the logging level
func SetLevel(level LogLevel) {
	currentLevel = level
}

// shouldLog checks if a message at the given level should be logged
func shouldLog(level LogLevel) bool {
	return level >= currentLevel
}

// formatMessage formats a log message with timestamp and level
func formatMessage(level string, format string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	if shouldLog(DEBUG) {
		logger.Println(formatMessage("DEBUG", format, args...))
	}
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	if shouldLog(INFO) {
		logger.Println(formatMessage("INFO", format, args...))
	}
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	if shouldLog(WARN) {
		logger.Println(formatMessage("WARN", format, args...))
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	if shouldLog(ERROR) {
		logger.Println(formatMessage("ERROR", format, args...))
	}
}

// Fatal logs a fatal message and exits
func Fatal(format string, args ...interface{}) {
	if shouldLog(FATAL) {
		logger.Fatalln(formatMessage("FATAL", format, args...))
	}
}

// Infof logs an info message with formatting
func Infof(format string, args ...interface{}) {
	Info(format, args...)
}

// Warnf logs a warning message with formatting
func Warnf(format string, args ...interface{}) {
	Warn(format, args...)
}

// Errorf logs an error message with formatting
func Errorf(format string, args ...interface{}) {
	Error(format, args...)
}

// Fatalf logs a fatal message with formatting and exits
func Fatalf(format string, args ...interface{}) {
	Fatal(format, args...)
}
