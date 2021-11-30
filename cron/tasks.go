package cron

import (
	"context"
	"time"

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
}

func Initialize(logger *zap.SugaredLogger, s *gocron.Scheduler, os opensea.OpenSeaClient, database *firestore.Client) *Tasks {
	logger.Info("Starting cron")
	var (
		ctx = context.TODO()

		t = Tasks{
			logger:   logger,
			s:        s,
			os:       os,
			database: database,
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

		// Fetch all collections where floor is 0
		zeroFloorDocs := database.Collection("collections").Where("floor", "==", 0)
		iter = zeroFloorDocs.Documents(ctx)
		defer iter.Stop()

		for {
			logger.Infof("Found collections with floor 0")
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				// TODO: Handle error.
				logger.Error(err)
			}
			logger.Info(doc.UpdateTime, doc.Data())

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
	docID := doc.Ref.ID
	floor := t.getOpenSeaFloor(docID)
	t.logger.Infof("Updating floor price to %v for %s", floor, docID)
	_, err := doc.Ref.Update(ctx, []firestore.Update{
		{Path: "floor", Value: floor},
		{Path: "updated", Value: time.Now()},
	})
	if err != nil {
		t.logger.Errorf("Error updating floor price: %v", err)
	}
}
