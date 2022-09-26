package sweeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mager/sweeper/config"
	"github.com/mager/sweeper/database"
	os "github.com/mager/sweeper/opensea"
	"go.uber.org/zap"
)

type SweeperClient struct {
	httpClient *http.Client
	logger     *zap.SugaredLogger
	basePath   string
}

// ProvideSweeper provides an HTTP client
func ProvideSweeper(cfg config.Config, logger *zap.SugaredLogger) *SweeperClient {
	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}

	return &SweeperClient{
		httpClient: &http.Client{
			Transport: tr,
		},
		logger:   logger,
		basePath: cfg.SweeperHost,
	}
}

var Options = ProvideSweeper

type UpdateResp struct {
	Success    bool                `json:"success"`
	Collection database.Collection `json:"collection"`
}

// AddCollection adds a collection to the database
func (s *SweeperClient) AddCollection(slug string) bool {
	u, err := url.Parse(fmt.Sprintf("%s/update", s.basePath))
	if err != nil {
		s.logger.Error(err)
		return false
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	var jsonStr = []byte(fmt.Sprintf("{\"slug\": \"%s\"}", slug))
	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonStr))
	if err != nil {
		s.logger.Error(err)
		return false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(err)
		return false
	}
	defer resp.Body.Close()

	var updateResp UpdateResp
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	if err != nil {

		s.logger.Error(err)
		return false
	}

	return true
}

// AddCollections adds multiple collection to the database
func (s *SweeperClient) AddCollections(slugs []string) bool {
	u, err := url.Parse(fmt.Sprintf("%s/update/collections", s.basePath))
	if err != nil {
		s.logger.Error(err)
		return false
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	var stringSlugs = strings.Join(slugs, "\", \"")
	var jsonStr = []byte(fmt.Sprintf("{\"slugs\": [\"%s\"]}", stringSlugs))
	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonStr))
	if err != nil {
		s.logger.Error(err)
		return false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(err)
		return false
	}
	defer resp.Body.Close()

	var updateResp UpdateResp
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	if err != nil {
		s.logger.Error(err)
		return false
	}

	time.Sleep(os.OpenSeaRateLimit)

	return true
}

// UpdateCollection updates a single collection
func (s *SweeperClient) UpdateCollection(slug string) *UpdateResp {
	updateResp := &UpdateResp{}

	u, err := url.Parse(fmt.Sprintf("%s/update/collection", s.basePath))
	if err != nil {
		s.logger.Error(err)
		return updateResp
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	var jsonStr = []byte(fmt.Sprintf("{\"slug\": \"%s\"}", slug))

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonStr))
	if err != nil {
		s.logger.Error(err)
		return updateResp
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(err)
		return updateResp
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	if err != nil {
		s.logger.Error(err)
		return updateResp
	}

	return updateResp
}

// UpdateUser adds a user to the database
func (s *SweeperClient) UpdateUser(address string) bool {
	u, err := url.Parse(fmt.Sprintf("%s/update/user", s.basePath))
	if err != nil {
		s.logger.Error(err)
		return false
	}
	q := u.Query()
	u.RawQuery = q.Encode()

	var jsonStr = []byte(fmt.Sprintf("{\"address\": \"%s\"}", address))
	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonStr))
	if err != nil {
		s.logger.Error(err)
		return false
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error(err)
		return false
	}
	defer resp.Body.Close()

	var updateResp UpdateResp
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	if err != nil {

		s.logger.Error(err)
		return false
	}

	return true
}
