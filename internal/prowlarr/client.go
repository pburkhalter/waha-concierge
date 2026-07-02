// Package prowlarr is a thin read-only Prowlarr v1 client. The bot uses it to
// report how much of an indexer's daily grab quota is spent, for the streaming
// dashboard's headroom panel.
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type APIError struct {
	Status int
	Detail string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("prowlarr %d: %s", e.Status, e.Detail)
}

// IndexerStat is one row of /api/v1/indexerstats.indexers.
type IndexerStat struct {
	IndexerID       int    `json:"indexerId"`
	IndexerName     string `json:"indexerName"`
	NumberOfGrabs   int    `json:"numberOfGrabs"`
	NumberOfQueries int    `json:"numberOfQueries"`
}

type indexerStatsResp struct {
	Indexers []IndexerStat `json:"indexers"`
}

// GrabsSince returns the grab count for the named indexer in the window
// [since, now]. Prowlarr aggregates this server-side, so we don't page history.
// Returns (count, true) on a match; (0, false) if the indexer isn't present in
// the stats window (e.g. it made no queries today).
func (c *Client) GrabsSince(ctx context.Context, indexerName string, since time.Time) (int, bool, error) {
	q := url.Values{}
	q.Set("startDate", since.UTC().Format("2006-01-02T15:04:05Z"))
	q.Set("endDate", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	var out indexerStatsResp
	if err := c.get(ctx, "/indexerstats?"+q.Encode(), &out); err != nil {
		return 0, false, err
	}
	for _, ix := range out.Indexers {
		if strings.EqualFold(ix.IndexerName, indexerName) {
			return ix.NumberOfGrabs, true, nil
		}
	}
	return 0, false, nil
}

// indexerDef is one row of /api/v1/indexer.
type indexerDef struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
}

// indexerStatusRow is one row of /api/v1/indexerstatus — only indexers that
// are currently failing/disabled appear here.
type indexerStatusRow struct {
	IndexerID    int        `json:"indexerId"`
	DisabledTill *time.Time `json:"disabledTill"`
}

// IndexerAvailable reports whether the named indexer is enabled and not
// currently in a failure back-off. Returns (available, found, err); found is
// false when no indexer matches the name.
func (c *Client) IndexerAvailable(ctx context.Context, indexerName string) (avail bool, found bool, err error) {
	var defs []indexerDef
	if err := c.get(ctx, "/indexer", &defs); err != nil {
		return false, false, err
	}
	id := -1
	enabled := false
	for _, d := range defs {
		if strings.EqualFold(d.Name, indexerName) {
			id, enabled = d.ID, d.Enable
			break
		}
	}
	if id < 0 {
		return false, false, nil
	}
	if !enabled {
		return false, true, nil
	}
	var statuses []indexerStatusRow
	if err := c.get(ctx, "/indexerstatus", &statuses); err != nil {
		return false, true, err
	}
	now := time.Now()
	for _, s := range statuses {
		if s.IndexerID == id && s.DisabledTill != nil && s.DisabledTill.After(now) {
			return false, true, nil // in back-off
		}
	}
	return true, true, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		detail := string(raw)
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return &APIError{Status: resp.StatusCode, Detail: detail}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
