package common

import "fmt"

func GetOpenSeaCollectionURL(docID string) string {
	return fmt.Sprintf("https://opensea.io/collection/%s", docID)
}
