package server

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Env struct {
	DBUrl     string        `env:"DB_URL,required"`
	Port      string        `env:"PORT" envDefault:":8080"`
	PasetoKey string        `env:"PASETO_KEY,required"`
	TokenTTL  time.Duration `env:"TOKEN_TTL" envDefault:"24h"`
}

func LoadEnv() (*Env, error) {
	var cfg Env
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return &cfg, nil
}
