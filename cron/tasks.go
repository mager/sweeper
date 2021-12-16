package cron

import (
	"context"
	"fmt"
	"strings"
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

type BQCollectionsUpdateRecord struct {
	Slug           string
	Floor          float64
	SevenDayVolume float64
	RequestTime    time.Time
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
		// Update new collections
		// TODO: Move to handler
		t.updateNewCollections(ctx)

		// Update all collections if their 7 day volume is over 0.5 ETH
		t.updateTierACollections(ctx)

	})

	s.Every(1).Day().Do(func() {
		// Update less active collections
		t.updateTierBCollections(ctx)
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

	t.recordCollectionsUpdateInBigQuery(docID, floor, sevenDayVol, now)

	time.Sleep(time.Second * 1)
	t.logger.Info("Sleeping for 1 second")
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

// updateNewCollections updates collections that were just added
func (t *Tasks) updateNewCollections(ctx context.Context) {
	t.logger.Info("Updating new collections")

	// Fetch all collections where floor is -1
	// These were recently added to the database from a new user connecting
	// their wallet
	var (
		newCollections = t.database.Collection("collections").Where("floor", "==", -1)
		iter           = newCollections.Documents(ctx)
		count          = 0
		slugs          = make([]string, 0)
	)

	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			t.logger.Error(err)
		}

		// Update the floor price
		t.updateCollectionStats(ctx, doc)
		count++
		slugs = append(slugs, doc.Ref.ID)

	}

	// Post to Discord
	t.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Updated %d New Collections", count),
			Description: "Collections added in the last 4 hours",
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Slugs",
					Value:  strings.Join(slugs, ", "),
					Inline: true,
				},
			},
		},
	)
}

// updateTierACollections updates the database if their 7 day volume is over 0.5 ETH
func (t *Tasks) updateTierACollections(ctx context.Context) {
	t.logger.Info("Updating Tier A collections (7 day volume is over 0.5 ETH)")

	var (
		twelveHoursAgo = time.Now().Add(-12 * time.Hour)
		iter           = t.database.Collection("collections").Where("updated", "<", twelveHoursAgo).Documents(ctx)
		count          = 0
		slugs          = make([]string, 0)
	)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			t.logger.Error(err)
		}

		if doc.Data()["7d"].(float64) > 0.5 {
			t.updateCollectionStats(ctx, doc)

			count++
			slugs = append(slugs, doc.Ref.ID)
		}
	}

	// Post to Discord
	t.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Updated %d Tier A Collections", count),
			Description: "7 day volume is over 0.5 ETH",
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Slugs",
					Value:  strings.Join(slugs, ", "),
					Inline: true,
				},
			},
		},
	)
}

// updateTierBCollections updates the database if their 7 day volume is ender 0.6 ETH
func (t *Tasks) updateTierBCollections(ctx context.Context) {
	t.logger.Info("Updating Tier B collections (7 day volume is under 0.6 ETH)")

	var (
		weekAgo = time.Now().Add(-7 * 24 * time.Hour)
		iter    = t.database.Collection("collections").Where("updated", "<", weekAgo).Documents(ctx)
		count   = 0
		slugs   = make([]string, 0)
	)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
			t.logger.Error(err)
		}

		if doc.Data()["7d"].(float64) < 0.6 {
			t.updateCollectionStats(ctx, doc)

			count++
			slugs = append(slugs, doc.Ref.ID)
		}
	}

	// Post to Discord
	t.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Updated %d Tier B Collections", count),
			Description: "7 day volume is under 0.6 ETH",
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Slugs",
					Value:  strings.Join(slugs, ", "),
					Inline: true,
				},
			},
		},
	)
}
