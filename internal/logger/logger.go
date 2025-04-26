package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger represents a simple file logger
type Logger struct {
	logDir string
	file   *os.File
}

// New creates a new logger with the given log directory
func New(logDir string) (*Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger := &Logger{
		logDir: logDir,
	}

	// Open or create the current log file
	if err := logger.rotateLogFile(); err != nil {
		return nil, err
	}

	return logger, nil
}

// rotateLogFile closes the current log file and opens a new one
func (l *Logger) rotateLogFile() error {
	// Close existing file if open
	if l.file != nil {
		l.file.Close()
	}

	// Create filename based on current time
	now := time.Now().UTC()
	filename := fmt.Sprintf("activity_%s.log", now.Format("2006-01-02_15"))
	filepath := filepath.Join(l.logDir, filename)

	// Open or create the log file
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file
	return nil
}

// CheckRotation rotates the log file if the hour has changed
func (l *Logger) CheckRotation() error {
	now := time.Now().UTC()
	currentHour := now.Format("2006-01-02_15")

	// Extract current log filename
	if l.file == nil {
		return l.rotateLogFile()
	}

	filename := filepath.Base(l.file.Name())
	expectedPrefix := fmt.Sprintf("activity_%s", currentHour)

	// If filename doesn't match current hour, rotate
	if len(filename) < len(expectedPrefix) || filename[:len(expectedPrefix)] != expectedPrefix {
		return l.rotateLogFile()
	}

	return nil
}

// Log writes a log entry with timestamp
func (l *Logger) Log(message string) error {
	if err := l.CheckRotation(); err != nil {
		return err
	}

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

	_, err := l.file.WriteString(logEntry)
	return err
}

// LogError logs an error with timestamp
func (l *Logger) LogError(message string, err error) error {
	errMsg := fmt.Sprintf("%s: %v", message, err)
	return l.Log(errMsg)
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
