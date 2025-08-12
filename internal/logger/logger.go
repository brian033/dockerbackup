package logger

import (
	"fmt"
	"log"
	"os"
)

type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
	Debugf(format string, args ...any)
	With(key string, value any) Logger
}

type SimpleLogger struct {
	prefix       string
	debugEnabled bool
}

func New() Logger {
	debug := os.Getenv("DOCKERBACKUP_DEBUG")
	debugEnabled := debug == "1" || debug == "true" || debug == "on" || debug == "DEBUG"
	l := &SimpleLogger{
		prefix:       "",
		debugEnabled: debugEnabled,
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return l
}

func (l *SimpleLogger) With(key string, value any) Logger {
	sep := ""
	if l.prefix != "" {
		sep = " "
	}
	return &SimpleLogger{
		prefix:       l.prefix + sep + fmt.Sprintf("[%s=%v]", key, value),
		debugEnabled: l.debugEnabled,
	}
}

func (l *SimpleLogger) Infof(format string, args ...any) {
	l.printf("INFO", format, args...)
}

func (l *SimpleLogger) Errorf(format string, args ...any) {
	l.printf("ERROR", format, args...)
}

func (l *SimpleLogger) Debugf(format string, args ...any) {
	if l.debugEnabled {
		l.printf("DEBUG", format, args...)
	}
}

func (l *SimpleLogger) printf(level string, format string, args ...any) {
	if l.prefix != "" {
		log.Printf("%s %s %s", level, l.prefix, fmt.Sprintf(format, args...))
	} else {
		log.Printf("%s %s", level, fmt.Sprintf(format, args...))
	}
}
