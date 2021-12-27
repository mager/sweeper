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
	"github.com/mager/sweeper/database"
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
	ctx context.Context,
	logger *zap.SugaredLogger,
	s *gocron.Scheduler,
	os opensea.OpenSeaClient,
	database *firestore.Client,
	bq *bigquery.Client,
	bot *discordgo.Session,
) *Tasks {
	logger.Info("Starting cron")
	var (
		t = Tasks{
			logger:   logger,
			s:        s,
			os:       os,
			database: database,
			bq:       bq,
			bot:      bot,
		}
	)

	// DEBUG: Custom queries
	// t.updateCollectionsWithCustomQuery(ctx)
	// t.deleteCollectionsWithCustomQuery(ctx)

	s.Every(1).Day().At("10:30").Do(func() {
		// Update new collections
		// TODO: Move to handler
		t.logger.Info("HELLO!")
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
		_, updated := database.UpdateCollectionStats(
			ctx,
			&t.os,
			t.bq,
			t.logger,
			doc,
		)

		if updated {
			count++
			slugs = append(slugs, doc.Ref.ID)
		}

	}

	// Post to Discord
	if count > 0 {
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

	t.logger.Infof("Updated %d new collections", count)
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
			_, updated := database.UpdateCollectionStats(
				ctx,
				&t.os,
				t.bq,
				t.logger,
				doc,
			)

			if updated {
				count++
				slugs = append(slugs, doc.Ref.ID)
			}
		}
	}

	// Post to Discord
	if count > 0 {
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

	t.logger.Infof("Updated %d Tier A collections", count)
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
			_, updated := database.UpdateCollectionStats(
				ctx,
				&t.os,
				t.bq,
				t.logger,
				doc,
			)

			if updated {
				count++
				slugs = append(slugs, doc.Ref.ID)
			}
		}
	}

	// Post to Discord
	if count > 0 {
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

	t.logger.Infof("Updated %d Tier B collections", count)
}

// updateCollectionsWithCustomQuery updates collections that were just added
func (t *Tasks) updateCollectionsWithCustomQuery(ctx context.Context) {
	t.logger.Info("Updating collections with custom query")

	var (
		q = "Missing thumbnail"
		// twoHoursAgo = time.Now().Add(-2 * time.Hour)

		collections = t.database.Collection("collections")
		iter        = collections.Documents(ctx)
		docs        = make([]*firestore.DocumentSnapshot, 0)
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

		docs = append(docs, doc)
		count++
	}

	// Fetch only docs where thumb is not present on the object
	for _, doc := range docs {
		if doc.Data()["thumb"] == nil {
			// Update the floor price
			database.UpdateCollectionStats(
				ctx,
				&t.os,
				t.bq,
				t.logger,
				doc,
			)
			time.Sleep(time.Millisecond * 500)
		}
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

// deleteCollectionsWithCustomQuery updates collections that were just added
func (t *Tasks) deleteCollectionsWithCustomQuery(ctx context.Context) {
	t.logger.Info("Updating collections with custom query")

	// Fetch all collections where floor is < 0.01
	// These were recently added to the database from a new user connecting
	// their wallet
	var (
		q           = "Below 0.01 floor"
		collections = t.database.Collection("collections").Where("floor", "<", 0.01)
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
		// TODO: Uncomment
		// doc.Ref.Delete(ctx)
		t.logger.Info("Deleted collection: https://opensea.io/collection/", doc.Ref.ID, " :", doc.Data()["floor"].(float64))
		count++

	}

	// Post to Discord
	t.bot.ChannelMessageSendEmbed(
		"920371422457659482",
		&discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Deleted %d collections", count),
			Description: fmt.Sprintf("Custom query: %s", q),
		},
	)
}
