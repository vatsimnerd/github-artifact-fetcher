package server

import (
	"context"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vatsimnerd/github-artifact-fetcher/config"
	"github.com/vatsimnerd/github-artifact-fetcher/fetcher"
)

type Server struct {
	cfg     *config.Config
	srv     *http.Server
	fetcher *fetcher.Fetcher
	stop    chan struct{}
}

var (
	log = logrus.WithField("module", "fetcher.server")
)

func New(cfg *config.Config) *Server {
	return &Server{
		cfg:     cfg,
		fetcher: fetcher.New(cfg),
		stop:    make(chan struct{}),
	}
}

func (s *Server) Start() {
	s.fetcher.Start()
	s.srv = &http.Server{
		Addr:    s.cfg.Addr,
		Handler: http.DefaultServeMux,
	}
	http.HandleFunc(s.cfg.Endpoint, s.handleEvents)
	log.WithFields(logrus.Fields{
		"endpoint": s.cfg.Endpoint,
		"addr":     s.cfg.Addr,
	}).Info("starting server")
	go func() {
		err := http.ListenAndServe(s.cfg.Addr, http.DefaultServeMux)
		if err != nil {
			log.WithError(err).Error("error while serving http")
		}
	}()
}

func (s *Server) Stop() {
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.fetcher.Stop()
	s.srv.Shutdown(ctx)
}
