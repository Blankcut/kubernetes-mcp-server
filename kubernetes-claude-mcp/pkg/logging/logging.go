package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap logger.
//
// Debug, Info, Warn, Error, DPanic, Panic and Fatal take a message followed by
// alternating key/value pairs, and log the pairs as structured fields:
//
//	logger.Info("HTTP request", "method", r.Method, "status", code)
//
// The embedded SugaredLogger's own versions of those methods run every
// argument through fmt.Sprint instead, which glued the pairs onto the message
// ("msg":"HTTP requestmethodGETstatus200"). Every other SugaredLogger method,
// including the *w and *f variants, is unchanged.
//
// Malformed pairs never panic. A trailing key with no value, or a non-string
// key, is reported by zap as a separate ERROR entry naming what was dropped.
type Logger struct {
	*zap.SugaredLogger

	// kv backs the key/value methods. It skips one extra stack frame so that
	// "caller" names the call site rather than this file.
	kv *zap.SugaredLogger
}

// NewLogger creates a new logger
func NewLogger() *Logger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Determine log level from environment variable
	logLevel := zap.InfoLevel
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch envLevel {
		case "debug":
			logLevel = zap.DebugLevel
		case "info":
			logLevel = zap.InfoLevel
		case "warn":
			logLevel = zap.WarnLevel
		case "error":
			logLevel = zap.ErrorLevel
		}
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		zap.NewAtomicLevelAt(logLevel),
	)

	return newLogger(zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)))
}

// newLogger wraps base. Every Logger is built here so that its two sugared
// loggers always share one core, name and set of fields.
func newLogger(base *zap.Logger) *Logger {
	return &Logger{
		SugaredLogger: base.Sugar(),
		kv:            base.WithOptions(zap.AddCallerSkip(1)).Sugar(),
	}
}

// With returns a logger with the specified key-value pairs
func (l *Logger) With(args ...interface{}) *Logger {
	return newLogger(l.SugaredLogger.With(args...).Desugar())
}

// Named returns a logger with the specified name
func (l *Logger) Named(name string) *Logger {
	return newLogger(l.Desugar().Named(name))
}

// Debug logs msg at debug level with keysAndValues as structured fields.
func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.kv.Debugw(msg, keysAndValues...)
}

// Info logs msg at info level with keysAndValues as structured fields.
func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	l.kv.Infow(msg, keysAndValues...)
}

// Warn logs msg at warn level with keysAndValues as structured fields.
func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	l.kv.Warnw(msg, keysAndValues...)
}

// Error logs msg at error level with keysAndValues as structured fields.
func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	l.kv.Errorw(msg, keysAndValues...)
}

// DPanic logs msg at dpanic level with keysAndValues as structured fields. It
// panics only in development mode, which NewLogger does not enable.
func (l *Logger) DPanic(msg string, keysAndValues ...interface{}) {
	l.kv.DPanicw(msg, keysAndValues...)
}

// Panic logs msg at panic level with keysAndValues as structured fields, then
// panics.
func (l *Logger) Panic(msg string, keysAndValues ...interface{}) {
	l.kv.Panicw(msg, keysAndValues...)
}

// Fatal logs msg at fatal level with keysAndValues as structured fields, then
// exits the process.
func (l *Logger) Fatal(msg string, keysAndValues ...interface{}) {
	l.kv.Fatalw(msg, keysAndValues...)
}
