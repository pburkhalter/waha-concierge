package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/pburkhalter/waha-concierge/internal/waha"
)

// status reports the current Sonarr + Radarr queues. Sonarr items get
// "Show — S01E03 — 73%, 4m left" formatting; Radarr is the same minus the
// episode segment. Items the user can't act on (importBlocked, error) are
// suffixed with an emoji marker so it's easy to scan.
func (b *Bot) status(ctx context.Context, ev waha.MessageEvent) error {
	sq, sErr := b.Sonarr.Queue(ctx)
	rq, rErr := b.Radarr.Queue(ctx)
	if sErr != nil && rErr != nil {
		return b.reply(ctx, ev, "⚠️ Konnte Queue nicht abfragen.")
	}

	var lines []string
	for _, it := range sq {
		lines = append(lines, "  📺 "+formatQueueRow(it.Title, it.Status, it.TrackedDownloadState, it.Timeleft, fmtPercent(it.Size, it.SizeLeft)))
	}
	for _, it := range rq {
		lines = append(lines, "  🎬 "+formatQueueRow(it.Title, it.Status, it.TrackedDownloadState, it.Timeleft, fmtPercent(it.Size, it.SizeLeft)))
	}

	if len(lines) == 0 {
		return b.reply(ctx, ev, "✅ Queue ist leer — nichts läuft gerade.")
	}
	body := fmt.Sprintf("⏳ *Aktuelle Downloads (%d):*\n", len(lines)) + strings.Join(lines, "\n")
	return b.reply(ctx, ev, body)
}

// wartet returns just the items that are queued/waiting (not actively
// downloading). Helps the group see "what's stuck".
func (b *Bot) wartet(ctx context.Context, ev waha.MessageEvent) error {
	sq, _ := b.Sonarr.Queue(ctx)
	rq, _ := b.Radarr.Queue(ctx)

	var lines []string
	for _, it := range sq {
		if it.Status == "queued" || it.TrackedDownloadState == "importBlocked" || it.Status == "warning" {
			lines = append(lines, "  📺 "+formatQueueRow(it.Title, it.Status, it.TrackedDownloadState, it.Timeleft, ""))
		}
	}
	for _, it := range rq {
		if it.Status == "queued" || it.TrackedDownloadState == "importBlocked" || it.Status == "warning" {
			lines = append(lines, "  🎬 "+formatQueueRow(it.Title, it.Status, it.TrackedDownloadState, it.Timeleft, ""))
		}
	}
	if len(lines) == 0 {
		return b.reply(ctx, ev, "✅ Kein Item wartet aktuell.")
	}
	return b.reply(ctx, ev, "🕗 *Wartet:*\n"+strings.Join(lines, "\n"))
}

// formatQueueRow shapes one line. The state markers and the time-left are
// emoji-prefixed so a long row still scans visually.
func formatQueueRow(title, status, downloadState, timeLeft, pct string) string {
	t := truncate(title, 50)
	var bits []string
	bits = append(bits, t)
	if pct != "" {
		bits = append(bits, pct)
	}
	if timeLeft != "" && timeLeft != "00:00:00" {
		bits = append(bits, timeLeft+" left")
	}
	switch downloadState {
	case "importBlocked":
		bits = append(bits, "⚠️ import blockiert")
	case "importPending":
		bits = append(bits, "⏳ wartet auf Import")
	}
	switch status {
	case "warning":
		bits = append(bits, "⚠️")
	case "error", "failed":
		bits = append(bits, "❌")
	}
	return strings.Join(bits, " · ")
}
