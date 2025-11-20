package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/rs/zerolog/log"
)

// Config вся необходимая конфигурация для сервиса
type Config struct {
	Port        string `env:"PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"INFO"`
}

func NewConfig() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	log.Info().Msg("Configuration loaded")
	return cfg, nil
}
