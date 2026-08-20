package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Log file location and rotation settings
const (
	LogDir        = "/tmp/gswitch"
	LogFileName   = "gswitch.log"
	LogMaxSize    = 5 * 1024 * 1024 // 5 MB
	LogMaxBackups = 3
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelError
)

// FileLogger handles file-based logging with rotation
type FileLogger struct {
	file        *os.File
	mu          sync.Mutex
	path        string
	maxSize     int64
	maxBackups  int
	currentSize int64
}

// NewFileLogger creates a new file logger with rotation support
func NewFileLogger() (*FileLogger, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(LogDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	path := filepath.Join(LogDir, LogFileName)

	// Open file for appending
	// #nosec G304 -- path is constructed from constants, not user input
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	return &FileLogger{
		file:        file,
		path:        path,
		maxSize:     LogMaxSize,
		maxBackups:  LogMaxBackups,
		currentSize: info.Size(),
	}, nil
}

// Write writes a log message with the specified level
func (l *FileLogger) Write(level LogLevel, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	levelStr := "DEBUG"
	switch level {
	case LogLevelInfo:
		levelStr = "INFO"
	case LogLevelError:
		levelStr = "ERROR"
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s [%-5s] %s\n", timestamp, levelStr, msg)

	n, err := l.file.WriteString(line)
	if err != nil {
		return
	}

	l.currentSize += int64(n)

	// Check if rotation is needed
	if l.currentSize >= l.maxSize {
		l.rotate()
	}
}

// rotate performs log file rotation
func (l *FileLogger) rotate() {
	if l.file != nil {
		l.file.Close()
	}

	// Remove oldest backup file
	oldestPath := fmt.Sprintf("%s.%d", l.path, l.maxBackups)
	_ = os.Remove(oldestPath)

	// Shift backup files: .2 -> .3, .1 -> .2, etc.
	// Errors are ignored as files may not exist
	for i := l.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.path, i)
		newPath := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	// Rename current log to .1
	_ = os.Rename(l.path, l.path+".1")

	// Create new log file
	// #nosec G304 -- path is constructed from constants, not user input
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		l.file = nil
		return
	}

	l.file = file
	l.currentSize = 0
}

// Close closes the log file
func (l *FileLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// Path returns the current log file path
func (l *FileLogger) Path() string {
	return l.path
}
