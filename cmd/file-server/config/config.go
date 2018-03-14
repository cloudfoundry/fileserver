package config

import (
	"encoding/json"
	"os"

	"code.cloudfoundry.org/debugserver"
	loggingclient "code.cloudfoundry.org/diego-logging-client"
	"code.cloudfoundry.org/lager/lagerflags"
)

type FileServerConfig struct {
	ServerAddress                   string               `json:"server_address,omitempty"`
	StaticDirectory                 string               `json:"static_directory,omitempty"`
	ConsulCluster                   string               `json:"consul_cluster,omitempty"`
	EnableConsulServiceRegistration bool                 `json:"enable_consul_service_registration,omitempty"`
	LoggregatorConfig               loggingclient.Config `json:"loggregator"`
	debugserver.DebugServerConfig
	lagerflags.LagerConfig
}

func DefaultFileServerConfig() FileServerConfig {
	return FileServerConfig{
		ServerAddress: "0.0.0.0:8080",
		LagerConfig:   lagerflags.DefaultLagerConfig(),
		LoggregatorConfig: loggingclient.Config{
			SourceID: "file_server",
		},
	}
}

func NewFileServerConfig(configPath string) (FileServerConfig, error) {
	fileServerConfig := DefaultFileServerConfig()

	configFile, err := os.Open(configPath)
	if err != nil {
		return FileServerConfig{}, err
	}

	defer configFile.Close()

	decoder := json.NewDecoder(configFile)
	err = decoder.Decode(&fileServerConfig)
	if err != nil {
		return FileServerConfig{}, err
	}

	return fileServerConfig, nil
}
