package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSearchRoundtrip(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	results := []SearchResult{
		{Slot: 1, TMDBID: 100, MediaType: "movie", Title: "A"},
		{Slot: 2, TMDBID: 200, MediaType: "tv", Title: "B"},
	}
	if err := s.SaveSearch(ctx, "chat", "user", results, time.Minute); err != nil {
		t.Fatalf("SaveSearch: %v", err)
	}
	got, err := s.LookupSearch(ctx, "chat", "user", 2)
	if err != nil {
		t.Fatalf("LookupSearch: %v", err)
	}
	if got.TMDBID != 200 || got.Title != "B" {
		t.Errorf("got %+v", got)
	}
	// Slot the sender didn't pick → ErrNotFound.
	if _, err := s.LookupSearch(ctx, "chat", "user", 7); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	// SaveSearch should *replace* — second save with one entry should leave
	// slot 2 missing from the lookup.
	if err := s.SaveSearch(ctx, "chat", "user", []SearchResult{{Slot: 1, TMDBID: 999, MediaType: "movie", Title: "Z"}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupSearch(ctx, "chat", "user", 2); err != ErrNotFound {
		t.Errorf("expected slot 2 to be gone after replace, got %v", err)
	}
}

func TestWelcomeCooldown(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	ok, err := s.MarkWelcomed(ctx, "chat", "alice", 24*time.Hour)
	if err != nil || !ok {
		t.Fatalf("first welcome should fire: ok=%v err=%v", ok, err)
	}
	ok2, err := s.MarkWelcomed(ctx, "chat", "alice", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok2 {
		t.Error("second welcome within cooldown should suppress")
	}
	ok3, err := s.MarkWelcomed(ctx, "chat", "alice", time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if !ok3 {
		t.Error("welcome after cooldown elapsed should fire again")
	}
}

func TestPollLookup(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	opts := []PollOption{
		{Index: 0, TMDBID: 11, MediaType: "movie", Title: "Sintel"},
		{Index: 1, TMDBID: 22, MediaType: "tv", Title: "Spider-Noir"},
	}
	if err := s.SavePoll(ctx, "p1", opts); err != nil {
		t.Fatal(err)
	}
	got, err := s.LookupPoll(ctx, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.TMDBID != 22 {
		t.Errorf("got %+v", got)
	}
	if _, err := s.LookupPoll(ctx, "p1", 9); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPendingImports(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.EnqueuePendingImport(ctx, "show:1", "Family Guy S20", `{}`); err != nil {
			t.Fatal(err)
		}
	}
	// Nothing is due yet (wait=1h, just-added rows).
	due, err := s.DueImports(ctx, time.Hour)
	if err != nil || len(due) != 0 {
		t.Fatalf("expected 0 due, got %d (err=%v)", len(due), err)
	}
	// With wait=0, everything is due.
	due, err = s.DueImports(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(due["show:1"]) != 5 {
		t.Errorf("expected 5 pending, got %d", len(due["show:1"]))
	}
	ids := make([]int64, 0, 5)
	for _, p := range due["show:1"] {
		ids = append(ids, p.ID)
	}
	if err := s.MarkFlushed(ctx, ids); err != nil {
		t.Fatal(err)
	}
	due, err = s.DueImports(ctx, 0)
	if err != nil || len(due) != 0 {
		t.Errorf("expected 0 due after flush, got %d (err=%v)", len(due), err)
	}
}
