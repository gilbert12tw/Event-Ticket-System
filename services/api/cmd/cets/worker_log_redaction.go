package main

import (
	"regexp"
	"strings"
)

var (
	workerLogEmailPattern    = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	workerLogTokenPattern    = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]+){1,2}\b`)
	workerLogEmployeePattern = regexp.MustCompile(`(?i)\bE[0-9]{4,}\b`)
)

func safeWorkerLogError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "worker batch failed"
	}
	message = workerLogEmailPattern.ReplaceAllString(message, "[redacted email]")
	message = workerLogTokenPattern.ReplaceAllString(message, "[redacted token]")
	message = workerLogEmployeePattern.ReplaceAllString(message, "[redacted employee]")
	return message
}
