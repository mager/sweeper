package main

import (
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
	"github.com/mager/sweeper/common"
	"github.com/mager/sweeper/cron"
	"github.com/mager/sweeper/database"
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
			database.Options,
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
) {
	logger, router, openSeaClient, bq, cs, cfg, database := common.Register(
		lc,
		logger,
		router,
		openSeaClient,
		bq,
		cs,
		s,
		database,
	)

	// Setup Discord Bot
	token := fmt.Sprintf("Bot %s", cfg.DiscordAuthToken)
	dg, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Route handler
	handler.New(logger, router, openSeaClient, bq, cs, cfg, database, dg)

	// Run cron tasks
	cron.Initialize(logger, s, openSeaClient, database, bq, dg)

	// Discord bot
	bot.New(dg, logger, database, openSeaClient)
}
