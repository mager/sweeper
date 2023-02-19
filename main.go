package main

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/gorilla/mux"
	"github.com/mager/go-opensea/opensea"
	"github.com/mager/go-reservoir/reservoir"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/etherscan"
	"github.com/mager/sweeper/handler"
	"github.com/mager/sweeper/logger"
	"github.com/mager/sweeper/nftfloorprice"
	"github.com/mager/sweeper/nftstats"
	os "github.com/mager/sweeper/opensea"
	reservoirClient "github.com/mager/sweeper/reservoir"
	"github.com/mager/sweeper/router"
	storageClient "github.com/mager/sweeper/storage"
	sweeperClient "github.com/mager/sweeper/sweeper"
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
			nftfloorprice.Options,
			nftstats.Options,
			os.Options,
			reservoirClient.Options,
			router.Options,
			storageClient.Options,
			sweeperClient.Options,
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
	nftFloorPrice *nftfloorprice.NFTFloorPriceClient,
	nftstats *nftstats.NFTStatsClient,
	openSeaClient *opensea.OpenSeaClient,
	reservoirClient *reservoir.ReservoirClient,
	router *mux.Router,
	storageClient *storage.Client,
	sweeperClient *sweeperClient.SweeperClient,
) {
	// TODO: Remove global context
	var ctx = context.Background()

	p := handler.Handler{
		BigQuery:      bq,
		Context:       ctx,
		Database:      database,
		Etherscan:     etherscan,
		Logger:        logger,
		NFTFloorPrice: nftFloorPrice,
		NFTStats:      nftstats,
		OpenSea:       openSeaClient,
		Reservoir:     reservoirClient,
		Router:        router,
		Storage:       storageClient,
		Sweeper:       sweeperClient,
	}
	handler.New(p)
}
