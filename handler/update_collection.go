package handler

import (
	"encoding/json"
	"net/http"

	"github.com/mager/sweeper/database"
)

type UpdateCollectionReq struct {
	Slug string `json:"slug"`
}
type UpdateCollectionResp struct {
	Queued bool `json:"queued"`
}

func (h *Handler) updateCollection(w http.ResponseWriter, r *http.Request) {
	var (
		req  = UpdateCollectionReq{}
		resp = UpdateCollectionResp{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	go h.updateSingleCollection(req.Slug)

	resp.Queued = true

	json.NewEncoder(w).Encode(resp)
}

// updateSingleCollection updates a single collection
func (h *Handler) updateSingleCollection(slug string) (database.Collection, bool) {
	var (
		err        error
		collection database.Collection
		updated    bool
	)

	docsnap, err := h.Database.Collection("collections").Doc(slug).Get(h.Context)

	if err != nil {
		h.Logger.Errorw(
			"Error fetching collection from Firestore, trying to add collection",
			"err", err,
		)
		floor, updated := database.AddCollectionToDBV2(h.Context, h.Reservoir, h.NFTFloorPrice, h.Logger, h.Database, slug)
		h.Logger.Infow(
			"Collection added",
			"collection", slug,
			"floor", floor,
			"updated", updated,
		)

		if updated {
			// Fetch collection
			collection = database.GetCollection(h.Context, h.Logger, h.Database, slug)
		}

		return collection, updated
	}

	if docsnap.Exists() {
		// Update collection
		h.Logger.Info("Collection found, updating")
		go database.UpdateCollectionStatsV2(
			h.Context,
			h.Logger,
			h.OpenSea,
			h.BigQuery,
			h.NFTStats,
			h.Reservoir,
			docsnap,
		)
	}

	collection = database.GetCollection(h.Context, h.Logger, h.Database, slug)

	return collection, updated
}
