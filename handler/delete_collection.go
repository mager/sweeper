package handler

import (
	"encoding/json"
	"net/http"
)

type DeleteCollectionReq struct {
	Slug string `json:"slug"`
}

func (h *Handler) deleteCollection(w http.ResponseWriter, r *http.Request) {
	var (
		req = DeleteCollectionReq{}
	)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Delete the colllection from the database
	_, err := h.Database.Collection("collections").Doc(req.Slug).Delete(h.Context)

	if err != nil {
		h.Logger.Infow("Error deleting collection from Firestore", "collection", req.Slug, "err", err)
	}
}
