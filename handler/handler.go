package handler

import (
	"context"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/coinstats"
	"github.com/mager/sweeper/infura"
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
	database     *firestore.Client
	bot          *discordgo.Session
	infuraClient *infura.InfuraClient
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
) *Handler {
	h := Handler{ctx, logger, router, os, bq, cs, database, bot, infuraClient}
	h.registerRoutes()

	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	h.router.HandleFunc("/info", h.getInfoV2).Methods("POST")
	h.router.HandleFunc("/v2/info", h.getInfoV2).Methods("POST")

	h.router.HandleFunc("/stats", h.getStats).Methods("GET")
	h.router.HandleFunc("/trending", h.getTrending).Methods("GET")

	h.router.HandleFunc("/collection/{slug}", h.getCollection).Methods("GET")
}
