package radarr

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
		w.Write([]byte(`{"records":[{"id":1,"movieId":7,"title":"Dune","status":"downloading"}]}`))
	})
	got, err := c.Queue(context.Background())
	if err != nil || len(got) != 1 || got[0].MovieID != 7 {
		t.Fatalf("queue: %v %#v", err, got)
	}
}

func TestRecentImports(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("eventType") != "1" {
			t.Fatalf("eventType %s", r.URL.Query().Get("eventType"))
		}
		w.Write([]byte(`{"records":[{"id":1,"movieId":5,"eventType":"downloadFolderImported","movie":{"title":"Dune","tmdbId":438631}}]}`))
	})
	got, err := c.RecentImports(context.Background(), 5)
	if err != nil || len(got) != 1 || got[0].Movie.Title != "Dune" || got[0].Movie.ImageURL == "" {
		t.Fatalf("history: %v %#v", err, got)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`boom`))
	})
	_, err := c.Queue(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 500 {
		t.Fatalf("want APIError 500, got %v", err)
	}
}
