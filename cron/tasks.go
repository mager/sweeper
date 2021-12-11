package cron

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/go-co-op/gocron"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

type Tasks struct {
	logger   *zap.SugaredLogger
	s        *gocron.Scheduler
	os       opensea.OpenSeaClient
	database *firestore.Client
	bq       *bigquery.Client
}

func Initialize(logger *zap.SugaredLogger, s *gocron.Scheduler, os opensea.OpenSeaClient, database *firestore.Client, bq *bigquery.Client) *Tasks {
	logger.Info("Starting cron")
	var (
		ctx = context.TODO()

		t = Tasks{
			logger:   logger,
			s:        s,
			os:       os,
			database: database,
			bq:       bq,
		}
	)
	s.Every(1).Hours().Do(func() {
		// DEBUG: Fetch all collections
		iter := database.Collection("collections").Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				logger.Errorf("Error fetching collections: %v", err)
				break
			}
			logger.Infof("Collection: %v", doc.Data())
		}

		// Fetch all collections where floor is -1
		// These were recently added to the database from a new user connecting
		// their wallet
		newCollections := database.Collection("collections").Where("floor", "==", -1)
		iter = newCollections.Documents(ctx)
		defer iter.Stop()

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// TODO: Handle error.
				logger.Error(err)
			}

			// Update the floor price
			t.updateFloorPrice(ctx, doc)
		}

		// Fetch all collections that haven't been updated in the past 2 hours
		hourAgo := time.Now().Add(-2 * time.Hour)
		hourAgoDocs := database.Collection("collections").Where("updated", "<", hourAgo)
		iter = hourAgoDocs.Documents(ctx)
		defer iter.Stop()

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// TODO: Handle error.
				logger.Error(err)
			}
			logger.Info("Found collection with old data")
			logger.Info(doc.UpdateTime, doc.Data())

			// Update the floor price
			t.updateFloorPrice(ctx, doc)
		}
	})

	return &t
}

// getOpenSeaFloor gets the floor price from collections on OpenSea
func (t *Tasks) getOpenSeaFloor(docID string) float64 {
	stat, err := t.os.GetCollectionStatsForSlug(docID)
	if err != nil {
		t.logger.Error(err)
	}
	return stat.FloorPrice
}

func (t *Tasks) updateFloorPrice(ctx context.Context, doc *firestore.DocumentSnapshot) {
	var (
		docID = doc.Ref.ID
		floor = t.getOpenSeaFloor(docID)
		now   = time.Now()
	)
	t.logger.Infof("Updating floor price to %v for %s", floor, docID)
	_, err := doc.Ref.Update(ctx, []firestore.Update{
		{Path: "floor", Value: floor},
		{Path: "updated", Value: now},
	})
	if err != nil {
		t.logger.Errorf("Error updating floor price: %v", err)
	}
	t.recordCollectionsUpdateInBigQuery(docID, floor, now)
}

type BQCollectionsUpdateRecord struct {
	Slug        string
	Floor       float64
	RequestTime time.Time
}

// recordCollectionsUpdateInBigQuery posts a info request event to BigQuery
func (h *Tasks) recordCollectionsUpdateInBigQuery(
	slug string,
	floor float64,
	t time.Time,
) {
	var (
		ctx     = context.Background()
		dataset = h.bq.DatasetInProject("floor-report-327113", "collections")
		table   = dataset.Table("update")
		u       = table.Inserter()

		items = []*BQCollectionsUpdateRecord{
			{
				Slug:        slug,
				Floor:       floor,
				RequestTime: t,
			},
		}
	)
	if err := u.Put(ctx, items); err != nil {
		h.logger.Error(err)
	}
}
