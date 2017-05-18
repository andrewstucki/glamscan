package main

import (
	"io"
	"log"
)

// Logger is a simple logger that implements custom logging and can theoretically write to file
// assuming a file is passed in as an io.Writer.
type Logger struct {
	debugEnabled bool

	debugLog   *log.Logger
	infoLog    *log.Logger
	printLog   *log.Logger // like info except no logging info
	warningLog *log.Logger
	errorLog   *log.Logger
}

// NewLogger returns a new instance of Logger
func NewLogger(debugEnabled bool, debugOut, infoOut, warningOut, errorOut io.Writer) *Logger {
	return &Logger{
		debugEnabled: debugEnabled,
		debugLog:     log.New(debugOut, "[DEBUG] ", log.Ldate|log.Ltime),
		infoLog:      log.New(infoOut, "[INFO] ", log.Ldate|log.Ltime),
		printLog:     log.New(infoOut, "", 0),
		warningLog:   log.New(warningOut, "[WARN] ", log.Ldate|log.Ltime),
		errorLog:     log.New(errorOut, "[ERROR] ", log.Ldate|log.Ltime),
	}
}

// Debug logs only if debug logging is enabled
func (l *Logger) Debug(message string, args ...interface{}) {
	if l.debugEnabled {
		l.debugLog.Printf(message, args...)
	}
}

// Info always logs
func (l *Logger) Info(message string, args ...interface{}) {
	l.infoLog.Printf(message, args...)
}

// Print always logs
func (l *Logger) Print(message string, args ...interface{}) {
	l.printLog.Printf(message, args...)
}

// Warn always logs
func (l *Logger) Warn(message string, args ...interface{}) {
	l.warningLog.Printf(message, args...)
}

// Error always logs
func (l *Logger) Error(message string, args ...interface{}) {
	l.errorLog.Printf(message, args...)
}
