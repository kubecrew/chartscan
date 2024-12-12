package utils

import (
	"log"
	"os"
)

func CreateLogger() *log.Logger {
	return log.New(os.Stdout, "[chartscan] ", log.LstdFlags)
}
