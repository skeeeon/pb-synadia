// Package utils provides utility helpers for pb-synadia.
package utils

import (
	"fmt"
	"log"
	"time"
)

// Logger provides categorized logging with visual prefixes.
type Logger struct {
	enabled bool
}

// NewLogger creates a logger. Pass false in production for silent operation.
func NewLogger(enabled bool) *Logger { return &Logger{enabled: enabled} }

func (l *Logger) write(prefix, format string, args ...any) {
	if !l.enabled {
		return
	}
	ts := time.Now().Format("15:04:05")
	log.Printf("[%s] %s %s", ts, prefix, fmt.Sprintf(format, args...))
}

func (l *Logger) Start(f string, a ...any)   { l.write("\U0001F680 START", f, a...) }
func (l *Logger) Success(f string, a ...any) { l.write("✅ SUCCESS", f, a...) }
func (l *Logger) Info(f string, a ...any)    { l.write("ℹ️  INFO", f, a...) }
func (l *Logger) Process(f string, a ...any) { l.write("⚙️  PROCESS", f, a...) }
func (l *Logger) Warning(f string, a ...any) { l.write("⚠️  WARNING", f, a...) }
func (l *Logger) Error(f string, a ...any)   { l.write("❌ ERROR", f, a...) }
func (l *Logger) Stop(f string, a ...any)    { l.write("\U0001F6D1 STOP", f, a...) }
func (l *Logger) Publish(f string, a ...any) { l.write("\U0001F4E4 PUBLISH", f, a...) }
func (l *Logger) Delete(f string, a ...any)  { l.write("\U0001F5D1️  DELETE", f, a...) }
