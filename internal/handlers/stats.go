package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pburkhalter/waha-concierge/internal/seerr"
	"github.com/pburkhalter/waha-concierge/internal/waha"
)

// stats summarises the library + recent request activity. Three lines so
// the message stays scannable.
func (b *Bot) stats(ctx context.Context, ev waha.MessageEvent) error {
	counts, err := b.Jellyfin.Counts(ctx)
	if err != nil {
		return b.reply(ctx, ev, "⚠️ Jellyfin counts fehlgeschlagen.")
	}
	reqs, err := b.Seerr.ListRequests(ctx, 100)
	if err != nil {
		reqs = nil
	}
	// Tally the top requester this week (by request count).
	byUser := map[string]int{}
	for _, r := range reqs {
		byUser[r.RequestedBy.DisplayName]++
	}
	top := ""
	topN := 0
	for u, n := range byUser {
		if n > topN {
			top = u
			topN = n
		}
	}

	body := fmt.Sprintf(`📊 *Library:*
  🎬 %d Filme
  📺 %d Serien (%d Episoden)

🎯 *Letzte 100 Requests:* %d (Top: %s)`,
		counts.Movie, counts.Series, counts.Episode, len(reqs), top)
	return b.reply(ctx, ev, body)
}

// ich shows the calling user's account snapshot. Pulled by matching their
// WhatsApp phone against the PhoneMap — empty mapping means we don't know
// who they are.
func (b *Bot) ich(ctx context.Context, ev waha.MessageEvent) error {
	phone := waha.ParsePhoneFromJID(ev.Participant)
	username := lookupUsernameByPhone(b.Cfg.PhoneMap, phone)
	if username == "" {
		return b.reply(ctx, ev,
			"🤔 Ich kenne deine WhatsApp-Nummer noch nicht. Frag den Admin nach `PHONE_MAP_*`-Mapping.")
	}
	reqs, err := b.Seerr.ListRequests(ctx, 200)
	if err != nil {
		return b.reply(ctx, ev, "⚠️ Seerr-Abfrage fehlgeschlagen.")
	}
	mine := []seerr.Request{}
	for _, r := range reqs {
		if strings.EqualFold(r.RequestedBy.JellyfinUserName, username) ||
			strings.EqualFold(r.RequestedBy.Username, username) {
			mine = append(mine, r)
		}
	}
	body := fmt.Sprintf(`👤 *Du:* %s
  🎯 %d Requests in der jüngsten Historie`, username, len(mine))
	return b.reply(ctx, ev, body)
}

// werHat answers "wer hat <titel>?" by searching Seerr for the title and
// then finding the matching request. Falls back to "unbekannt" when the
// item isn't in any tracked request.
func (b *Bot) werHat(ctx context.Context, ev waha.MessageEvent, query string) error {
	q := trimQuery(query)
	if q == "" {
		return b.reply(ctx, ev, "Beispiel: "+b.Cfg.MentionToken()+" wer hat Dune")
	}
	results, err := b.Seerr.Search(ctx, q)
	if err != nil {
		return b.reply(ctx, ev, "⚠️ Suche fehlgeschlagen.")
	}
	top := pickTopMedia(results, 1)
	if len(top) == 0 {
		return b.reply(ctx, ev, "🤷 Keine Treffer für *"+q+"*.")
	}
	r, err := b.Seerr.FindRequestByTMDB(ctx, top[0].ID)
	if errors.Is(err, seerr.ErrNotFound) {
		return b.reply(ctx, ev, fmt.Sprintf("🤷 *%s* wurde von niemandem in der jüngsten Historie angefragt.", top[0].DisplayTitle()))
	}
	if err != nil {
		return b.reply(ctx, ev, "⚠️ Seerr-Abfrage fehlgeschlagen.")
	}
	who := r.RequestedBy.DisplayName
	if who == "" {
		who = r.RequestedBy.JellyfinUserName
	}
	return b.reply(ctx, ev, fmt.Sprintf("👤 *%s* wurde von *%s* angefragt.", top[0].DisplayTitle(), who))
}

// lookupUsernameByPhone returns the username whose mapped phone matches.
func lookupUsernameByPhone(phoneMap map[string]string, phone string) string {
	for u, p := range phoneMap {
		if p == phone {
			return u
		}
	}
	return ""
}
