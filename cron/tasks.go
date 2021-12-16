package cron

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/go-co-op/gocron"
	"github.com/mager/sweeper/common"
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
	s.Every(4).Hours().Do(func() {
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
			t.updateCollectionStats(ctx, doc)
		}

		// Fetch all collections that haven't been updated in the past 12 hours
		lastUpdated := time.Now().Add(-12 * time.Hour)
		oldDocs := database.Collection("collections").Where("updated", "<", lastUpdated)
		iter = oldDocs.Documents(ctx)
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

			// Update the floor price & other stats
			t.updateCollectionStats(ctx, doc)
		}
	})

	return &t
}

// getOpenSeaStats gets the floor price from collections on OpenSea
func (t *Tasks) getOpenSeaStats(docID string) opensea.OpenSeaCollectionStat {
	stat, err := t.os.GetCollectionStatsForSlug(docID)
	if err != nil {
		t.logger.Error(err)
	}
	return stat
}

func (t *Tasks) updateCollectionStats(ctx context.Context, doc *firestore.DocumentSnapshot) {
	var (
		docID       = doc.Ref.ID
		stats       = t.getOpenSeaStats(docID)
		floor       = stats.FloorPrice
		sevenDayVol = stats.SevenDayVolume
		now         = time.Now()
	)

	t.logger.Infof("Updating floor price to %v for %s", floor, docID)

	_, err := doc.Ref.Update(ctx, []firestore.Update{
		{Path: "floor", Value: floor},
		{Path: "7d", Value: sevenDayVol},
		{Path: "updated", Value: now},
	})
	if err != nil {
		t.logger.Errorf("Error updating floor price: %v", err)
	}

	// Post to Discord
	t.bot.ChannelMessageSend(
		"920371422457659482",
		fmt.Sprintf("New floor price for %s (%s): %vΞ", docID, common.GetOpenSeaCollectionURL(docID), floor),
	)

	t.recordCollectionsUpdateInBigQuery(docID, floor, sevenDayVol, now)

	time.Sleep(time.Second * 1)
	t.logger.Info("Sleeping for 1 second")
}

type BQCollectionsUpdateRecord struct {
	Slug           string
	Floor          float64
	SevenDayVolume float64
	RequestTime    time.Time
}

// recordCollectionsUpdateInBigQuery posts a info request event to BigQuery
func (h *Tasks) recordCollectionsUpdateInBigQuery(
	slug string,
	floor float64,
	sevenDayVol float64,
	t time.Time,
) {
	var (
		ctx     = context.Background()
		dataset = h.bq.DatasetInProject("floor-report-327113", "collections")
		table   = dataset.Table("update")
		u       = table.Inserter()

		items = []*BQCollectionsUpdateRecord{
			{
				Slug:           slug,
				Floor:          floor,
				SevenDayVolume: sevenDayVol,
				RequestTime:    t,
			},
		}
	)
	if err := u.Put(ctx, items); err != nil {
		h.logger.Error(err)
	}
}
