package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.Logger for structured logging
type Logger struct {
	*zap.Logger
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error, fatal
	Format string `yaml:"format"` // json, text
	Output string `yaml:"output"` // stdout, stderr, or file path
}

// New creates a new logger with the given configuration
func New(cfg LogConfig) (*Logger, error) {
	var zapConfig zap.Config

	// Set log level
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	// Configure encoder based on format
	if cfg.Format == "json" {
		zapConfig = zap.NewProductionConfig()
		zapConfig.Level = zap.NewAtomicLevelAt(level)
	} else {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.Level = zap.NewAtomicLevelAt(level)
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	// Set output
	if cfg.Output != "" && cfg.Output != "stdout" && cfg.Output != "stderr" {
		zapConfig.OutputPaths = []string{cfg.Output}
		zapConfig.ErrorOutputPaths = []string{cfg.Output}
	} else if cfg.Output == "stderr" {
		zapConfig.OutputPaths = []string{"stderr"}
		zapConfig.ErrorOutputPaths = []string{"stderr"}
	}

	// Build logger
	zapLogger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{zapLogger}, nil
}

// Sync flushes any buffered log entries
func (l *Logger) Sync() error {
	return l.Logger.Sync()
}

