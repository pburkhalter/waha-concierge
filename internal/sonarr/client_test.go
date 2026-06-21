package sonarr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "k", time.Second)
}

func TestQueue(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/queue" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"records":[{"id":1,"seriesId":2,"episodeId":3,"title":"S01E01","status":"downloading","sizeleft":1000}]}`))
	})
	got, err := c.Queue(context.Background())
	if err != nil || len(got) != 1 || got[0].EpisodeID != 3 || got[0].SizeLeft != 1000 {
		t.Fatalf("queue: %v %#v", err, got)
	}
}

func TestRecentImports(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("eventType") != "3" {
			t.Fatalf("eventType %s", r.URL.Query().Get("eventType"))
		}
		w.Write([]byte(`{"records":[{"id":10,"seriesId":7,"eventType":"downloadFolderImported","series":{"title":"Show"},"episode":{"seasonNumber":1,"episodeNumber":2}}]}`))
	})
	got, err := c.RecentImports(context.Background(), 5)
	if err != nil || len(got) != 1 || got[0].Series.Title != "Show" || got[0].Series.ImageURL == "" {
		t.Fatalf("history: %v %#v", err, got)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`nope`))
	})
	_, err := c.Queue(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("want APIError 401, got %v", err)
	}
}
