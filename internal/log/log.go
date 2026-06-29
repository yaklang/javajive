// Package log is a minimal, self-contained logging shim that mirrors the small
// subset of the original yaklang logging API used by the ported Java tooling.
//
// It deliberately avoids any third-party logging dependency: output goes to the
// standard library logger. All log messages are emitted in English.
package log

import (
	"fmt"
	stdlog "log"
	"os"
	"strings"
)

// Level controls the minimum severity that will be printed.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

var currentLevel = InfoLevel

func init() {
	stdlog.SetFlags(stdlog.LstdFlags)
	switch strings.ToLower(os.Getenv("JAVAJIVE_LOG_LEVEL")) {
	case "debug":
		currentLevel = DebugLevel
	case "warn", "warning":
		currentLevel = WarnLevel
	case "error":
		currentLevel = ErrorLevel
	default:
		currentLevel = InfoLevel
	}
}

// SetLevel adjusts the global minimum log level.
func SetLevel(l Level) { currentLevel = l }

func output(level Level, tag, msg string) {
	if level < currentLevel {
		return
	}
	stdlog.Printf("[%s] %s", tag, msg)
}

func sprint(args ...interface{}) string         { return strings.TrimSuffix(fmt.Sprintln(args...), "\n") }
func sprintf(f string, a ...interface{}) string { return fmt.Sprintf(f, a...) }

// Debug-level helpers.
func Debug(args ...interface{}) { output(DebugLevel, "DEBUG", sprint(args...)) }
func Debugf(format string, args ...interface{}) {
	output(DebugLevel, "DEBUG", sprintf(format, args...))
}

// Info-level helpers.
func Info(args ...interface{})                 { output(InfoLevel, "INFO", sprint(args...)) }
func Infof(format string, args ...interface{}) { output(InfoLevel, "INFO", sprintf(format, args...)) }

// Warn-level helpers.
func Warn(args ...interface{})                 { output(WarnLevel, "WARN", sprint(args...)) }
func Warnf(format string, args ...interface{}) { output(WarnLevel, "WARN", sprintf(format, args...)) }

// Error-level helpers.
func Error(args ...interface{}) { output(ErrorLevel, "ERROR", sprint(args...)) }
func Errorf(format string, args ...interface{}) {
	output(ErrorLevel, "ERROR", sprintf(format, args...))
}

// Fatal helpers print at error level and terminate the process.
func Fatal(args ...interface{}) {
	output(ErrorLevel, "FATAL", sprint(args...))
	os.Exit(1)
}

func Fatalln(args ...interface{}) {
	output(ErrorLevel, "FATAL", sprint(args...))
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	output(ErrorLevel, "FATAL", sprintf(format, args...))
	os.Exit(1)
}
