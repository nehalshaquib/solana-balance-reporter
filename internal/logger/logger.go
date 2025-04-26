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

	// Create filename based on current time with seconds precision
	now := time.Now().UTC()
	filename := fmt.Sprintf("activity_%s.log", now.Format("2006-01-02_15_04_05"))
	return l.openLogFile(filename)
}

// SetFilename sets a specific log filename and opens that file
func (l *Logger) SetFilename(filename string) error {
	// Close existing file if open
	if l.file != nil {
		l.file.Close()
	}

	return l.openLogFile(filename)
}

// openLogFile opens the specified log file
func (l *Logger) openLogFile(filename string) error {
	filepath := filepath.Join(l.logDir, filename)

	// Open or create the log file
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file
	return nil
}

// CheckRotation rotates the log file if needed
func (l *Logger) CheckRotation() error {
	// If file is nil, create a new one
	if l.file == nil {
		return l.rotateLogFile()
	}

	// We want to rotate logs every hour, check if we're at the start of an hour
	now := time.Now().UTC()

	// If we're within the first 10 seconds of an hour, rotate the log file
	if now.Minute() == 0 && now.Second() < 10 {
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
