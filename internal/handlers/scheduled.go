package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pburkhalter/waha-concierge/internal/store"
)

// WeeklyDigest posts a "this week in the library" summary every Sunday
// (or whenever the scheduler fires it). Pulls the last 7 days from
// Jellyfin's recently-added.
func (b *Bot) WeeklyDigest(ctx context.Context) error {
	items, err := b.Jellyfin.RecentlyAdded(ctx, 50)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var movies, episodes []string
	episodeShows := map[string]int{}
	for _, it := range items {
		if it.DateCreated.Before(cutoff) {
			continue
		}
		switch it.Type {
		case "Movie":
			year := ""
			if it.ProductionYear > 0 {
				year = fmt.Sprintf(" (%d)", it.ProductionYear)
			}
			movies = append(movies, "  🎬 "+truncate(it.Name, 50)+year)
		case "Episode":
			name := it.SeriesName
			if name == "" {
				name = "?"
			}
			episodeShows[name]++
		}
	}
	for show, n := range episodeShows {
		episodes = append(episodes, fmt.Sprintf("  📺 %s — %d Episoden", show, n))
	}
	if len(movies) == 0 && len(episodes) == 0 {
		// Stay quiet on slow weeks instead of posting an empty message.
		return nil
	}
	var sections []string
	if len(movies) > 0 {
		sections = append(sections, "*Filme*\n"+strings.Join(movies, "\n"))
	}
	if len(episodes) > 0 {
		sections = append(sections, "*Serien*\n"+strings.Join(episodes, "\n"))
	}
	body := "📅 *Diese Woche neu auf Jellyfin:*\n\n" + strings.Join(sections, "\n\n")
	_, err = b.WAHA.SendText(ctx, b.Cfg.WAHAChatID, body, nil)
	return err
}

// WeeklyPoll posts a poll asking the group what to watch tonight. Options
// are the 4 most-recently-added movies. Poll mapping is stored so a vote
// later can be turned into a "tonight's pick" announcement.
func (b *Bot) WeeklyPoll(ctx context.Context) error {
	items, err := b.Jellyfin.RecentlyAdded(ctx, 20)
	if err != nil {
		return err
	}
	// Pick the 4 most-recent movies.
	var picks []string
	var opts []store.PollOption
	for _, it := range items {
		if it.Type != "Movie" {
			continue
		}
		label := truncate(it.Name, 50)
		if it.ProductionYear > 0 {
			label = fmt.Sprintf("%s (%d)", label, it.ProductionYear)
		}
		picks = append(picks, label)
		opts = append(opts, store.PollOption{
			Index:     len(opts),
			MediaType: "movie",
			Title:     it.Name,
		})
		if len(picks) >= 4 {
			break
		}
	}
	if len(picks) < 2 {
		return nil // not enough material for a poll
	}
	pollID, err := b.WAHA.SendPoll(ctx, b.Cfg.WAHAChatID,
		"🎬 Was schauen wir heute Abend?", picks, false)
	if err != nil {
		return err
	}
	return b.Store.SavePoll(ctx, pollID, opts)
}

// DailyHealth checks for stuck items in Sonarr/Radarr (importBlocked,
// long-queued). Posts only if there's something actionable so the group
// isn't pinged with "all good" noise every morning.
func (b *Bot) DailyHealth(ctx context.Context) error {
	sq, _ := b.Sonarr.Queue(ctx)
	rq, _ := b.Radarr.Queue(ctx)
	stuck := 0
	var lines []string
	for _, it := range sq {
		if it.TrackedDownloadState == "importBlocked" || it.Status == "warning" || it.Status == "error" {
			stuck++
			lines = append(lines, "  📺 "+truncate(it.Title, 50)+" — "+it.TrackedDownloadState)
		}
	}
	for _, it := range rq {
		if it.TrackedDownloadState == "importBlocked" || it.Status == "warning" || it.Status == "error" {
			stuck++
			lines = append(lines, "  🎬 "+truncate(it.Title, 50)+" — "+it.TrackedDownloadState)
		}
	}
	if stuck == 0 {
		return nil
	}
	body := fmt.Sprintf("🩺 *%d Items hängen:*\n%s", stuck, strings.Join(lines, "\n"))
	_, err := b.WAHA.SendText(ctx, b.Cfg.WAHAChatID, body, nil)
	return err
}
