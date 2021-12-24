package handler

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/etherscan"
	"github.com/mager/sweeper/infura"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

// Handler struct for HTTP requests
type Handler struct {
	ctx             context.Context
	logger          *zap.SugaredLogger
	router          *mux.Router
	os              opensea.OpenSeaClient
	bq              *bigquery.Client
	cs              coinstats.CoinstatsClient
	database        *firestore.Client
	bot             *discordgo.Session
	infuraClient    *infura.InfuraClient
	etherscanClient *etherscan.EtherscanClient
}

// New creates a Handler struct
func New(
	ctx context.Context,
	logger *zap.SugaredLogger,
	router *mux.Router,
	os opensea.OpenSeaClient,
	bq *bigquery.Client,
	cs coinstats.CoinstatsClient,
	database *firestore.Client,
	bot *discordgo.Session,
	infuraClient *infura.InfuraClient,
	etherscanClient *etherscan.EtherscanClient,
) *Handler {
	h := Handler{ctx, logger, router, os, bq, cs, database, bot, infuraClient, etherscanClient}
	h.registerRoutes()

	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	// Address
	h.router.HandleFunc("/info", h.getInfoV2).Methods("POST")
	h.router.HandleFunc("/address/{address}", h.getInfoV3).Methods("GET")

	// Stats
	h.router.HandleFunc("/stats", h.getStats).Methods("GET")
	h.router.HandleFunc("/trending", h.getTrending).Methods("GET")

	// Collections
	h.router.HandleFunc("/collection/{slug}", h.getCollection).Methods("GET")

	// Testing
	h.router.HandleFunc("/collection/{slug}/tokens", h.getCollectionTokens).Methods("GET")
}
