package storage

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"cloud.google.com/go/storage"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	bucketName = "public.floor.report"
)

// ProvideStorage provides a Google Cloud storage client
func ProvideStorage(lc fx.Lifecycle, logger *zap.SugaredLogger) *storage.Client {
	client, err := storage.NewClient(context.TODO())
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	return client
}

var Options = ProvideStorage

func UploadUserMetadata(
	ctx context.Context,
	logger *zap.SugaredLogger,
	sc *storage.Client,
	r *http.Request,
) bool {
	// Limit file size to 10MB
	r.ParseMultipartForm(10 << 20)

	address := strings.ToLower(r.FormValue("address"))
	logger.Infow("Updating avatar for user", "address", address)

	// Get file from formdata
	file, _, err := r.FormFile("file")
	if err != nil {
		return false
	}
	defer file.Close()

	b := sc.Bucket(bucketName)

	// Convert file to []byte
	buf := make([]byte, 1024*1024)
	n, err := file.Read(buf)
	if err != nil {
		logger.Error(err)
		return false
	}

	// Upload file to bucket
	w := b.Object(fmt.Sprintf("%s.png", address)).NewWriter(ctx)
	w.CacheControl = "no-cache"
	w.Write(buf[:n])
	if err := w.Close(); err != nil {
		logger.Error(err)
		return false
	}

	logger.Infow("Successfully updated metadata for user", "address", address)
	return true
}
