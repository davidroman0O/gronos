package logging

import (
	"fmt"
	"log/slog"
)

// I hate having to do that tho

type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}

type NoopLogger struct{}

func (l NoopLogger) Info(msg string, args ...interface{}) {}

func (l NoopLogger) Warn(msg string, args ...interface{}) {}

func (l NoopLogger) Error(msg string, args ...interface{}) {}

func (l NoopLogger) Debug(msg string, args ...interface{}) {}

type SlogLogger struct {
	logger *slog.Logger
}

func (l SlogLogger) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, args...)
}

func (l SlogLogger) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, args...)
}

func (l SlogLogger) Error(msg string, args ...interface{}) {
	l.logger.Error(msg, args...)
}

func (l SlogLogger) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, args...)
}

func NewSlog() SlogLogger {
	return SlogLogger{
		logger: slog.Default(),
	}
}

type FmtLogger struct{}

func (l FmtLogger) Info(msg string, args ...interface{}) {
	p := fmt.Sprintf(msg, args...)
	fmt.Println("INFO:", p)
}

func (l FmtLogger) Warn(msg string, args ...interface{}) {
	p := fmt.Sprintf(msg, args...)
	fmt.Println("WARN:", p)
}

func (l FmtLogger) Error(msg string, args ...interface{}) {
	p := fmt.Sprintf(msg, args...)
	fmt.Println("ERROR:", p)
}

func (l FmtLogger) Debug(msg string, args ...interface{}) {
	p := fmt.Sprintf(msg, args...)
	fmt.Println("DEBUG:", p)
}

func NewFmt() FmtLogger {
	return FmtLogger{}
}
