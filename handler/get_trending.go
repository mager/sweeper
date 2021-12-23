package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type GetTrendingResp struct {
	TopHighestFloor []TrendingCollection `json:"topHighestFloor"`
	TopWeeklyVolume []TrendingCollection `json:"topWeeklyVolume"`
}

type TrendingCollection struct {
	Rank  int     `json:"rank"`
	Name  string  `json:"name"`
	Slug  string  `json:"slug"`
	Value float64 `json:"value"`
}

func (h *Handler) getTrending(w http.ResponseWriter, r *http.Request) {
	var (
		ctx         = context.TODO()
		resp        = GetTrendingResp{}
		collections = h.database.Collection("collections")
	)

	// Fetch collections with the highest floor price
	highestFloorIter := collections.OrderBy("floor", firestore.Desc).Limit(20).Documents(ctx)
	for {
		doc, err := highestFloorIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Errorf("Error fetching collections: %v", err)
			break
		}
		resp.TopHighestFloor = append(resp.TopHighestFloor, TrendingCollection{
			Rank:  len(resp.TopHighestFloor) + 1,
			Name:  doc.Data()["name"].(string),
			Slug:  doc.Data()["slug"].(string),
			Value: doc.Data()["floor"].(float64),
		})
	}

	// Fetch collections with the highest 7d weekly volume
	highestWeeklyVolumeIter := collections.OrderBy("7d", firestore.Desc).Limit(10).Documents(ctx)
	for {
		doc, err := highestWeeklyVolumeIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Errorf("Error fetching collections: %v", err)
			break
		}
		resp.TopWeeklyVolume = append(resp.TopWeeklyVolume, TrendingCollection{
			Rank:  len(resp.TopWeeklyVolume) + 1,
			Name:  doc.Data()["name"].(string),
			Slug:  doc.Data()["slug"].(string),
			Value: doc.Data()["7d"].(float64),
		})
	}

	json.NewEncoder(w).Encode(resp)
}
