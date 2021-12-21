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

	// DEBUG: Custom query
	// t.updateCollectionsWithCustomQuery(ctx)

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
		t.os.UpdateCollectionStats(ctx, t.bq, t.logger, doc)
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
			t.os.UpdateCollectionStats(ctx, t.bq, t.logger, doc)

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
			t.os.UpdateCollectionStats(ctx, t.bq, t.logger, doc)

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

// updateCollectionsWithCustomQuery updates collections that were just added
func (t *Tasks) updateCollectionsWithCustomQuery(ctx context.Context) {
	t.logger.Info("Updating collections with custom query")

	// Fetch all collections where floor is -1
	// These were recently added to the database from a new user connecting
	// their wallet
	var (
		q           = "Updated in the last 2 hours"
		twoHoursAgo = time.Now().Add(-2 * time.Hour)

		collections = t.database.Collection("collections").Where("updated", "<", twoHoursAgo)
		iter        = collections.Documents(ctx)
		count       = 0
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
		t.os.UpdateCollectionStats(ctx, t.bq, t.logger, doc)
		count++

	}

	// Post to Discord
	t.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Updated %d Collections", count),
			Description: fmt.Sprintf("Custom query: %s", q),
		},
	)
}
