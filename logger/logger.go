package logger

import (
	"fmt"
	"log"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level represents logging levels
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Global logging configuration
var currentLevel Level = INFO

// Per-level file loggers (nil until InitLogFiles is called)
var fileLoggers [4]*log.Logger // indexed by Level: DEBUG=0, INFO=1, WARN=2, ERROR=3

// InitLogFiles creates the logs directory and opens one log file per level with rotation.
// Each file is appended to across runs and contains a timestamp prefix per line.
// Uses lumberjack for automatic log rotation (100MB max size, 3 backups, 28 days retention).
func InitLogFiles() error {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		return fmt.Errorf("could not create logs directory: %w", err)
	}

	names := [4]string{"logs/debug.log", "logs/info.log", "logs/warn.log", "logs/error.log"}
	for i, name := range names {
		logger := &lumberjack.Logger{
			Filename:   name,
			MaxSize:    100, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   // days
			Compress:   true, // compress old logs
		}
		fileLoggers[i] = log.New(logger, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	}
	return nil
}

// CloseLogFiles flushes and closes all open log files.
// Lumberjack handles file lifecycle, so nothing is required here but the
// function is kept for symmetry and future extension.
func CloseLogFiles() {
	// No-op: lumberjack handles file closing via GC.
}

// SetLevel configures the global log level from a string, defaulting to INFO on unknown values.
func SetLevel(level string) {
	currentLevel = parseLevel(level)
}

// parseLevel converts string to Level
func parseLevel(level string) Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Debug logs a debug-level message.
func Debug(format string, args ...interface{}) {
	if fileLoggers[DEBUG] != nil {
		fileLoggers[DEBUG].Printf("[DEBUG] "+format, args...)
	}
	if currentLevel <= DEBUG {
		fmt.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info-level message.
func Info(format string, args ...interface{}) {
	if fileLoggers[INFO] != nil {
		fileLoggers[INFO].Printf("[INFO] "+format, args...)
	}
	if currentLevel <= INFO {
		fmt.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning-level message.
func Warn(format string, args ...interface{}) {
	if fileLoggers[WARN] != nil {
		fileLoggers[WARN].Printf("[WARN] "+format, args...)
	}
	if currentLevel <= WARN {
		fmt.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error-level message.
func Error(format string, args ...interface{}) {
	if fileLoggers[ERROR] != nil {
		fileLoggers[ERROR].Printf("[ERROR] "+format, args...)
	}
	if currentLevel <= ERROR {
		fmt.Printf("[ERROR] "+format, args...)
	}
}
