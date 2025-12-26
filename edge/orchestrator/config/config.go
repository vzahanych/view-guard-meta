package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"

	auditlogtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	aigwtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	evtbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	objectstoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	statemngtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	telemetryotel "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/telemetry-otel"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	webgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/types"
)

// Config represents the application configuration
type Config struct {
	Environment   string                                 `yaml:"environment"`
	LogLevel      string                                 `yaml:"log_level"`
	LogFormat     string                                 `yaml:"log_format"`
	ConfigFile    string                                 `yaml:"config_file"`
	EventBus      evtbustypes.EventBusConfig             `yaml:"event_bus"`
	AI            aigwtypes.AIGatewayConfig              `yaml:"ai"`
	MetaStorage   metastoragetypes.MetaStorageConfig     `yaml:"meta_storage"`
	ObjectStorage objectstoragetypes.ObjectStorageConfig `yaml:"object_storage"`
	VMGateway     vmgatewaytypes.VMGatewayConfig         `yaml:"vm_gateway"`
	CCTV          cctvtypes.CCTVServiceConfig            `yaml:"cctv"`
	Telemetry     telemetryotel.Config                   `yaml:"telemetry"`
	WebGateway    webgatewaytypes.WebGatewayConfig       `yaml:"web_gateway"`
	AuditLog      auditlogtypes.AuditLogConfig            `yaml:"audit_log"`
	StateManager  statemngtypes.StateManagerConfig        `yaml:"state_manager"`
}

// Load loads configuration from a YAML file.
// If configPath is empty, it searches common locations:
// - ./config.yaml
// - ./config/config.yaml
// - /etc/view-guard-edge/config.yaml
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		// Try common locations
		commonPaths := []string{
			"./config.yaml",
			"./config/config.yaml",
			"/etc/view-guard-edge/config.yaml",
		}
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}
		if configPath == "" {
			return nil, fmt.Errorf("no config file found in common locations and no path provided")
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Unmarshal the YAML into a map first to handle the "edge" root key
	var raw map[string]interface{}
	if parseErr := yaml.Unmarshal(data, &raw); parseErr != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", parseErr)
	}

	// Extract the "edge" section if present
	var edgeSection map[string]interface{}
	if edge, ok := raw["edge"].(map[string]interface{}); ok {
		edgeSection = edge
	} else {
		// If no "edge" key, assume the config is at root level
		edgeSection = raw
	}

	// Marshal the edge section back to YAML for unmarshaling into Config
	edgeYAML, marshalErr := yaml.Marshal(edgeSection)
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to marshal edge section: %w", marshalErr)
	}

	var cfg Config
	if unmarshalErr := yaml.Unmarshal(edgeYAML, &cfg); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", unmarshalErr)
	}

	// Set the config file path for reference
	absPath, absErr := filepath.Abs(configPath)
	if absErr != nil {
		absPath = configPath
	}
	cfg.ConfigFile = absPath

	return &cfg, nil
}

// ConfigProvider creates the Config with fx lifecycle management.
// It reads the config path from the CONFIG_PATH environment variable,
// or uses the provided defaultPath if set.
// If neither is set, it searches common locations.
func ConfigProvider(defaultPath string) func(logger *zap.Logger) (*Config, error) {
	return func(logger *zap.Logger) (*Config, error) {
		configPath := defaultPath
		if configPath == "" {
			configPath = os.Getenv("CONFIG_PATH")
		}

		cfg, err := Load(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}

		if logger != nil {
			logger.Info("Configuration loaded",
				zap.String("config_file", cfg.ConfigFile),
				zap.String("environment", cfg.Environment),
			)
		}

		return cfg, nil
	}
}

// ConfigProviderWithPath creates a ConfigProvider that uses a specific config path.
// This is useful when the config path is known at compile time or from flags.
// Logger is optional - if provided, it will be used for logging config loading.
func ConfigProviderWithPath(configPath string) fx.Option {
	return fx.Provide(func() (*Config, error) {
		cfg, err := Load(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		return cfg, nil
	})
}

// LoggerProvider creates a zap.Logger based on the configuration.
// It uses the log_level and log_format fields from Config to configure the logger.
func LoggerProvider(cfg *Config) (*zap.Logger, error) {
	if cfg == nil {
		// Fallback to development logger if config is nil
		return zap.NewDevelopment()
	}

	// Parse log level
	var level zapcore.Level
	logLevel := strings.ToLower(cfg.LogLevel)
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "fatal":
		level = zapcore.FatalLevel
	default:
		level = zapcore.InfoLevel // Default to info
	}

	// Configure encoder based on log format
	logFormat := strings.ToLower(cfg.LogFormat)
	var encoderConfig zapcore.EncoderConfig
	var encoder zapcore.Encoder

	if logFormat == "json" {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		// Default to text/console format (development style)
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Create core
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stderr), level)

	// Create logger with additional fields
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	
	// Add default fields
	logger = logger.With(
		zap.String("environment", cfg.Environment),
	)

	return logger, nil
}
