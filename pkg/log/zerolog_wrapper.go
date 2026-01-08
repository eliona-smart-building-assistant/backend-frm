package log

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type Logger interface {
	Debug() *zerolog.Event
	Info() *zerolog.Event
	Warn() *zerolog.Event
	Error() *zerolog.Event
	Fatal() *zerolog.Event
	Trace() *zerolog.Event
	With() zerolog.Context
}
type Level zerolog.Level

const (
	DebugLevel = Level(zerolog.DebugLevel)
	InfoLevel  = Level(zerolog.InfoLevel)
	WarnLevel  = Level(zerolog.WarnLevel)
	ErrorLevel = Level(zerolog.ErrorLevel)
	FatalLevel = Level(zerolog.FatalLevel)
	TraceLevel = Level(zerolog.TraceLevel)
)

// NewLogger creates new logger instance
func NewLogger(out io.Writer, level Level) Logger {
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "message"
	zerolog.TimeFieldFormat = time.RFC3339Nano

	_, isDevEnv := os.LookupEnv("DEV_ENVIRONMENT")

	if isDevEnv {
		consoleWriter := zerolog.ConsoleWriter{Out: out, TimeFormat: "15:04:05.999999"}
		l := zerolog.New(consoleWriter).
			Level(zerolog.Level(level)).
			With().Timestamp().Logger()

		return &l
	}

	// Production logger
	l := zerolog.New(out).Level(zerolog.Level(level)).
		With().Timestamp().Logger()

	return &l
}

func FromZerologLogger(l zerolog.Logger) Logger {
	return &l
}

func SetLogLevel(l Logger, level Level) {
	if z, ok := l.(*zerolog.Logger); ok {
		z.Level(zerolog.Level(level))
	}
}

func NoopLogger() Logger {
	l := zerolog.Nop()
	return &l
}
