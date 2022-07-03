package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/database"
	"google.golang.org/api/iterator"
)

type UpdateStatsReq struct {
	Success bool `json:"success"`
}

func (h *Handler) updateStats(w http.ResponseWriter, r *http.Request) {
	var (
		resp = UpdateUsersResp{}
	)

	resp.Success = h.doUpdateStats()

	json.NewEncoder(w).Encode(resp)
}

// doUpdateAddresses updates a collection of addresses
func (h *Handler) doUpdateStats() bool {
	var (
		ctx              = context.Background()
		collections      = h.Database.Collection("collections")
		users            = h.Database.Collection("users")
		collectionsIter  = collections.Documents(h.Context)
		usersIter        = users.Documents(h.Context)
		c                database.Collection
		u                database.User
		collectionsCount = 0
		usersCount       = 0
	)

	// Fetch collections from Firestore
	for {
		doc, err := collectionsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.Logger.Error(err)
		}

		err = doc.DataTo(&c)
		if err != nil {
			h.Logger.Error(err)
		}
		collectionsCount++
	}

	// Fetch users from Firestore
	for {
		doc, err := usersIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.Logger.Error(err)
		}

		err = doc.DataTo(&u)
		if err != nil {
			h.Logger.Error(err)
		}
		usersCount++
	}

	h.Logger.Infof("Found %d collections & %d users", collectionsCount, usersCount)

	h.Database.Collection("features").Doc("stats").Set(ctx, map[string]interface{}{
		"totalCollections": collectionsCount,
		"totalUsers":       usersCount,
		"updated":          time.Now(),
	}, firestore.MergeAll)

	return true
}
