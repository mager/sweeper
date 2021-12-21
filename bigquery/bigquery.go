package bigquery

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/bigquery"
	"go.uber.org/zap"
)

type BQInfoRequestRecord struct {
	Address       string
	NumNFTs       int
	UnrealizedBag float64
	RequestTime   time.Time
}

type BQCollectionsUpdateRecord struct {
	Slug           string
	Floor          float64
	SevenDayVolume float64
	RequestTime    time.Time
}

// ProvideBQ provides a bigquery client
func ProvideBQ() *bigquery.Client {
	projectID := "floor-report-327113"

	client, err := bigquery.NewClient(context.TODO(), projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return client
}

var Options = ProvideBQ

// RecordRequestInBigQuery posts a info request event to BigQuery
func RecordRequestInBigQuery(
	dataset *bigquery.Dataset,
	logger *zap.SugaredLogger,
	address string,
) {
	var (
		ctx   = context.Background()
		table = dataset.Table("requests")
		u     = table.Inserter()

		items = []*BQInfoRequestRecord{
			{
				Address: address,
				// NumNFTs:       numNFTs,
				// UnrealizedBag: unrealizedBag,
				RequestTime: time.Now(),
			},
		}
	)
	if err := u.Put(ctx, items); err != nil {
		logger.Error(err)
	}
}

// RecordCollectionsUpdateInBigQuery posts a info request event to BigQuery
func RecordCollectionsUpdateInBigQuery(
	bq *bigquery.Client,
	logger *zap.SugaredLogger,
	slug string,
	floor float64,
	sevenDayVol float64,
	t time.Time,
) {
	var (
		ctx     = context.Background()
		dataset = bq.DatasetInProject("floor-report-327113", "collections")
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
		logger.Error(err)
	}
}
