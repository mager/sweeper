package main

import (
	"cloud.google.com/go/bigquery"
	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	bq "github.com/mager/sweeper/bigquery"
	cs "github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/common"
	"github.com/mager/sweeper/cron"
	"github.com/mager/sweeper/handler"
	"github.com/mager/sweeper/logger"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/router"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	fx.New(
		fx.Provide(
			logger.Options,
			router.Options,
			opensea.Options,
			bq.Options,
			cs.Options,
			cron.Options,
		),
		fx.Invoke(Register),
	).Run()
}

func Register(
	lc fx.Lifecycle,
	logger *zap.SugaredLogger,
	router *mux.Router,
	os opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs cs.CoinstatsClient,
	s *gocron.Scheduler,
) {
	logger, router, os, bq, cs, cfg := common.Register(
		lc, logger, router, os, bq, cs, s,
	)

	handler.New(logger, router, os, bq, cs, cfg)
}
