package utils

import (
	"log"
	"os"
)

// CreateLogger initializes and returns a new logger instance.
// The logger writes to the standard output and prefixes each log entry
// with "[chartscan]" followed by the date and time.
func CreateLogger() *log.Logger {
	return log.New(os.Stdout, "[chartscan] ", log.LstdFlags)
}
