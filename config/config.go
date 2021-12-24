package config

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	OpenSeaAPIKey    string
	DiscordAuthToken string
	InfuraKey        string
	EtherscanAPIKey  string
}

func ProvideConfig() Config {
	var cfg Config
	err := envconfig.Process("floorreport", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}
	return cfg
}

var Options = ProvideConfig
