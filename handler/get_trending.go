package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	"cloud.google.com/go/firestore"
	"github.com/mager/sweeper/utils"
	"google.golang.org/api/iterator"
)

type GetTrendingResp struct {
	TopHighestFloor []TrendingCollection `json:"topHighestFloor"`
	TopWeeklyVolume []TrendingCollection `json:"topWeeklyVolume"`
}

type TrendingCollection struct {
	Rank       int     `json:"rank"`
	Name       string  `json:"name"`
	Slug       string  `json:"slug"`
	Thumb      string  `json:"thumb"`
	Value      float64 `json:"value"`
	ValueExtra float64 `json:"valueExtra"`
}

func (h *Handler) getTrending(w http.ResponseWriter, r *http.Request) {
	var (
		ctx                     = context.TODO()
		resp                    = GetTrendingResp{}
		collections             = h.database.Collection("collections")
		highestFloorCollections = make([]TrendingCollection, 0)
		highestFloorCounter     = 0
	)

	// Fetch collections with the highest floor price
	highestFloorIter := collections.Documents(ctx)

	for {
		doc, err := highestFloorIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			h.logger.Errorf("Error fetching collections: %v", err)
			break
		}

		thumb, ok := doc.Data()["thumb"].(string)
		if !ok {
			thumb = ""
		}

		sevenDayVolume, ok := doc.Data()["7d"].(float64)
		if !ok {
			sevenDayVolume = 0.0
		}

		// Only add collections with a weekly volume of over 1 ETH
		if sevenDayVolume > 1.0 {
			highestFloorCollections = append(highestFloorCollections, TrendingCollection{
				Name:  doc.Data()["name"].(string),
				Slug:  doc.Data()["slug"].(string),
				Thumb: thumb,
				Value: doc.Data()["floor"].(float64),
			})
		}
	}
	h.logger.Infow("Fetched collections", "howmany", len(highestFloorCollections))

	// Sort highest floor collections by floor price
	sort.Slice(highestFloorCollections[:], func(i, j int) bool {
		return highestFloorCollections[i].Value > highestFloorCollections[j].Value
	})

	// Only add the first 25 items to the response
	for _, collection := range highestFloorCollections {
		if highestFloorCounter < 25 {
			collection.Rank = highestFloorCounter + 1
			resp.TopHighestFloor = append(resp.TopHighestFloor, collection)
		}
		highestFloorCounter++
	}

	// Fetch collections with the highest 7d weekly volume
	highestWeeklyVolumeIter := collections.OrderBy("7d", firestore.Desc).Limit(25).Documents(ctx)
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
			Rank:       len(resp.TopWeeklyVolume) + 1,
			Name:       doc.Data()["name"].(string),
			Slug:       doc.Data()["slug"].(string),
			Thumb:      doc.Data()["thumb"].(string),
			Value:      utils.RoundFloat(doc.Data()["7d"].(float64), 2),
			ValueExtra: utils.RoundFloat(doc.Data()["floor"].(float64), 2),
		})
	}

	json.NewEncoder(w).Encode(resp)
}
