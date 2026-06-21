package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pburkhalter/waha-concierge/internal/seerr"
	"github.com/pburkhalter/waha-concierge/internal/store"
	"github.com/pburkhalter/waha-concierge/internal/waha"
)

// ─── inbound webhook payloads (Sonarr/Radarr "Connect → Webhook") ────────

// sonarrWebhook is a tolerant subset of Sonarr's outgoing Connect webhook.
// We only read what we actually use; the real payload has many more fields.
type sonarrWebhook struct {
	EventType string `json:"eventType"`
	Series    struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		TmdbID   int    `json:"tmdbId"`
		TvdbID   int    `json:"tvdbId"`
		ImagesV2 []struct {
			RemoteURL string `json:"remoteUrl"`
			Type      string `json:"coverType"`
		} `json:"images"`
	} `json:"series"`
	Episodes []struct {
		Title         string `json:"title"`
		SeasonNumber  int    `json:"seasonNumber"`
		EpisodeNumber int    `json:"episodeNumber"`
	} `json:"episodes"`
}

// radarrWebhook is a tolerant subset of Radarr's outgoing Connect webhook.
type radarrWebhook struct {
	EventType string `json:"eventType"`
	Movie     struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Year     int    `json:"year"`
		TmdbID   int    `json:"tmdbId"`
		ImagesV2 []struct {
			RemoteURL string `json:"remoteUrl"`
			Type      string `json:"coverType"`
		} `json:"images"`
	} `json:"movie"`
}

// ─── routing ──────────────────────────────────────────────────────────────

// WebhookHandler returns the http.Handler the bot exposes for upstream
// Sonarr+Radarr Connect webhooks. Mount at e.g. /webhook/sonarr and
// /webhook/radarr respectively in main.
func (b *Bot) WebhookHandler(source string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "body", http.StatusBadRequest)
			return
		}
		// Always 200 — upstream retries are noisy and uninformative.
		go func(payload []byte) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := b.processWebhook(ctx, source, payload); err != nil {
				b.Log.Warn("notify webhook failed",
					"err", err, "source", source, "body_len", len(payload))
			}
		}(body)
		w.WriteHeader(http.StatusOK)
	})
}

func (b *Bot) processWebhook(ctx context.Context, source string, payload []byte) error {
	switch source {
	case "sonarr":
		var ev sonarrWebhook
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		return b.handleSonarr(ctx, ev)
	case "radarr":
		var ev radarrWebhook
		if err := json.Unmarshal(payload, &ev); err != nil {
			return err
		}
		return b.handleRadarr(ctx, ev)
	}
	return fmt.Errorf("unknown source %q", source)
}

// ─── sonarr (episodes) ────────────────────────────────────────────────────

// handleSonarr buffers Download events. The flush worker batches them per
// (series, season) before sending so a season import doesn't spam the
// chat with 20 individual pings.
func (b *Bot) handleSonarr(ctx context.Context, ev sonarrWebhook) error {
	if ev.EventType != "Download" && ev.EventType != "Upgrade" {
		return nil
	}
	if len(ev.Episodes) == 0 || ev.Series.Title == "" {
		return nil
	}
	ep := ev.Episodes[0]
	showKey := fmt.Sprintf("sonarr:%d:%d", ev.Series.ID, ep.SeasonNumber)
	displayName := fmt.Sprintf("%s S%02d", ev.Series.Title, ep.SeasonNumber)

	payload := pendingPayload{
		SeriesTitle: ev.Series.Title,
		Season:      ep.SeasonNumber,
		Episode:     ep.EpisodeNumber,
		EpisodeName: ep.Title,
		TmdbID:      ev.Series.TmdbID,
		PosterURL:   pickPoster(ev.Series.ImagesV2, "poster"),
	}
	raw, _ := json.Marshal(payload)
	return b.Store.EnqueuePendingImport(ctx, showKey, displayName, string(raw))
}

// ─── radarr (movies) ──────────────────────────────────────────────────────

// handleRadarr ships a movie notification immediately — no batching since
// movies arrive one at a time and the user expects a fast ping.
func (b *Bot) handleRadarr(ctx context.Context, ev radarrWebhook) error {
	if ev.EventType != "Download" && ev.EventType != "MovieFileImported" {
		return nil
	}
	if ev.Movie.Title == "" {
		return nil
	}
	body, mentions := b.formatMovieNotice(ctx, ev)
	poster := pickPoster(ev.Movie.ImagesV2, "poster")
	if poster != "" {
		_, err := b.WAHA.SendImage(ctx, b.Cfg.WAHAChatID, poster, body, mentions)
		return err
	}
	_, err := b.WAHA.SendText(ctx, b.Cfg.WAHAChatID, body, mentions)
	return err
}

func (b *Bot) formatMovieNotice(ctx context.Context, ev radarrWebhook) (string, []string) {
	year := ""
	if ev.Movie.Year > 0 {
		year = fmt.Sprintf(" (%d)", ev.Movie.Year)
	}
	header := fmt.Sprintf("🎬 *%s%s* ist verfügbar.", ev.Movie.Title, year)
	mention, requester := b.requesterMention(ctx, ev.Movie.TmdbID)
	if mention != "" {
		return header + "\n🎉 " + mention + " — viel Spass!", []string{requester}
	}
	return header + "\n🍿 " + b.Cfg.JellyfinExternalURL, nil
}

// ─── grouping flush ───────────────────────────────────────────────────────

// FlushPending posts buffered Sonarr notifications. Call from a periodic
// ticker (e.g. every 60s) with a `wait` like 10*time.Minute — anything
// older than wait gets grouped and sent.
func (b *Bot) FlushPending(ctx context.Context, wait time.Duration) error {
	groups, err := b.Store.DueImports(ctx, wait)
	if err != nil {
		return err
	}
	for showKey, items := range groups {
		if len(items) == 0 {
			continue
		}
		body, mentions, ids := b.formatEpisodeGroup(ctx, showKey, items)
		if _, err := b.WAHA.SendText(ctx, b.Cfg.WAHAChatID, body, mentions); err != nil {
			b.Log.Warn("flush send failed", "err", err, "show", showKey)
			continue
		}
		if err := b.Store.MarkFlushed(ctx, ids); err != nil {
			b.Log.Warn("mark flushed failed", "err", err, "show", showKey)
		}
	}
	return nil
}

// pendingPayload is the json we stuff into the store while waiting to
// flush. Kept small — only what the format step actually reads.
type pendingPayload struct {
	SeriesTitle string `json:"series_title"`
	Season      int    `json:"season"`
	Episode     int    `json:"episode"`
	EpisodeName string `json:"episode_name"`
	TmdbID      int    `json:"tmdb_id"`
	PosterURL   string `json:"poster_url"`
}

// formatEpisodeGroup turns N buffered single-episode rows into one tidy
// WhatsApp message. Mentions the requester of the series (if any) once
// per group, not per episode.
func (b *Bot) formatEpisodeGroup(ctx context.Context, _ string, items []store.PendingImport) (string, []string, []int64) {
	if len(items) == 0 {
		return "", nil, nil
	}
	ids := make([]int64, 0, len(items))
	displayName := items[0].DisplayName
	tmdbID := 0
	episodes := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
		var p pendingPayload
		_ = json.Unmarshal([]byte(it.PayloadJSON), &p)
		if tmdbID == 0 {
			tmdbID = p.TmdbID
		}
		episodes = append(episodes,
			fmt.Sprintf("  📺 S%02dE%02d — %s", p.Season, p.Episode, truncate(p.EpisodeName, 50)))
	}

	header := fmt.Sprintf("✨ *%s* — %d Episoden hinzugefügt:", displayName, len(items))
	mention, requester := b.requesterMention(ctx, tmdbID)
	var mentions []string
	if mention != "" {
		header += "\n🎉 " + mention
		mentions = []string{requester}
	}
	body := header + "\n\n" + strings.Join(episodes, "\n")
	if len(items) >= 8 {
		// Long lists become noisy in WhatsApp — drop the per-episode rows
		// and link to Jellyfin instead.
		body = header + fmt.Sprintf("\n\n🍿 %s", b.Cfg.JellyfinExternalURL)
	}
	return body, mentions, ids
}

// ─── requester @mention ───────────────────────────────────────────────────

// requesterMention finds the WhatsApp jid for the user who requested a
// given tmdb id. Returns ("@<phone>", "<phone>@c.us") for SendText's
// mentions slice, or ("","") when we can't resolve.
func (b *Bot) requesterMention(ctx context.Context, tmdbID int) (text string, jid string) {
	if tmdbID == 0 {
		return "", ""
	}
	r, err := b.Seerr.FindRequestByTMDB(ctx, tmdbID)
	if errors.Is(err, seerr.ErrNotFound) || err != nil {
		return "", ""
	}
	username := strings.ToLower(r.RequestedBy.JellyfinUserName)
	if username == "" {
		username = strings.ToLower(r.RequestedBy.Username)
	}
	phone := b.Cfg.PhoneMap[username]
	if phone == "" {
		return "", ""
	}
	return "@" + phone, waha.FormatJID(phone)
}

// pickPoster returns the first image URL of the requested coverType, or "".
func pickPoster(images []struct {
	RemoteURL string `json:"remoteUrl"`
	Type      string `json:"coverType"`
}, want string) string {
	for _, im := range images {
		if strings.EqualFold(im.Type, want) {
			return im.RemoteURL
		}
	}
	return ""
}
