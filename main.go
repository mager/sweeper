package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/bot"
	cs "github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/common"
	"github.com/mager/sweeper/cron"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/handler"
	"github.com/mager/sweeper/infura"
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
			database.Options,
			infura.Options,
		),
		fx.Invoke(Register),
	).Run()
}

func Register(
	lc fx.Lifecycle,
	logger *zap.SugaredLogger,
	router *mux.Router,
	openSeaClient opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs cs.CoinstatsClient,
	s *gocron.Scheduler,
	database *firestore.Client,
	infuraClient *ethclient.Client,
) {
	var ctx = context.Background()
	logger, router, openSeaClient, bq, cs, cfg, database, infuraClient := common.Register(
		lc,
		logger,
		router,
		openSeaClient,
		bq,
		cs,
		s,
		database,
		infuraClient,
	)

	// Setup Discord Bot
	token := fmt.Sprintf("Bot %s", cfg.DiscordAuthToken)
	dg, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Route handler
	handler.New(ctx, logger, router, openSeaClient, bq, cs, cfg, database, dg, infuraClient)

	// Run cron tasks
	cron.Initialize(ctx, logger, s, openSeaClient, database, bq, dg)

	// Discord bot
	bot.New(ctx, dg, logger, database, openSeaClient)
}
