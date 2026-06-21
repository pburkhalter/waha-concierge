// Package radarr is a thin Radarr v3 client. Mirrors sonarr's surface but for
// movies — separate package because the queue/history payloads differ in
// nested-entity shape (Movie vs Series+Episode).
package radarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	return fmt.Sprintf("radarr %d: %s", e.Status, e.Detail)
}

type QueueItem struct {
	ID                   int    `json:"id"`
	MovieID              int    `json:"movieId"`
	Title                string `json:"title"`
	Status               string `json:"status"`
	TrackedDownloadState string `json:"trackedDownloadState"`
	Size                 int64  `json:"size"`
	SizeLeft             int64  `json:"sizeleft"`
	Timeleft             string `json:"timeleft"`
	Protocol             string `json:"protocol"`
	DownloadClient       string `json:"downloadClient"`
}

type queueEnvelope struct {
	Records []QueueItem `json:"records"`
}

func (c *Client) Queue(ctx context.Context) ([]QueueItem, error) {
	q := url.Values{}
	q.Set("pageSize", "50")
	q.Set("includeMovie", "true")
	var env queueEnvelope
	if err := c.get(ctx, "/queue?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Records, nil
}

type HistoryItem struct {
	ID          int       `json:"id"`
	MovieID     int       `json:"movieId"`
	Date        time.Time `json:"date"`
	EventType   string    `json:"eventType"`
	SourceTitle string    `json:"sourceTitle"`
	Movie       struct {
		Title    string `json:"title"`
		TmdbID   int    `json:"tmdbId"`
		ImageURL string `json:"-"`
	} `json:"movie"`
}

type historyEnvelope struct {
	Records []HistoryItem `json:"records"`
}

func (c *Client) RecentImports(ctx context.Context, take int) ([]HistoryItem, error) {
	if take <= 0 {
		take = 20
	}
	q := url.Values{}
	q.Set("pageSize", strconv.Itoa(take))
	q.Set("sortKey", "date")
	q.Set("sortDirection", "descending")
	q.Set("eventType", "1")
	var env historyEnvelope
	if err := c.get(ctx, "/history?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	for i := range env.Records {
		env.Records[i].Movie.ImageURL = fmt.Sprintf("%s/MediaCover/%d/poster.jpg", c.BaseURL, env.Records[i].MovieID)
	}
	return env.Records, nil
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v3"+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
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
		return &APIError{Status: resp.StatusCode, Detail: truncate(string(raw), 200)}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode: %w (body=%s)", err, truncate(string(raw), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
