package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jesusthecreator017/TermText/internal/auth"
	"github.com/jesusthecreator017/TermText/internal/hub"
	"github.com/jesusthecreator017/TermText/internal/server/handler"
)

type Server struct {
	cfg     *Config
	mux     *http.ServeMux
	handler *handler.Handler
	tokens  *auth.TokenMaker
	hub     *hub.Hub
	hubStop context.CancelFunc
	logger  *slog.Logger
	http    *http.Server
}

func NewServer() (*Server, error) {
	cfg, err := CreateConfig()
	if err != nil {
		return nil, err
	}

	tokens, err := auth.NewTokenMaker(cfg.Env.PasetoKey, cfg.Env.TokenTTL)
	if err != nil {
		return nil, err
	}

	logger := slog.Default()
	mux := http.NewServeMux()

	router := newMessageRouter(cfg.DB, logger)
	h := hub.New(router, logger)
	hubCtx, hubStop := context.WithCancel(context.Background())
	go h.Run(hubCtx)

	hdl := handler.New(cfg.DB, logger, tokens, auth.DefaultParams(), h)
	root := Mount(mux, hdl, tokens, logger)

	s := &Server{
		cfg:     cfg,
		mux:     mux,
		handler: hdl,
		tokens:  tokens,
		hub:     h,
		hubStop: hubStop,
		logger:  logger,
		http: &http.Server{
			Addr:              cfg.Env.Port,
			Handler:           root,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
	return s, nil
}

func (s *Server) Start() error {
	s.logger.Info("server listening", "addr", s.http.Addr)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")
	s.hubStop()
	err := s.http.Shutdown(ctx)
	s.cfg.Close()
	return err
}
