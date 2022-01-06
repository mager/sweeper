package database

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	bq "github.com/mager/sweeper/bigquery"
	"github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/utils"
	"go.uber.org/zap"
)

type Collection struct {
	Name            string    `firestore:"name" json:"name"`
	Thumb           string    `firestore:"thumb" json:"thumb"`
	Floor           float64   `firestore:"floor" json:"floor"`
	Slug            string    `firestore:"slug" json:"slug"`
	OneDayVolume    float64   `firestore:"1d" json:"1d"`
	SevenDayVolume  float64   `firestore:"7d" json:"7d"`
	ThirtyDayVolume float64   `firestore:"30d" json:"30d"`
	MarketCap       float64   `firestore:"cap" json:"cap"`
	TotalSupply     float64   `firestore:"supply" json:"supply"`
	NumOwners       int       `firestore:"num" json:"num"`
	TotalSales      float64   `firestore:"sales" json:"sales"`
	Updated         time.Time `firestore:"updated" json:"updated"`
}

// TODO: Delete
type CollectionV2 struct {
	Name            string    `firestore:"name" json:"name"`
	Thumb           string    `firestore:"thumb" json:"thumb"`
	Floor           float64   `firestore:"floor" json:"floor"`
	Slug            string    `firestore:"slug" json:"slug"`
	OneDayVolume    float64   `firestore:"1d" json:"1d"`
	SevenDayVolume  float64   `firestore:"7d" json:"7d"`
	ThirtyDayVolume float64   `firestore:"30d" json:"30d"`
	MarketCap       float64   `firestore:"cap" json:"cap"`
	TotalSupply     float64   `firestore:"supply" json:"supply"`
	NumOwners       int       `firestore:"num" json:"num"`
	TotalSales      float64   `firestore:"sales" json:"sales"`
	Updated         time.Time `firestore:"updated" json:"updated"`
}

type User struct {
	Name    string `firestore:"name" json:"name"`
	Photo   bool   `firestore:"photo" json:"photo"`
	ENSName string `firestore:"ensName" json:"ensName"`

	// Wallet
	Wallet Wallet `firestore:"wallet" json:"wallet"`

	// Following
	Collections []string `firestore:"collections" json:"collections"`

	// Socials
	Slug    string `firestore:"slug" json:"slug"`
	Twitter string `firestore:"twitter" json:"twitter"`
	OpenSea string `firestore:"openSea" json:"openSea"`

	// Settings
	IsWhale     bool `firestore:"isWhale" json:"isWhale"`
	ShouldIndex bool `firestore:"shouldIndex" json:"shouldIndex"`
}

type WalletCollection struct {
	Name     string        `firestore:"name" json:"name"`
	Slug     string        `firestore:"slug" json:"slug"`
	ImageURL string        `firestore:"imageUrl" json:"imageUrl"`
	NFTs     []WalletAsset `firestore:"nfts" json:"nfts"`
}

type WalletAsset struct {
	Name     string `firestore:"name" json:"name"`
	TokenID  string `firestore:"tokenId" json:"tokenId"`
	ImageURL string `firestore:"imageUrl" json:"imageUrl"`
	// TODO: Add Traits
}

type Wallet struct {
	Collections []WalletCollection `firestore:"collections" json:"collections"`
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

		if err.Error() == opensea.OpenSeaNotFoundError {
			DeleteCollection(ctx, logger, doc)
		}
	}

	var (
		stats        = collection.Collection.Stats
		floor        = stats.FloorPrice
		sevenDayVol  = stats.SevenDayVolume
		oneDayVol    = stats.OneDayVolume
		thirtyDayVol = stats.ThirtyDayVolume
		marketCap    = stats.MarketCap
		numOwners    = stats.NumOwners
		totalSupply  = stats.TotalSupply
		totalSales   = stats.TotalSales
		now          = time.Now()
		updated      bool
	)

	if collection.Collection.Slug != "" && floor >= 0.01 {
		logger.Infow("Updating floor price", "floor", floor, "collection", docID)

		// Update collection
		_, err := doc.Ref.Update(ctx, []firestore.Update{
			{Path: "1d", Value: utils.RoundFloat(oneDayVol, 3)},
			{Path: "30d", Value: utils.RoundFloat(thirtyDayVol, 3)},
			{Path: "7d", Value: utils.RoundFloat(sevenDayVol, 3)},
			{Path: "cap", Value: utils.RoundFloat(marketCap, 3)},
			{Path: "floor", Value: floor},
			{Path: "num", Value: numOwners},
			{Path: "sales", Value: utils.RoundFloat(totalSales, 3)},
			{Path: "supply", Value: utils.RoundFloat(totalSupply, 3)},
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
		logger.Infow("Floor below 0.01", "collection", docID, "floor", floor)
	}

	time.Sleep(time.Millisecond * 500)

	return floor, updated
}

func AddCollectionToDB(
	ctx context.Context,
	openSeaClient *opensea.OpenSeaClient,
	logger *zap.SugaredLogger,
	database *firestore.Client,
	slug string,
) (float64, bool) {
	// Add collection to db
	var (
		c = Collection{
			Updated: time.Now(),
		}
		floor float64
	)

	// Get collection from OpenSea
	collection, err := openSeaClient.GetCollection(slug)
	stat := collection.Collection.Stats
	floor = stat.FloorPrice

	if err != nil {
		logger.Error(err)
		return floor, false
	}
	if stat.FloorPrice >= 0.01 {
		c.Floor = stat.FloorPrice
		c.MarketCap = stat.MarketCap
		c.NumOwners = stat.NumOwners
		c.OneDayVolume = stat.OneDayVolume
		c.SevenDayVolume = stat.SevenDayVolume
		c.ThirtyDayVolume = stat.ThirtyDayVolume
		c.Thumb = collection.Collection.ImageURL
		c.TotalSales = stat.TotalSales
		c.TotalSupply = stat.TotalSupply
		c.Slug = slug
		c.Name = collection.Collection.Name
		_, err := database.Collection("collections").Doc(slug).Set(ctx, c)
		if err != nil {
			logger.Error(err)
			return floor, false
		}
		logger.Infow("Added collection", "collection", slug)
	}

	return floor, true
}

func DeleteCollection(
	ctx context.Context,
	logger *zap.SugaredLogger,
	doc *firestore.DocumentSnapshot,
) {
	collection := doc.Ref.ID

	// Delete collection from db
	_, err := doc.Ref.Delete(ctx)
	if err != nil {
		logger.Error(err)
		return
	}
	logger.Infow("Deleted collection", "collection", collection)
}
