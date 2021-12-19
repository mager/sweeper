package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mager/sweeper/opensea"
)

type GetCollectionResp struct {
	Name              string                    `json:"name"`
	Slug              string                    `json:"slug"`
	Floor             float64                   `json:"floor"`
	WeeklyVolumeETH   float64                   `json:"weeklyVolumeETH"`
	Updated           time.Time                 `json:"updated"`
	Thumb             string                    `json:"thumb"`
	OpenSeaCollection opensea.OpenSeaCollection `json:"opensea_collection"`
}

// getCollection is the route handler for the GET /collection/{slug} endpoint
func (h *Handler) getCollection(w http.ResponseWriter, r *http.Request) {
	var (
		ctx        = context.TODO()
		resp       = GetCollectionResp{}
		collection = h.database.Collection("collections")
		slug       = mux.Vars(r)["slug"]
	)

	// Fetch collection from database
	docsnap, err := collection.Doc(slug).Get(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	d := docsnap.Data()

	// Set slug
	resp.Name = d["name"].(string)
	resp.Slug = slug
	resp.Floor = d["floor"].(float64)
	resp.WeeklyVolumeETH = d["7d"].(float64)
	resp.Updated = d["updated"].(time.Time)

	// Fetch collection from OpenSea
	openSeaCollection, err := h.os.GetCollection(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Hydrate response with OpenSea content
	resp.Thumb = openSeaCollection.Collection.ImageURL

	resp.OpenSeaCollection = openSeaCollection.Collection

	json.NewEncoder(w).Encode(resp)
}
