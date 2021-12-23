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

	stats, err := openSeaClient.GetCollectionStatsForSlug(docID)
	if err != nil {
		logger.Error(err)
	}

	var (
		floor       = stats.FloorPrice
		sevenDayVol = stats.SevenDayVolume
		now         = time.Now()
		updated     bool
	)

	if floor >= 0.01 {
		logger.Infof("Updating floor price to %v for %s", floor, docID)

		doc.Ref.Update(ctx, []firestore.Update{
			{Path: "floor", Value: floor},
			{Path: "7d", Value: sevenDayVol},
			{Path: "updated", Value: now},
		})

		bq.RecordCollectionsUpdateInBigQuery(
			bigQueryClient,
			logger,
			docID,
			floor,
			sevenDayVol,
			now,
		)

		updated = true
	}

	return floor, updated
}
