package database

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"github.com/mager/go-opensea/opensea"
	"github.com/mager/sweeper/nftfloorprice"
	"github.com/mager/sweeper/nftstats"
	os "github.com/mager/sweeper/opensea"
	"github.com/mager/sweeper/reservoir"
	"github.com/mager/sweeper/utils"
	"go.uber.org/zap"
)

var (
	collectionDenylist = []string{
		"deadlink123",
		"heir-game",
		"spritelites",
		"sky-club-by-jump",
	}
)

type Collection struct {
	Name            string      `firestore:"name" json:"name"`
	Thumb           string      `firestore:"thumb" json:"thumb"`
	Floor           float64     `firestore:"floor" json:"floor"`
	Slug            string      `firestore:"slug" json:"slug"`
	OneDayVolume    float64     `firestore:"1d" json:"1d"`
	SevenDayVolume  float64     `firestore:"7d" json:"7d"`
	ThirtyDayVolume float64     `firestore:"30d" json:"30d"`
	MarketCap       float64     `firestore:"cap" json:"cap"`
	TotalSupply     float64     `firestore:"supply" json:"supply"`
	NumOwners       int         `firestore:"num" json:"num"`
	TotalSales      float64     `firestore:"sales" json:"sales"`
	Updated         time.Time   `firestore:"updated" json:"updated"`
	TopNFTs         []TopNFT    `firestore:"topNFTs" json:"topNFTs"`
	Contract        string      `firestore:"contract" json:"contract"`
	Attributes      []Attribute `firestore:"attributes" json:"attributes"`
}

type Attribute struct {
	Key   string  `firestore:"key" json:"key"`
	Value string  `firestore:"value" json:"value"`
	Floor float64 `firestore:"floor" json:"floor"`
	Image string  `firestore:"image" json:"image"`
}

type User struct {
	Name    string `firestore:"name" json:"name"`
	Bio     string `firestore:"bio" json:"bio"`
	Photo   bool   `firestore:"photo" json:"photo"`
	ENSName string `firestore:"ensName" json:"ensName"`

	// Wallet
	Wallet Wallet `firestore:"wallet" json:"wallet"`

	// Following
	Collections []string `firestore:"collections" json:"collections"`

	// Socials
	Slug      string `firestore:"slug" json:"slug"`
	Twitter   string `firestore:"twitter" json:"twitter"`
	DiscordID string `firestore:"discordID" json:"discordID"`

	// Settings
	IsFren      bool `firestore:"isFren" json:"isFren"`
	ShouldIndex bool `firestore:"shouldIndex" json:"shouldIndex"`
}

type WalletCollection struct {
	Name     string        `firestore:"name" json:"name"`
	Slug     string        `firestore:"slug" json:"slug"`
	ImageURL string        `firestore:"imageUrl" json:"imageUrl"`
	NFTs     []WalletAsset `firestore:"nfts" json:"nfts"`
	Floor    float64       `firestore:"floor" json:"floor"`
}

type WalletAsset struct {
	Name       string      `firestore:"name" json:"name"`
	TokenID    string      `firestore:"tokenId" json:"tokenId"`
	ImageURL   string      `firestore:"imageUrl" json:"imageUrl"`
	Attributes []Attribute `firestore:"attributes" json:"attributes"`
	Floor      float64     `firestore:"floor" json:"floor"`
}

type Trait struct {
	TraitType string `firestore:"traitType" json:"traitType"`
	Value     string `firestore:"value" json:"value"`
}

type Wallet struct {
	Collections []WalletCollection `firestore:"collections" json:"collections"`
	UpdatedAt   time.Time          `firestore:"updatedAt" json:"updatedAt"`
}

type Contract struct {
	Name      string  `firestore:"name" json:"name"`
	Address   string  `firestore:"address" json:"address"`
	NumTokens int     `firestore:"numTokens" json:"numTokens"`
	LastBlock int64   `firestore:"lastBlock" json:"lastBlock"`
	Tokens    []Token `firestore:"tokens" json:"tokens"`
	Updated   int64   `firestore:"updated" json:"updated"`
}

type Token struct {
	ID        int64  `firestore:"id" json:"id"`
	Owner     string `firestore:"owner" json:"owner"`
	LastSale  int64  `firestore:"lastSale" json:"lastSale"`
	DiscordID int64  `firestore:"discordId" json:"discordId"`
}

type Alias struct {
	Slug string `firestore:"slug" json:"slug"`
}

type TopNFT struct {
	Image  string `firestore:"image" json:"image"`
	Name   string `firestore:"name" json:"name"`
	OSLink string `firestore:"osLink" json:"osLink"`
}

const (
	// MaxFloorPrice is the maximum floor price
	MaxFloorPrice = 100.0
)

// ProvideDB provides a firestore client
func ProvideDB() *firestore.Client {
	projectID := "floorreport"

	client, err := firestore.NewClient(context.TODO(), projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return client
}

var Options = ProvideDB

func UpdateCollectionStats(
	ctx context.Context,
	logger *zap.SugaredLogger,
	openSeaClient *opensea.OpenSeaClient,
	bigQueryClient *bigquery.Client,
	nftstatsClient *nftstats.NFTStatsClient,
	reservoirClient *reservoir.ReservoirClient,
	doc *firestore.DocumentSnapshot,
) bool {
	docID := doc.Ref.ID

	// Fetch collection from OpenSea
	collection, err := openSeaClient.GetCollection(docID)
	if err != nil {
		logger.Error(err)

		if err.Error() == "collection_not_found" {
			DeleteCollection(ctx, logger, doc)
		}
	}

	// Fetch collection from NFT Stats
	topNFTs, err := nftstatsClient.GetTopNFTs(docID)
	if err != nil {
		logger.Error(err)
	}

	var (
		stats        = collection.Stats
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
		contract     string
		attritubes   []reservoir.Attribute
	)

	if len(collection.PrimaryAssetContracts) > 0 {
		contract = collection.PrimaryAssetContracts[0].Address
	}

	// Fetch attribute floors from Reservoir
	if contract != "" && floor >= 0.01 {
		attritubes = reservoirClient.GetAllAttributesForContract(contract)
	}

	if collection.Slug != "" {
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
			{Path: "thumb", Value: collection.ImageURL},
			{Path: "updated", Value: now},
			{Path: "topNFTs", Value: topNFTs},
			{Path: "contract", Value: contract},
			{Path: "attributes", Value: adaptAttributes(attritubes)},
		})
		if err != nil {
			logger.Error(err)
		}

		// bq.RecordCollectionsUpdateInBigQuery(
		// 	bigQueryClient,
		// 	logger,
		// 	docID,
		// 	floor,
		// 	sevenDayVol,
		// 	now,
		// )

		updated = true
	} else {
		logger.Infow("Floor below 0.005", "collection", docID, "floor", floor)
	}

	time.Sleep(time.Millisecond * os.OpenSeaRateLimit)

	return updated
}

func AddCollectionToDB(
	ctx context.Context,
	openSeaClient *opensea.OpenSeaClient,
	nftFloorPriceClient *nftfloorprice.NFTFloorPriceClient,
	logger *zap.SugaredLogger,
	database *firestore.Client,
	slug string,
) (float64, bool) {
	var err error
	// If slug is in collectionDenylist, return
	if utils.Contains(collectionDenylist, slug) {
		logger.Infow("Collection is in denylist", "collection", slug)
		return 0, false
	}
	// Add collection to db
	c := Collection{
		Updated: time.Now(),
	}
	floor := 0.0
	// Get collection from OpenSea
	floor = getCollectionFromOpenSeaAndUpdateC(&c, slug, logger, openSeaClient)
	if slug == "cryptopunks" {
		// Fetch floor from NFT Floor Price
		floor, err = nftFloorPriceClient.GetFloorPriceFromCollection(slug)
		if err != nil {
			logger.Error(err)
		}

		logger.Infow("Fetched floor from NFT Floor Price", "slug", slug, "floor", floor)
		c.Floor = floor
	}

	// Add collection to db
	if floor > 0.0 && floor <= MaxFloorPrice {
		_, err = database.Collection("collections").Doc(slug).Set(ctx, c)
		if err != nil {
			logger.Error(err)
			return floor, false
		}
	} else {
		logger.Infow("Floor was 0", "slug", slug)
		return floor, false
	}

	return floor, true
}

func getCollectionFromOpenSeaAndUpdateC(c *Collection, slug string, logger *zap.SugaredLogger, openSeaClient *opensea.OpenSeaClient) float64 {
	// Get collection from OpenSea
	collection, err := openSeaClient.GetCollection(slug)
	stat := collection.Stats
	if err != nil {
		logger.Error(err)
		return 0.0
	}

	logger.Infow("Fetched floor price from OpenSea", "collection", slug, "floor", stat.FloorPrice)

	c.Floor = stat.FloorPrice
	c.MarketCap = stat.MarketCap
	c.NumOwners = stat.NumOwners
	c.OneDayVolume = stat.OneDayVolume
	c.SevenDayVolume = stat.SevenDayVolume
	c.ThirtyDayVolume = stat.ThirtyDayVolume
	c.Thumb = collection.ImageURL
	c.TotalSales = stat.TotalSales
	c.TotalSupply = stat.TotalSupply
	c.Slug = slug
	c.Name = collection.Name

	return stat.FloorPrice
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

func GetCollection(ctx context.Context, logger *zap.SugaredLogger, database *firestore.Client, slug string) Collection {
	var c Collection
	docsnap, err := database.Collection("collections").Doc(slug).Get(ctx)
	if err != nil {
		logger.Errorw(
			"Error fetching collection from Firestore",
			"err", err,
		)
		return c
	}

	err = docsnap.DataTo(&c)
	if err != nil {
		logger.Errorw(
			"Error casting collection from Firestore",
			"err", err,
		)
		return c
	}

	return c
}

func GetTopNFTs(ctx context.Context, logger *zap.SugaredLogger, nftstatsClient *nftstats.NFTStatsClient, slug string) []TopNFT {
	// Call NFT Stats API
	nfts, err := nftstatsClient.GetTopNFTs(slug)
	if err != nil {
		logger.Errorw(
			"Error fetching top NFTs from NFT Stats",
			"err", err,
		)
	}
	var resp []TopNFT
	for _, nft := range nfts {
		resp = append(resp, TopNFT{
			Image:  nft.Image,
			Name:   nft.Name,
			OSLink: nft.OSLink,
		})
	}

	return resp
}

func adaptAttributes(attrs []reservoir.Attribute) []Attribute {
	var (
		resp  []Attribute
		floor float64
		image string
	)

	for _, attr := range attrs {
		if len(attr.FloorAskPrices) > 0 && attr.FloorAskPrices[0] > 0 {
			// Convert to float64
			floor = attr.FloorAskPrices[0]
			if len(attr.SampleImages) > 0 {
				image = attr.SampleImages[0]
			}
			resp = append(resp, Attribute{
				Key:   attr.Key,
				Value: attr.Value,
				Floor: floor,
				Image: image,
			})
		}
	}
	return resp

}
