package kafka

type Logger interface {
	Info(area string, message string, args ...interface{})
	Warning(area string, message string, args ...interface{})
	Error(area string, message string, args ...interface{})
	Debug(area string, message string, args ...interface{})
}

type NoopLogger struct{}

func (n NoopLogger) Info(area string, message string, args ...interface{}) {}

func (n NoopLogger) Warning(area string, message string, args ...interface{}) {}

func (n NoopLogger) Error(area string, message string, args ...interface{}) {}

func (n NoopLogger) Debug(area string, message string, args ...interface{}) {}
