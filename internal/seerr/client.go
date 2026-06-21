// Package seerr is a thin Jellyseerr v1 client. The bot uses it to search
// TMDB-backed media, create requests on behalf of WhatsApp users, and look up
// who requested what (for notification enrichment).
package seerr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const posterBase = "https://image.tmdb.org/t/p/w500"

// ErrNotFound is returned by lookup helpers when the requested entity does not
// exist (404 from Seerr or filtered out client-side).
var ErrNotFound = errors.New("not found")

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
	return fmt.Sprintf("seerr %d: %s", e.Status, e.Detail)
}

// SearchResult is one row of the mixed movie/tv result list returned by
// Seerr's /search endpoint. Movies use Title+ReleaseDate, TV uses
// Name+FirstAirDate — DisplayTitle/Year normalise that.
type SearchResult struct {
	ID           int    `json:"id"`
	MediaType    string `json:"mediaType"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	FirstAirDate string `json:"firstAirDate,omitempty"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"posterPath"`
}

func (r SearchResult) DisplayTitle() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

func (r SearchResult) Year() string {
	d := r.ReleaseDate
	if d == "" {
		d = r.FirstAirDate
	}
	if len(d) < 4 {
		return ""
	}
	return d[:4]
}

func (r SearchResult) PosterURL() string {
	if r.PosterPath == "" {
		return ""
	}
	return posterBase + r.PosterPath
}

// searchEnvelope matches Seerr's paginated /search response.
type searchEnvelope struct {
	Results []SearchResult `json:"results"`
}

func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("language", "en")
	var env searchEnvelope
	if err := c.get(ctx, "/search?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Results, nil
}

// RequestSpec is the minimum a caller has to provide to create a request.
// Seasons is only honoured for tv; nil means "all seasons" (Seerr's default).
type RequestSpec struct {
	TMDBID    int
	MediaType string
	Seasons   []int
}

type requestBody struct {
	MediaID   int    `json:"mediaId"`
	MediaType string `json:"mediaType"`
	Seasons   []int  `json:"seasons,omitempty"`
}

type requestResp struct {
	ID int `json:"id"`
}

func (c *Client) Request(ctx context.Context, spec RequestSpec) (int, error) {
	body := requestBody{
		MediaID:   spec.TMDBID,
		MediaType: spec.MediaType,
	}
	if spec.MediaType == "tv" {
		body.Seasons = spec.Seasons
	}
	var out requestResp
	if err := c.post(ctx, "/request", body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

type Request struct {
	ID        int       `json:"id"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Type      string    `json:"type"`
	Media     struct {
		TmdbID int `json:"tmdbId"`
		Status int `json:"status"`
	} `json:"media"`
	RequestedBy struct {
		ID               int    `json:"id"`
		Username         string `json:"username"`
		JellyfinUserName string `json:"jellyfinUsername"`
		DisplayName      string `json:"displayName"`
	} `json:"requestedBy"`
}

type listRequestsEnvelope struct {
	Results []Request `json:"results"`
}

func (c *Client) ListRequests(ctx context.Context, take int) ([]Request, error) {
	if take <= 0 {
		take = 20
	}
	q := url.Values{}
	q.Set("take", strconv.Itoa(take))
	q.Set("sort", "added")
	var env listRequestsEnvelope
	if err := c.get(ctx, "/request?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	return env.Results, nil
}

// MediaTitle resolves a tmdbId to its display title via Seerr's TMDB-proxy
// endpoint (/movie/{id} or /tv/{id}). Seerr's /request response only carries
// the tmdbId, so callers that want a human-readable title need this extra
// hop. Returns ("", nil) when the response has no title/name field rather
// than erroring, so callers can fall back to "TMDB#<id>".
func (c *Client) MediaTitle(ctx context.Context, mediaType string, tmdbID int) (string, error) {
	if tmdbID <= 0 {
		return "", nil
	}
	path := fmt.Sprintf("/movie/%d", tmdbID)
	if mediaType == "tv" {
		path = fmt.Sprintf("/tv/%d", tmdbID)
	}
	var env struct {
		Title string `json:"title"` // movies
		Name  string `json:"name"`  // tv
	}
	if err := c.get(ctx, path, &env); err != nil {
		return "", err
	}
	if env.Title != "" {
		return env.Title, nil
	}
	return env.Name, nil
}

// FindRequestByTMDB scans the most recent requests for a match on tmdbId.
// Seerr lacks a per-tmdbId index, so the bot keeps `take` modest (notifications
// only need to identify the requester of a *just-fulfilled* item).
func (c *Client) FindRequestByTMDB(ctx context.Context, tmdbID int) (*Request, error) {
	reqs, err := c.ListRequests(ctx, 50)
	if err != nil {
		return nil, err
	}
	for i := range reqs {
		if reqs[i].Media.TmdbID == tmdbID {
			return &reqs[i], nil
		}
	}
	return nil, ErrNotFound
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1"+path, nil)
	if err != nil {
		return err
	}
	c.auth(req)
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/v1"+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
