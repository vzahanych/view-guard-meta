package types


type WebGatewayConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
}