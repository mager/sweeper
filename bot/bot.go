package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"github.com/dustin/go-humanize"
	"github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

func New(
	dg *discordgo.Session,
	logger *zap.SugaredLogger,
	database *firestore.Client,
	openSeaClient opensea.OpenSeaClient,
) {
	var err error
	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate(logger, database, openSeaClient))

	// In this example, we only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsGuildMessages

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Cleanly close down the Discord session.
	defer dg.Close()
}

func messageCreate(logger *zap.SugaredLogger, database *firestore.Client, openSeaClient opensea.OpenSeaClient) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {

		// Ignore all messages created by the bot itself
		// This isn't required in this specific example but it's a good practice.
		if m.Author.ID == s.State.User.ID {
			return
		}

		// floor command
		fCommand(logger, database, s, m)
		uCommand(logger, database, openSeaClient, s, m)
	}
}

// fCommand is a command that returns the floor for a collection
func fCommand(logger *zap.SugaredLogger, database *firestore.Client, s *discordgo.Session, m *discordgo.MessageCreate) {
	var (
		ctx = context.TODO()
		re  = regexp.MustCompile(`^(?m)f (.*)`)
	)

	for i, match := range re.FindAllString(m.Content, -1) {
		if i == 0 {
			// The slug is everything after `f `
			var slug = match[2:]

			// Fetch the record from database
			docsnap, err := database.Collection("collections").Doc(slug).Get(ctx)
			if err != nil {
				logger.Errorw("Error fetching document", "error", err)
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Error fetching floor for %s", slug))
				return
			}

			// Get the floor price from the record
			var (
				floorPrice = docsnap.Data()["floor"].(float64)
				msg        strings.Builder
			)
			msg.WriteString(fmt.Sprintf("The floor price for %s is: ", slug))
			msg.WriteString(fmt.Sprintf("%.2f", floorPrice))
			msg.WriteString(fmt.Sprintf(" (last updated %s)", humanize.Time(docsnap.Data()["updated"].(time.Time))))

			s.ChannelMessageSend(m.ChannelID, msg.String())
		}
	}
}

// uCommand is a command that updates the floor for a collection
func uCommand(
	logger *zap.SugaredLogger,
	database *firestore.Client,
	openSeaClient opensea.OpenSeaClient,
	s *discordgo.Session,
	m *discordgo.MessageCreate) {
	var (
		ctx = context.TODO()
		re  = regexp.MustCompile(`^(?m)u (.*)`)
	)

	for i, match := range re.FindAllString(m.Content, -1) {
		if i == 0 {
			// The slug is everything after `u `
			var slug = match[2:]

			// Fetch the record from database
			docsnap, err := database.Collection("collections").Doc(slug).Get(ctx)
			if err != nil {
				logger.Errorw("Error fetching document", "error", err)
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Error fetching floor for %s", slug))
				return
			}

			floor := openSeaClient.UpdateCollectionStats(ctx, logger, docsnap)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("The floor price for %s was updated to: %.2f", slug, floor))
		}
	}
}
