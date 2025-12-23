package types


// ObjectStorageConfig contains object storage configuration
type ObjectStorageConfig struct {
	Provider  string `yaml:"provider"`
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
}