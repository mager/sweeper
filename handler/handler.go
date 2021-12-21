package handler

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

// Handler struct for HTTP requests
type Handler struct {
	ctx          context.Context
	logger       *zap.SugaredLogger
	router       *mux.Router
	os           opensea.OpenSeaClient
	bq           *bigquery.Client
	cs           coinstats.CoinstatsClient
	cfg          config.Config
	database     *firestore.Client
	bot          *discordgo.Session
	infuraClient *ethclient.Client
}

// New creates a Handler struct
func New(
	ctx context.Context,
	logger *zap.SugaredLogger,
	router *mux.Router,
	os opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs coinstats.CoinstatsClient,
	cfg config.Config,
	database *firestore.Client,
	bot *discordgo.Session,
	infuraClient *ethclient.Client,
) *Handler {
	h := Handler{ctx, logger, router, os, bq, cs, cfg, database, bot, infuraClient}
	h.registerRoutes()

	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	h.router.HandleFunc("/info", h.getInfoV2).Methods("POST")
	h.router.HandleFunc("/v2/info", h.getInfoV2).Methods("POST")

	h.router.HandleFunc("/stats", h.getStats).Methods("GET")

	h.router.HandleFunc("/collection/{slug}", h.getCollection).Methods("GET")
}
