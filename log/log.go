package log

import (
	"context"
	"io"
	"sync"

	kitlog "github.com/go-kit/kit/log"
	kitlevel "github.com/go-kit/kit/log/level"
	diadem "github.com/diademnetwork/go-diadem"
	tlog "github.com/tendermint/tendermint/libs/log"
)

// For compatibility with tendermint logger

type TMLogger tlog.Logger

var (
	NewTMLogger   = tlog.NewTMLogger
	NewTMFilter   = tlog.NewFilter
	TMAllowLevel  = tlog.AllowLevel
	NewSyncWriter = kitlog.NewSyncWriter
	Root          TMLogger
	Default       *diadem.Logger
	LevelKey      = kitlevel.Key()
)

var onceSetup sync.Once

func setupRootLogger(w io.Writer) {
	rootLoggerFunc := func(w io.Writer) TMLogger {
		return NewTMLogger(NewSyncWriter(w))
	}
	Root = rootLoggerFunc(w)
}

func setupDiademLogger(logLevel string, w io.Writer) {
	tlogTr := func(w io.Writer) kitlog.Logger {
		return tlog.NewTMFmtLogger(w)
	}
	Default = diadem.MakeDiademLogger(logLevel, w, tlogTr)
}

func Setup(diademLogLevel, dest string) {
	onceSetup.Do(func() {
		w := diadem.MakeFileLoggerWriter(diademLogLevel, dest)
		setupRootLogger(w)
		setupDiademLogger(diademLogLevel, w)
	})
}

// Info logs a message at level Info
func Info(msg string, keyvals ...interface{}) {
	Default.Info(msg, keyvals...)
}

// Debug logs a message at level Debug
func Debug(msg string, keyvals ...interface{}) {
	Default.Debug(msg, keyvals...)
}

// Error logs a message at level Error
func Error(msg string, keyvals ...interface{}) {
	Default.Error(msg, keyvals...)
}

// Warn logs a message at level Warn
func Warn(msg string, keyvals ...interface{}) {
	Default.Warn(msg, keyvals...)
}

type contextKey string

func (c contextKey) String() string {
	return "log " + string(c)
}

var (
	contextKeyLog = contextKey("log")
)

func SetContext(ctx context.Context, log diadem.Logger) context.Context {
	return context.WithValue(ctx, contextKeyLog, log)
}

func Log(ctx context.Context) tlog.Logger {
	logger, _ := ctx.Value(contextKeyLog).(tlog.Logger)
	if logger == nil {
		return Root
	}

	return logger
}
