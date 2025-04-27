package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger handles application logging
type Logger struct {
	file      *os.File
	dir       string
	filename  string
	mu        sync.Mutex // Mutex to guard file operations
	isInitial bool       // Track if this is initial creation
}

// New creates a new logger
func New(logsDir string) (*Logger, error) {
	// Ensure logs directory exists
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create default log file (startup log)
	startupFilename := fmt.Sprintf("startup_%s.log", time.Now().UTC().Format("2006-01-02_15_04_05"))

	// Return logger without opening file initially
	return &Logger{
		dir:       logsDir,
		filename:  startupFilename,
		isInitial: true,
	}, nil
}

// getLogFilePath returns the full path to the log file
func (l *Logger) getLogFilePath() string {
	return filepath.Join(l.dir, l.filename)
}

// ensureLogFileOpen ensures the log file is open and ready for writing
func (l *Logger) ensureLogFileOpen() error {
	// If file is already open, return
	if l.file != nil {
		return nil
	}

	// Make sure directory exists
	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create or open log file
	filePath := l.getLogFilePath()
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = f

	// For first log file, write startup header
	if l.isInitial {
		l.isInitial = false
		l.file.WriteString(fmt.Sprintf("=== Logging started at %s ===\n",
			time.Now().UTC().Format("2006-01-02 15:04:05")))
	}

	return nil
}

// SetFilename changes the log file
func (l *Logger) SetFilename(filename string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// If this is the same filename, don't change anything
	if l.filename == filename {
		return nil
	}

	// Close existing file if open
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return fmt.Errorf("failed to close old log file: %w", err)
		}
		l.file = nil
	}

	// Update filename
	l.filename = filename

	// Ensure the new log file exists and is open
	return l.ensureLogFileOpen()
}

// Close closes the log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}

	return nil
}

// Log logs a message
func (l *Logger) Log(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Make sure file is open
	if err := l.ensureLogFileOpen(); err != nil {
		fmt.Printf("Error ensuring log file is open: %v\n", err)
		return
	}

	// Format message with timestamp
	now := time.Now().UTC()
	logLine := fmt.Sprintf("[%s] INFO: %s\n", now.Format("2006-01-02 15:04:05"), message)

	// Write to file
	if _, err := l.file.WriteString(logLine); err != nil {
		fmt.Printf("Error writing to log file: %v\n", err)
	}

	// Also print to stdout
	fmt.Print(logLine)
}

// LogError logs an error message
func (l *Logger) LogError(message string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Make sure file is open
	if openErr := l.ensureLogFileOpen(); openErr != nil {
		fmt.Printf("Error ensuring log file is open: %v\n", openErr)
		return
	}

	// Format error message with timestamp
	now := time.Now().UTC()
	logLine := fmt.Sprintf("[%s] ERROR: %s: %v\n", now.Format("2006-01-02 15:04:05"), message, err)

	// Write to file
	if _, writeErr := l.file.WriteString(logLine); writeErr != nil {
		fmt.Printf("Error writing to log file: %v\n", writeErr)
	}

	// Also print to stderr
	fmt.Fprint(os.Stderr, logLine)
}

// Sync flushes the file
func (l *Logger) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Sync()
	}

	return nil
}
