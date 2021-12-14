package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type StatsResp struct {
	Total int `json:"total"`
}

// getStats is the route handler for the GET /stats endpoint
func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = context.TODO()
		resp = StatsResp{}
		docs = make([]*firestore.DocumentRef, 0)
	)

	// Fetch all collections
	iter := h.database.Collection("collections").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Errorf("Error fetching collections: %v", err)
			break
		}
		docs = append(docs, doc.Ref)
	}

	resp.Total = len(docs)

	json.NewEncoder(w).Encode(resp)
}
