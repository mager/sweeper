package cron

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
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
	bot      *discordgo.Session
}

func Initialize(
	logger *zap.SugaredLogger,
	s *gocron.Scheduler,
	os opensea.OpenSeaClient,
	database *firestore.Client,
	bq *bigquery.Client,
	bot *discordgo.Session,
) *Tasks {
	logger.Info("Starting cron")
	var (
		ctx = context.TODO()

		t = Tasks{
			logger:   logger,
			s:        s,
			os:       os,
			database: database,
			bq:       bq,
			bot:      bot,
		}
	)
	s.Every(6).Hours().Do(func() {
		// Fetch all collections where floor is -1
		// These were recently added to the database from a new user connecting
		// their wallet
		newCollections := database.Collection("collections").Where("floor", "==", -1)
		iter := newCollections.Documents(ctx)
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

		// Fetch all collections that haven't been updated in the past 6 hours
		sixHoursAgo := time.Now().Add(-6 * time.Hour)
		sixHoursAgoDocs := database.Collection("collections").Where("updated", "<", sixHoursAgo)
		iter = sixHoursAgoDocs.Documents(ctx)
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

	// Post to Discord
	t.bot.ChannelMessageSend(
		"920371422457659482",
		fmt.Sprintf("New floor price for %s (%s): %vΞ", docID, getOpenSeaCollectionURL(docID), floor),
	)

	t.recordCollectionsUpdateInBigQuery(docID, floor, now)
}

func getOpenSeaCollectionURL(docID string) string {
	return fmt.Sprintf("https://opensea.io/collection/%s", docID)
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
