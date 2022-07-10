package main

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/gorilla/mux"
	"github.com/mager/go-opensea/opensea"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/etherscan"
	"github.com/mager/sweeper/handler"
	"github.com/mager/sweeper/logger"
	"github.com/mager/sweeper/nftstats"
	os "github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/reservoir"
	"github.com/mager/sweeper/router"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	fx.New(
		fx.Provide(
			bq.Options,
			config.Options,
			database.Options,
			etherscan.Options,
			logger.Options,
			nftstats.Options,
			os.Options,
			reservoir.Options,
			router.Options,
		),
		fx.Invoke(Register),
	).Run()
}

func Register(
	lc fx.Lifecycle,
	bq *bigquery.Client,
	cfg config.Config,
	database *firestore.Client,
	etherscan *etherscan.EtherscanClient,
	logger *zap.SugaredLogger,
	nftstats *nftstats.NFTStatsClient,
	openSeaClient *opensea.OpenSeaClient,
	reservoirClient *reservoir.ReservoirClient,
	router *mux.Router,
) {
	// TODO: Remove global context
	var ctx = context.Background()

	// TODO: Set concurrency back to default after moving this out
	//	bot.New(ctx, dg, logger, database, openSeaClient)

	p := handler.Handler{
		BigQuery:  bq,
		Context:   ctx,
		Database:  database,
		Etherscan: etherscan,
		Logger:    logger,
		NFTStats:  nftstats,
		OpenSea:   openSeaClient,
		Reservoir: reservoirClient,
		Router:    router,
	}
	handler.New(p)
}
