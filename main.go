package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/bot"
	cs "github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/cron"
	"github.com/mager/sweeper/database"
	ethscan "github.com/mager/sweeper/etherscan"
	"github.com/mager/sweeper/handler"
	"github.com/mager/sweeper/infura"
	"github.com/mager/sweeper/logger"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/router"
	"github.com/nanmu42/etherscan-api"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func main() {
	fx.New(
		fx.Provide(
			bq.Options,
			config.Options,
			cron.Options,
			cs.Options,
			database.Options,
			ethscan.Options,
			infura.Options,
			logger.Options,
			opensea.Options,
			router.Options,
		),
		fx.Invoke(Register),
	).Run()
}

func Register(
	lc fx.Lifecycle,
	bq *bigquery.Client,
	cfg config.Config,
	cs cs.CoinstatsClient,
	etherscanClient *etherscan.Client,
	database *firestore.Client,
	infuraClient *infura.InfuraClient,
	logger *zap.SugaredLogger,
	openSeaClient opensea.OpenSeaClient,
	router *mux.Router,
	s *gocron.Scheduler,
) {
	// TODO: Remove global context
	var ctx = context.Background()

	// Setup Discord Bot
	token := fmt.Sprintf("Bot %s", cfg.DiscordAuthToken)
	dg, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Route handler
	handler.New(
		ctx,
		logger,
		router,
		openSeaClient,
		bq,
		cs,
		database,
		dg,
		infuraClient,
	)

	// Run cron tasks
	// cron.Initialize(ctx, logger, s, openSeaClient, database, bq, dg)

	// Discord bot
	// TODO: Move this bot out of this app!
	//       - https://github.com/mager/sweeper/issues/8
	// TODO: Set concurrency back to default after moving this out
	bot.New(ctx, dg, logger, database, openSeaClient)
}
