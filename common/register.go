package common

import (
	"context"
	"log"
	"net/http"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	"github.com/kelseyhightower/envconfig"
	cs "github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func Register(
	lc fx.Lifecycle,
	logger *zap.SugaredLogger,
	router *mux.Router,
	os opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs cs.CoinstatsClient,
	s *gocron.Scheduler,
	database *firestore.Client,
) (
	*zap.SugaredLogger,
	*mux.Router,
	opensea.OpenSeaClient,
	*bigquery.Client,
	cs.CoinstatsClient,
	config.Config,
	*firestore.Client,
) {
	var cfg config.Config
	err := envconfig.Process("floorreport", &cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	lc.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				addr := ":8080"
				logger.Info("Listening on ", addr)

				if err != nil {
					log.Fatal(err.Error())
				}
				s.Every(24).Hours().Do(func() {
					logger.Info("Ping")
				})
				s.StartAsync()

				go http.ListenAndServe(addr, router)
				return nil
			},
			OnStop: func(context.Context) error {
				defer logger.Sync()
				return nil
			},
		},
	)

	return logger, router, os, bq, cs, cfg, database
}
