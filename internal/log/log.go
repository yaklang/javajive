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
	"sync/atomic"
)

// Level controls the minimum severity that will be printed.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	QuietLevel
)

var currentLevel atomic.Int32

func init() {
	stdlog.SetFlags(stdlog.LstdFlags)
	if lvl, err := ParseLevel(os.Getenv("JAVAJIVE_LOG_LEVEL")); err == nil {
		SetLevel(lvl)
	} else {
		SetLevel(InfoLevel)
	}
}

// ParseLevel parses a textual log level name (case-insensitive).
// Accepted values are "debug", "info", "warn"/"warning", "error", and
// "quiet"/"off"/"none".
func ParseLevel(raw string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "quiet", "off", "none":
		return QuietLevel, nil
	default:
		return InfoLevel, fmt.Errorf("invalid log level %q (want debug|info|warn|error|quiet)", raw)
	}
}

// GetLevel returns the current global minimum log level safely across goroutines.
func GetLevel() Level { return Level(currentLevel.Load()) }

// SetLevel adjusts the global minimum log level safely across goroutines.
func SetLevel(l Level) { currentLevel.Store(int32(l)) }

// Enabled reports whether messages at level l would be emitted.
func Enabled(l Level) bool { return l >= Level(currentLevel.Load()) }

func output(level Level, tag, msg string) {
	if !Enabled(level) {
		return
	}
	stdlog.Printf("[%s] %s", tag, msg)
}

func sprint(args ...interface{}) string         { return strings.TrimSuffix(fmt.Sprintln(args...), "\n") }
func sprintf(f string, a ...interface{}) string { return fmt.Sprintf(f, a...) }

// Debug-level helpers.
func Debug(args ...interface{}) {
	if !Enabled(DebugLevel) {
		return
	}
	output(DebugLevel, "DEBUG", sprint(args...))
}
func Debugf(format string, args ...interface{}) {
	if !Enabled(DebugLevel) {
		return
	}
	output(DebugLevel, "DEBUG", sprintf(format, args...))
}

// Info-level helpers.
func Info(args ...interface{}) {
	if !Enabled(InfoLevel) {
		return
	}
	output(InfoLevel, "INFO", sprint(args...))
}
func Infof(format string, args ...interface{}) {
	if !Enabled(InfoLevel) {
		return
	}
	output(InfoLevel, "INFO", sprintf(format, args...))
}

// Warn-level helpers.
func Warn(args ...interface{}) {
	if !Enabled(WarnLevel) {
		return
	}
	output(WarnLevel, "WARN", sprint(args...))
}
func Warnf(format string, args ...interface{}) {
	if !Enabled(WarnLevel) {
		return
	}
	output(WarnLevel, "WARN", sprintf(format, args...))
}

// Error-level helpers.
func Error(args ...interface{}) {
	if !Enabled(ErrorLevel) {
		return
	}
	output(ErrorLevel, "ERROR", sprint(args...))
}
func Errorf(format string, args ...interface{}) {
	if !Enabled(ErrorLevel) {
		return
	}
	output(ErrorLevel, "ERROR", sprintf(format, args...))
}

// Fatal helpers print at error level and terminate the process.
func Fatal(args ...interface{}) {
	stdlog.Printf("[FATAL] %s", sprint(args...))
	os.Exit(1)
}

func Fatalln(args ...interface{}) {
	stdlog.Printf("[FATAL] %s", sprint(args...))
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	stdlog.Printf("[FATAL] %s", sprintf(format, args...))
	os.Exit(1)
}
