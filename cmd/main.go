package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/vatsimnerd/github-artifact-fetcher/config"
	"github.com/vatsimnerd/github-artifact-fetcher/server"
)

var (
	log = logrus.WithField("module", "receiver.main")
)

func main() {

	cfg, err := config.Read("fetcher.yaml")
	if err != nil {
		log.WithError(err).Fatal("error reading config")
	}

	logLevel, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = logrus.InfoLevel
		log.WithField("log_level", cfg.LogLevel).Error("error parsing log_level")
	}
	logrus.SetLevel(logLevel)

	srv := server.New(cfg)

	srv.Start()
	defer srv.Stop()

	sigs := make(chan os.Signal, 1024)
	signal.Notify(sigs, syscall.SIGINT)
	defer signal.Reset()
	defer close(sigs)

	<-sigs

}
