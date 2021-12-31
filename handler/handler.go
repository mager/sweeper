package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

var (
	CollectionTypeNew   CollectionType = "new"
	CollectionTypeTierA CollectionType = "a"
	CollectionTypeTierB CollectionType = "b"
	UpdateConfig                       = map[CollectionType]Config{
		CollectionTypeNew: {
			desc: "Collections added in the last 4 hours",
			log:  "Updating new collections",
			queryCond: Condition{
				path:  "floor",
				op:    "==",
				value: -1,
			},
		},
		CollectionTypeTierA: {
			desc: "7 day volume is over 0.5 ETH",
			log:  "Updating Tier A collections (7 day volume is over 0.5 ETH)",
			queryCond: Condition{
				path:  "updated",
				op:    "<",
				value: time.Now().Add(-6 * time.Hour),
			},
			updateCond: Condition{
				path:  "7d",
				op:    ">",
				value: 0.5,
			},
		},
		CollectionTypeTierB: {
			desc: "7 day volume is under 0.6 ETH",
			log:  "Updating Tier B collections (7 day volume is under 0.6 ETH)",
			queryCond: Condition{
				path:  "updated",
				op:    "<",
				value: time.Now().Add(-7 * 24 * time.Hour),
			},
			updateCond: Condition{
				path:  "7d",
				op:    "<",
				value: 0.6,
			},
		},
	}
)

type CollectionType string

type Req struct {
	CollectionType CollectionType `json:"collection_type"`
}

type Resp struct {
	Success bool `json:"success"`
}

type Condition struct {
	path, op string
	value    interface{}
}

type Config struct {
	desc       string
	queryCond  Condition
	updateCond Condition
	log        string
}

// Handler struct for HTTP requests
type Handler struct {
	ctx      context.Context
	router   *mux.Router
	logger   *zap.SugaredLogger
	os       opensea.OpenSeaClient
	database *firestore.Client
	bq       *bigquery.Client
	bot      *discordgo.Session
}

// New creates a Handler struct
func New(
	ctx context.Context,
	router *mux.Router,
	logger *zap.SugaredLogger,
	os opensea.OpenSeaClient,
	database *firestore.Client,
	bq *bigquery.Client,
	bot *discordgo.Session,
) *Handler {
	h := Handler{ctx, router, logger, os, database, bq, bot}
	h.registerRoutes()
	return &h
}

// RegisterRoutes registers all the routes for the route handler
func (h *Handler) registerRoutes() {
	h.router.HandleFunc("/update", h.update).
		Methods("POST")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var (
		req  Req
		resp = Resp{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.updateCollections(req.CollectionType)

	json.NewEncoder(w).Encode(resp)
}

// updateCollections updates the collections in the database based on a custom config
func (h *Handler) updateCollections(collectionType CollectionType) {
	// Fetch config
	c, found := UpdateConfig[collectionType]

	if !found {
		h.logger.Errorf("Invalid collection type: %s", collectionType)
		return
	}

	h.logger.Info(c.log)

	var (
		collections = h.database.Collection("collections")
		iter        = collections.Documents(h.ctx)
		count       = 0
		slugs       = make([]string, 0)
	)

	if c.queryCond.path != "" {
		iter = collections.Where(c.queryCond.path, c.queryCond.op, c.queryCond.value).Documents(h.ctx)
	}

	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			h.logger.Error(err)
		}

		// Update the floor price
		var updated bool
		if c.updateCond.path != "" {
			if c.updateCond.op == "<" {
				if doc.Data()[c.updateCond.path].(float64) < c.updateCond.value.(float64) {
					_, updated = database.UpdateCollectionStats(
						h.ctx,
						&h.os,
						h.bq,
						h.logger,
						doc,
					)
				} else {
					h.logger.Infow(
						"Floor not updated",
						"collection", doc.Ref.ID,
						"cond", fmt.Sprintf(
							"%s %.2f %s %.2f",
							c.updateCond.path,
							doc.Data()[c.updateCond.path].(float64),
							c.updateCond.op,
							c.updateCond.value,
						),
					)
				}
			} else if c.updateCond.op == ">" {
				if doc.Data()[c.updateCond.path].(float64) > c.updateCond.value.(float64) {
					_, updated = database.UpdateCollectionStats(
						h.ctx,
						&h.os,
						h.bq,
						h.logger,
						doc,
					)
				} else {
					h.logger.Infow(
						"Floor not updated",
						"collection", doc.Ref.ID,
						"cond", fmt.Sprintf(
							"%s %.2f %s %.2f",
							c.updateCond.path,
							doc.Data()[c.updateCond.path].(float64),
							c.updateCond.op,
							c.updateCond.value,
						),
					)
				}
			}
		} else {
			_, updated = database.UpdateCollectionStats(
				h.ctx,
				&h.os,
				h.bq,
				h.logger,
				doc,
			)
		}

		// Sleep because OpenSea throttles requests
		// Rate limit is 4 requests per second
		time.Sleep(time.Millisecond * 250)

		if updated {
			count++
			slugs = append(slugs, doc.Ref.ID)
		}
	}

	// Post to Discord
	if count > 0 {
		h.bot.ChannelMessageSendEmbed(
			"920371422457659482",
			&discordgo.MessageEmbed{
				Title:       fmt.Sprintf("Updated %d collections", count),
				Description: c.desc,
				Fields: []*discordgo.MessageEmbedField{
					{
						Name:   "Slugs",
						Value:  strings.Join(slugs, ", "),
						Inline: true,
					},
				},
			},
		)
	}

	h.logger.Infof("Updated %d collections", count)
}
