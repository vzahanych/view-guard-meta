package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for structured logging
type Logger struct {
	*zap.Logger
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string
	Format string
	Output string
}

// New creates a new logger based on configuration
func New(cfg LogConfig) (*Logger, error) {
	var config zap.Config
	var encoderConfig zapcore.EncoderConfig

	// Set log level
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// Configure encoder based on format
	if cfg.Format == "json" {
		config = zap.NewProductionConfig()
		encoderConfig = zap.NewProductionEncoderConfig()
		config.Encoding = "json"
	} else {
		config = zap.NewDevelopmentConfig()
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		config.Encoding = "console"
	}

	// Set encoder config
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	config.EncoderConfig = encoderConfig
	config.Level = zap.NewAtomicLevelAt(level)

	// Set output
	if cfg.Output != "" && cfg.Output != "stdout" {
		config.OutputPaths = []string{cfg.Output}
		config.ErrorOutputPaths = []string{cfg.Output}
	}

	// Build logger
	zapLogger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &Logger{zapLogger}, nil
}

// Sync flushes any buffered log entries
func (l *Logger) Sync() {
	_ = l.Logger.Sync()
}

// WithFields creates a child logger with additional fields
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{l.Logger.With(fields...)}
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...interface{}) {
	l.Logger.Info(msg, convertFields(fields...)...)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields ...interface{}) {
	l.Logger.Error(msg, convertFields(fields...)...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...interface{}) {
	l.Logger.Warn(msg, convertFields(fields...)...)
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...interface{}) {
	l.Logger.Debug(msg, convertFields(fields...)...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields ...interface{}) {
	l.Logger.Fatal(msg, convertFields(fields...)...)
}

// convertFields converts interface{} fields to zap.Field
func convertFields(fields ...interface{}) []zap.Field {
	zapFields := make([]zap.Field, 0, len(fields)/2)
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		zapFields = append(zapFields, zap.Any(key, fields[i+1]))
	}
	return zapFields
}

// NewNopLogger creates a no-op logger for testing
func NewNopLogger() *Logger {
	return &Logger{zap.NewNop()}
}

