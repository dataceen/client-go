package dataceen

import (
	"fmt"
	"os"
)

// LogLevel is one of debug/info/warn/error.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Logger is the interface the client uses for all internal logging. It mirrors
// the TS port's Logger.log(level, message, error?). Adapt slog/zap/zerolog in
// roughly ten lines.
type Logger interface {
	Log(level LogLevel, message string, err error)
}

type consoleLogger struct{}

func (consoleLogger) Log(level LogLevel, message string, err error) {
	prefix := fmt.Sprintf("[dataceen-client] [%s]", level)
	w := os.Stdout
	if level == LevelWarn || level == LevelError {
		w = os.Stderr
	}
	if err != nil {
		fmt.Fprintln(w, prefix, message, err)
	} else {
		fmt.Fprintln(w, prefix, message)
	}
}

// ConsoleLogger writes to stdout (debug/info) and stderr (warn/error) with a
// "[dataceen-client]" prefix.
var ConsoleLogger Logger = consoleLogger{}

type silentLogger struct{}

func (silentLogger) Log(LogLevel, string, error) {}

// SilentLogger discards all log entries. Use in tests.
var SilentLogger Logger = silentLogger{}
