package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/config"
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
			bq.Options,
			config.Options,
			database.Options,
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
	database *firestore.Client,
	logger *zap.SugaredLogger,
	openSeaClient opensea.OpenSeaClient,
	router *mux.Router,
) {
	// TODO: Remove global context
	var ctx = context.Background()

	// Setup Discord Bot
	token := fmt.Sprintf("Bot %s", cfg.DiscordAuthToken)
	dg, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// TODO: Set concurrency back to default after moving this out
	//	bot.New(ctx, dg, logger, database, openSeaClient)

	handler.New(ctx, router, logger, openSeaClient, database, bq, dg)
}
