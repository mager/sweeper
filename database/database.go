package database

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

type CollectionV2 struct {
	Name           string    `firestore:"name" json:"name"`
	Thumb          string    `firestore:"thumb" json:"thumb"`
	Floor          float64   `firestore:"floor" json:"floor"`
	Slug           string    `firestore:"slug" json:"slug"`
	SevenDayVolume float64   `firestore:"7d" json:"7d"`
	Updated        time.Time `firestore:"updated" json:"updated"`
}

// ProvideDB provides a firestore client
func ProvideDB() *firestore.Client {
	projectID := "floor-report-327113"

	client, err := firestore.NewClient(context.TODO(), projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return client
}

var Options = ProvideDB

func UpdateCollectionStats(
	ctx context.Context,
	openSeaClient *opensea.OpenSeaClient,
	bigQueryClient *bigquery.Client,
	logger *zap.SugaredLogger,
	doc *firestore.DocumentSnapshot,
) (float64, bool) {
	var docID = doc.Ref.ID

	// Fetch collection from OpenSea
	collection, err := openSeaClient.GetCollection(docID)
	if err != nil {
		logger.Error(err)
	}

	var (
		stats       = collection.Collection.Stats
		floor       = stats.FloorPrice
		sevenDayVol = stats.SevenDayVolume
		now         = time.Now()
		updated     bool
	)

	if collection.Collection.Slug != "" && floor >= 0.01 {
		logger.Infow("Updating floor price", "floor", floor, "collection", docID)

		// Update collection
		_, err := doc.Ref.Update(ctx, []firestore.Update{
			{Path: "floor", Value: floor},
			{Path: "7d", Value: sevenDayVol},
			{Path: "thumb", Value: collection.Collection.ImageURL},
			{Path: "updated", Value: now},
		})
		if err != nil {
			logger.Error(err)
		}

		bq.RecordCollectionsUpdateInBigQuery(
			bigQueryClient,
			logger,
			docID,
			floor,
			sevenDayVol,
			now,
		)

		updated = true
	} else {
		logger.Infow("Floor too low", "collection", docID)
	}

	time.Sleep(time.Millisecond * 500)

	return floor, updated
}

func AddCollectionToDB(
	ctx context.Context,
	openSeaClient *opensea.OpenSeaClient,
	logger *zap.SugaredLogger,
	database *firestore.Client,
	collection opensea.OpenSeaCollectionCollection,
	c CollectionV2,
) {
	// Add collection to db
	c.Updated = time.Now()

	// Get stats
	stat, err := openSeaClient.GetCollectionStatsForSlug(collection.Slug)
	if err != nil {
		logger.Error(err)
	}
	if stat.FloorPrice >= 0.01 {
		c.Floor = stat.FloorPrice
		c.SevenDayVolume = stat.SevenDayVolume
		c.Thumb = collection.ImageURL
		_, err := database.Collection("collections").Doc(collection.Slug).Set(ctx, c)
		if err != nil {
			logger.Error(err)
			return
		}
		logger.Infow("Added collection", "collection", collection.Slug)
	}
}
