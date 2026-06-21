// Package jellyfin is a thin Jellyfin client. The bot only needs read access
// to surface "recently added" and library counts, plus a stable poster URL
// builder for WAHA's sendImage call.
package jellyfin

import (
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

type Client struct {
	BaseURL string
	APIKey  string
	UserID  string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey, userID string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		UserID:  userID,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type APIError struct {
	Status int
	Detail string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jellyfin %d: %s", e.Status, e.Detail)
}

type RecentItem struct {
	ID             string    `json:"Id"`
	Name           string    `json:"Name"`
	Type           string    `json:"Type"`
	SeriesName     string    `json:"SeriesName"`
	SeasonName     string    `json:"SeasonName"`
	Overview       string    `json:"Overview"`
	ProductionYear int       `json:"ProductionYear"`
	DateCreated    time.Time `json:"DateCreated"`
	PosterURL      string    `json:"-"`
}

type recentEnvelope struct {
	Items []RecentItem `json:"Items"`
}

func (c *Client) RecentlyAdded(ctx context.Context, n int) ([]RecentItem, error) {
	if n <= 0 {
		n = 20
	}
	q := url.Values{}
	q.Set("SortBy", "DateCreated")
	q.Set("SortOrder", "Descending")
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", "Movie,Episode")
	q.Set("Limit", strconv.Itoa(n))
	q.Set("Fields", "Overview,SeriesName,SeasonName,ProductionYear,DateCreated")
	var env recentEnvelope
	if err := c.get(ctx, "/Users/"+url.PathEscape(c.UserID)+"/Items?"+q.Encode(), &env); err != nil {
		return nil, err
	}
	for i := range env.Items {
		env.Items[i].PosterURL = c.PosterURL(env.Items[i].ID)
	}
	return env.Items, nil
}

type Counts struct {
	Movie   int `json:"MovieCount"`
	Series  int `json:"SeriesCount"`
	Episode int `json:"EpisodeCount"`
	Album   int `json:"AlbumCount"`
}

func (c *Client) Counts(ctx context.Context) (*Counts, error) {
	var out Counts
	if err := c.get(ctx, "/Items/Counts", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PosterURL is the URL handed to WAHA's sendImage. The api_key query param is
// included so it works even when the Jellyfin instance disallows anonymous
// image access — WAHA fetches the URL server-side, never client-facing, so
// leaking the key over WhatsApp isn't a concern.
func (c *Client) PosterURL(itemID string) string {
	q := url.Values{}
	q.Set("maxWidth", "500")
	if c.APIKey != "" {
		q.Set("api_key", c.APIKey)
	}
	return fmt.Sprintf("%s/Items/%s/Images/Primary?%s", c.BaseURL, url.PathEscape(itemID), q.Encode())
}

// Lookup is the slim result of a TMDB→Jellyfin id resolution.
type Lookup struct {
	ID       string
	Name     string
	Type     string // "Movie" | "Series"
	ServerID string
}

// LibraryCounts returns the number of items in each VirtualFolder
// (Filme / Serien / ...). Strictly per-library — orphan rows that live
// in the global Items table but no longer belong to any active library
// are excluded, so the numbers match what users see in the app.
type LibraryCounts struct {
	Movies   int
	Series   int
	Episodes int
}

// CountLibrary returns the number of items of `itemType` ("Movie",
// "Series", "Episode") that resolve under the given parent library id.
// Used by the stats command to avoid the misleading global /Items/Counts
// total (which includes orphans from old/destroyed libraries).
func (c *Client) CountLibrary(ctx context.Context, parentID, itemType string) (int, error) {
	q := url.Values{}
	q.Set("ParentId", parentID)
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", itemType)
	q.Set("Limit", "0") // we only want the count, not the rows
	path := fmt.Sprintf("/Users/%s/Items?%s", c.UserID, q.Encode())
	var env struct {
		TotalRecordCount int `json:"TotalRecordCount"`
	}
	if err := c.get(ctx, path, &env); err != nil {
		return 0, err
	}
	return env.TotalRecordCount, nil
}

// FindByTMDB resolves a TMDB id to the matching Jellyfin item. Returns
// ErrNotFound when Jellyfin hasn't scanned the title yet (notification
// fired before the library scan finished).
func (c *Client) FindByTMDB(ctx context.Context, tmdbID int, mediaType string) (*Lookup, error) {
	if tmdbID <= 0 {
		return nil, ErrNotFound
	}
	jfType := "Movie"
	if mediaType == "tv" || mediaType == "series" {
		jfType = "Series"
	}
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("IncludeItemTypes", jfType)
	q.Set("AnyProviderIdEquals", fmt.Sprintf("Tmdb.%d", tmdbID))
	q.Set("Fields", "ProviderIds")
	q.Set("Limit", "1")
	path := fmt.Sprintf("/Users/%s/Items?%s", c.UserID, q.Encode())

	var env struct {
		Items []struct {
			ID       string `json:"Id"`
			Name     string `json:"Name"`
			Type     string `json:"Type"`
			ServerID string `json:"ServerId"`
		} `json:"Items"`
	}
	if err := c.get(ctx, path, &env); err != nil {
		return nil, err
	}
	if len(env.Items) == 0 {
		return nil, ErrNotFound
	}
	it := env.Items[0]
	return &Lookup{ID: it.ID, Name: it.Name, Type: it.Type, ServerID: it.ServerID}, nil
}

// ErrNotFound signals "nothing matched". Callers fall back to a library
// landing link rather than a per-item deep link.
var ErrNotFound = errors.New("jellyfin: not found")

// ItemWebURL builds the Jellyfin web client URL for a given item id. The
// `externalURL` is the user-facing host (e.g. https://jellyfin.home....).
// Returns "" when no externalURL is set so callers can decide what to
// emit instead.
func ItemWebURL(externalURL, itemID, serverID string) string {
	if externalURL == "" || itemID == "" {
		return ""
	}
	q := url.Values{}
	q.Set("id", itemID)
	if serverID != "" {
		q.Set("serverId", serverID)
	}
	return fmt.Sprintf("%s/web/#/details?%s", externalURL, q.Encode())
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("X-Emby-Token", c.APIKey)
	req.Header.Set("Accept", "application/json")
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
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
