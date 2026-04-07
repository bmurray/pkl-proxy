package main

import (
	"fmt"
	"log/slog"
	"os"
)

// verbose controls whether detailed request logging is shown.
// Set via -v or --verbose flag.
var verbose bool

// initLogger configures the default slog logger to write to stderr
// with an appropriate log level based on the verbose flag.
func initLogger() {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

// logInfo prints an informational message to stderr, only when verbose is enabled.
func logInfo(format string, args ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// logWarn prints a warning message to stderr (always shown).
func logWarn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// logError prints an error message to stderr (always shown).
func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
