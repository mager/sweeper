package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mager/sweeper/database"
)

type UpdateCollectionResp struct {
	Success    bool                `json:"success"`
	Collection database.Collection `json:"collection"`
}

func (h *Handler) updateCollection(w http.ResponseWriter, r *http.Request) {
	var (
		resp = UpdateCollectionResp{}
	)

	vars := mux.Vars(r)
	slug := vars["slug"]

	resp.Collection, resp.Success = h.updateSingleCollection(slug)

	json.NewEncoder(w).Encode(resp)
}

// updateSingleCollection updates a single collection
func (h *Handler) updateSingleCollection(slug string) (database.Collection, bool) {
	var (
		err        error
		floor      float64
		collection database.Collection
		updated    bool
	)

	docsnap, err := h.Database.Collection("collections").Doc(slug).Get(h.Context)

	if err != nil {
		h.Logger.Errorw(
			"Error fetching collection from Firestore, trying to add collection",
			"err", err,
		)
		floor, updated := database.AddCollectionToDB(h.Context, h.OpenSea, h.NFTFloorPrice, h.Logger, h.Database, slug)
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
		updated = database.UpdateCollectionStats(
			h.Context,
			h.Logger,
			h.OpenSea,
			h.BigQuery,
			h.NFTStats,
			h.Reservoir,
			docsnap,
		)
		h.Logger.Infow(
			"Collection updated",
			"collection", docsnap.Ref.ID,
			"floor", floor,
			"updated", updated,
		)
	}

	if updated {
		// Fetch collection
		collection = database.GetCollection(h.Context, h.Logger, h.Database, slug)
	}

	return collection, updated
}
