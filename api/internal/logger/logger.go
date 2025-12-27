package logger

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

var (
	log *logrus.Logger
)

func init() {
	log = logrus.New()
	log.SetOutput(os.Stdout)

	// Set log level from environment
	levelStr := os.Getenv("BFM_LOG_LEVEL")
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		log.SetLevel(logrus.DebugLevel)
	case "INFO":
		log.SetLevel(logrus.InfoLevel)
	case "WARN", "WARNING":
		log.SetLevel(logrus.WarnLevel)
	case "ERROR":
		log.SetLevel(logrus.ErrorLevel)
	case "FATAL":
		log.SetLevel(logrus.FatalLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	// Set log format from environment (default to JSON)
	formatStr := os.Getenv("BFM_LOG_FORMAT")
	switch strings.ToLower(formatStr) {
	case "plaintext", "plain", "text":
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		// Default to JSON if not specified
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	}
}

// LogLevel represents the logging level (kept for backward compatibility)
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// LogFormat represents the logging format (kept for backward compatibility)
type LogFormat int

const (
	FormatJSON LogFormat = iota
	FormatPlaintext
)

// SetLevel sets the logging level
func SetLevel(level LogLevel) {
	switch level {
	case DEBUG:
		log.SetLevel(logrus.DebugLevel)
	case INFO:
		log.SetLevel(logrus.InfoLevel)
	case WARN:
		log.SetLevel(logrus.WarnLevel)
	case ERROR:
		log.SetLevel(logrus.ErrorLevel)
	case FATAL:
		log.SetLevel(logrus.FatalLevel)
	}
}

// SetFormat sets the logging format
func SetFormat(format LogFormat) {
	switch format {
	case FormatJSON:
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	case FormatPlaintext:
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}
}

// Debug logs a debug message
func Debug(format string, args ...interface{}) {
	if len(args) > 0 {
		log.Debug(fmt.Sprintf(format, args...))
	} else {
		log.Debug(format)
	}
}

// Info logs an info message
func Info(format string, args ...interface{}) {
	if len(args) > 0 {
		log.Info(fmt.Sprintf(format, args...))
	} else {
		log.Info(format)
	}
}

// Warn logs a warning message
func Warn(format string, args ...interface{}) {
	if len(args) > 0 {
		log.Warn(fmt.Sprintf(format, args...))
	} else {
		log.Warn(format)
	}
}

// Error logs an error message
func Error(format string, args ...interface{}) {
	if len(args) > 0 {
		log.Error(fmt.Sprintf(format, args...))
	} else {
		log.Error(format)
	}
}

// Fatal logs a fatal message and exits
func Fatal(format string, args ...interface{}) {
	if len(args) > 0 {
		log.Fatal(fmt.Sprintf(format, args...))
	} else {
		log.Fatal(format)
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
