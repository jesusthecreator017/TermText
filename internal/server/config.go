package server

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	sqlcgen "github.com/jesusthecreator017/TermText/internal/db/sqlc"
)

type Config struct {
	Pool *pgxpool.Pool
	DB   *sqlcgen.Queries
	Env  *Env
}

func CreateConfig() (*Config, error) {
	env, err := LoadEnv()
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(context.Background(), env.DBUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	return &Config{
		Pool: pool,
		DB:   sqlcgen.New(pool),
		Env:  env,
	}, nil
}

func (c *Config) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}
