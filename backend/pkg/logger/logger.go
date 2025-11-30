package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents log level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

// Logger is the main logger struct
type Logger struct {
	mu         sync.RWMutex
	level      Level
	output     io.Writer
	jsonFormat bool
	component  string
	fields     map[string]interface{}
	exitFunc   func(int)
}

// Config holds logger configuration
type Config struct {
	Level     string
	Format    string // "json" or "text"
	Output    io.Writer
	Component string
	ExitFunc  func(int) // For fatal logs, defaults to os.Exit
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger
func Init(config Config) error {
	var err error
	once.Do(func() {
		defaultLogger, err = New(config)
	})
	return err
}

// New creates a new logger instance
func New(config Config) (*Logger, error) {
	level := parseLevel(config.Level)
	output := config.Output
	if output == nil {
		output = os.Stdout
	}

	exitFunc := config.ExitFunc
	if exitFunc == nil {
		exitFunc = os.Exit
	}

	return &Logger{
		level:      level,
		output:     output,
		jsonFormat: strings.ToLower(config.Format) == "json",
		component:  config.Component,
		fields:     make(map[string]interface{}),
		exitFunc:   exitFunc,
	}, nil
}

// parseLevel parses log level from string
func parseLevel(levelStr string) Level {
	levelStr = strings.ToUpper(strings.TrimSpace(levelStr))
	switch levelStr {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		// Default to INFO
		return LevelInfo
	}
}

// WithComponent creates a new logger instance with a component name
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		level:      l.level,
		output:     l.output,
		jsonFormat: l.jsonFormat,
		component:  component,
		fields:     make(map[string]interface{}),
		exitFunc:   l.exitFunc,
	}
}

// WithField adds a field to the logger context
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		level:      l.level,
		output:     l.output,
		jsonFormat: l.jsonFormat,
		component:  l.component,
		fields:     make(map[string]interface{}),
		exitFunc:   l.exitFunc,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new field
	newLogger.fields[key] = value
	return newLogger
}

// WithFields adds multiple fields to the logger context
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		level:      l.level,
		output:     l.output,
		jsonFormat: l.jsonFormat,
		component:  l.component,
		fields:     make(map[string]interface{}),
		exitFunc:   l.exitFunc,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// log writes a log entry
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}

	if l.jsonFormat {
		l.logJSON(level, message)
	} else {
		l.logText(level, message)
	}
}

// logJSON writes a JSON formatted log entry
func (l *Logger) logJSON(level Level, message string) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     levelNames[level],
		Component: l.component,
		Message:   message,
		Fields:    l.fields,
	}

	if len(l.fields) == 0 {
		entry.Fields = nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to text logging if JSON marshal fails
		l.logText(level, message)
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	fmt.Fprintln(l.output, string(data))
}

// logText writes a text formatted log entry
func (l *Logger) logText(level Level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := levelNames[level]

	var prefix string
	if l.component != "" {
		prefix = fmt.Sprintf("[%s] [%s] %s", timestamp, levelName, l.component)
	} else {
		prefix = fmt.Sprintf("[%s] [%s]", timestamp, levelName)
	}

	// Add fields if any
	if len(l.fields) > 0 {
		var fieldStrs []string
		for k, v := range l.fields {
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%v", k, v))
		}
		prefix += " " + strings.Join(fieldStrs, " ")
	}

	l.mu.RLock()
	defer l.mu.RUnlock()
	fmt.Fprintf(l.output, "%s %s\n", prefix, message)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelFatal, format, args...)
	l.exitFunc(1)
}

// Debugf logs a debug message (alias for Debug)
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(format, args...)
}

// Infof logs an info message (alias for Info)
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(format, args...)
}

// Warnf logs a warning message (alias for Warn)
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(format, args...)
}

// Errorf logs an error message (alias for Error)
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(format, args...)
}

// Fatalf logs a fatal message and exits (alias for Fatal)
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Fatal(format, args...)
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// Package-level convenience functions that use the default logger

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	} else {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	} else {
		log.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, args...)
	} else {
		log.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	} else {
		log.Printf("[ERROR] "+format, args...)
	}
}

// Fatal logs a fatal message and exits using the default logger
func Fatal(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Fatal(format, args...)
	} else {
		log.Fatalf("[FATAL] "+format, args...)
	}
}

// Default returns the default logger instance
func Default() *Logger {
	if defaultLogger == nil {
		// Initialize with defaults if not initialized
		defaultLogger, _ = New(Config{
			Level:  "INFO",
			Format: "text",
		})
	}
	return defaultLogger
}
