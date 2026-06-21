package seerr

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

func TestSearch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" || r.URL.Query().Get("query") != "dune" {
			t.Fatalf("unexpected req %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"results":[{"id":438631,"mediaType":"movie","title":"Dune","releaseDate":"2021-10-22","posterPath":"/p.jpg"}]}`))
	})
	got, err := c.Search(context.Background(), "dune")
	if err != nil || len(got) != 1 || got[0].DisplayTitle() != "Dune" || got[0].Year() != "2021" || got[0].PosterURL() != posterBase+"/p.jpg" {
		t.Fatalf("search: %v %#v", err, got)
	}
}

func TestRequest(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":42}`))
	})
	id, err := c.Request(context.Background(), RequestSpec{TMDBID: 1, MediaType: "movie"})
	if err != nil || id != 42 {
		t.Fatalf("request: %v %d", err, id)
	}
}

func TestListRequestsAndFind(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"id":1,"media":{"tmdbId":99}},{"id":2,"media":{"tmdbId":7}}]}`))
	})
	all, err := c.ListRequests(context.Background(), 5)
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %v %v", err, all)
	}
	got, err := c.FindRequestByTMDB(context.Background(), 7)
	if err != nil || got.ID != 2 {
		t.Fatalf("find: %v %#v", err, got)
	}
}

func TestFindRequestByTMDBNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[]}`))
	})
	_, err := c.FindRequestByTMDB(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte(`boom`))
	})
	_, err := c.Search(context.Background(), "x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Fatalf("want APIError 503, got %v", err)
	}
}
