package sweeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

type SweeperClient struct {
	httpClient *http.Client
	logger     *zap.SugaredLogger
	basePath   string
}

// ProvideSweeper provides an HTTP client
func ProvideSweeper(logger *zap.SugaredLogger) *SweeperClient {
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
		basePath: "https://sweeper.floor.report",
	}
}

var Options = ProvideSweeper

type UpdateResp struct {
	Success bool `json:"success"`
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

	time.Sleep(time.Millisecond * 250)

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

	time.Sleep(time.Millisecond * 250)

	return true
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