// Package handlers wires intents + WAHA events to the upstream APIs and
// formats the WhatsApp replies. It implements waha.Handler so the webhook
// receiver can dispatch directly into it.
package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/pburkhalter/waha-concierge/internal/config"
	"github.com/pburkhalter/waha-concierge/internal/intents"
	"github.com/pburkhalter/waha-concierge/internal/jellyfin"
	"github.com/pburkhalter/waha-concierge/internal/radarr"
	"github.com/pburkhalter/waha-concierge/internal/seerr"
	"github.com/pburkhalter/waha-concierge/internal/sonarr"
	"github.com/pburkhalter/waha-concierge/internal/store"
	"github.com/pburkhalter/waha-concierge/internal/waha"
)

// Bot is the dispatcher. Construct it once at startup and pass to
// waha.Receiver as the Handler.
type Bot struct {
	Cfg      *config.Config
	Log      *slog.Logger
	WAHA     *waha.Client
	Seerr    *seerr.Client
	Sonarr   *sonarr.Client
	Radarr   *radarr.Client
	Jellyfin *jellyfin.Client
	Store    *store.Store

	// SearchTTL caps how long a numeric reply ("1") remains bound to the
	// most recent suche from the same sender. Keep short to avoid stale
	// replies firing requests for the wrong title.
	SearchTTL time.Duration

	// WelcomeCooldown suppresses repeat welcomes for the same user on a
	// rejoin within this window. WhatsApp groups occasionally fire join
	// events spuriously (e.g. on participant.changed flicker).
	WelcomeCooldown time.Duration
}

// New returns a Bot with sensible default TTLs.
func New(cfg *config.Config, log *slog.Logger, w *waha.Client, sr *seerr.Client,
	so *sonarr.Client, ra *radarr.Client, jf *jellyfin.Client, st *store.Store) *Bot {
	return &Bot{
		Cfg: cfg, Log: log, WAHA: w, Seerr: sr, Sonarr: so, Radarr: ra,
		Jellyfin: jf, Store: st,
		SearchTTL:       2 * time.Minute,
		WelcomeCooldown: 24 * time.Hour,
	}
}

// OnMessage routes inbound messages through the intents parser.
func (b *Bot) OnMessage(ctx context.Context, ev waha.MessageEvent) error {
	cmd := intents.Parse(ev.Body, b.Cfg.MentionToken())
	if cmd.Kind == intents.KindNone {
		return nil
	}
	b.Log.Info("intent",
		"kind", cmd.Kind.String(),
		"arg", cmd.Arg,
		"chat", ev.From,
		"sender", ev.Participant)
	switch cmd.Kind {
	case intents.KindHelp:
		return b.help(ctx, ev)
	case intents.KindLibrary:
		return b.library(ctx, ev)
	case intents.KindStatus:
		return b.status(ctx, ev)
	case intents.KindNeu:
		return b.neu(ctx, ev)
	case intents.KindIch:
		return b.ich(ctx, ev)
	case intents.KindSuche:
		return b.suche(ctx, ev, cmd.Arg)
	case intents.KindRequest:
		return b.request(ctx, ev, cmd.Arg)
	case intents.KindNumericReply:
		return b.numericReply(ctx, ev, cmd.Arg)
	case intents.KindWerHat:
		return b.werHat(ctx, ev, cmd.Arg)
	case intents.KindStats:
		return b.stats(ctx, ev)
	case intents.KindWartet:
		return b.wartet(ctx, ev)
	}
	return nil
}

// OnGroupJoin fires the welcome message (unless we already greeted this
// user within WelcomeCooldown).
func (b *Bot) OnGroupJoin(ctx context.Context, ev waha.GroupJoinEvent) error {
	for _, p := range ev.Participants {
		ok, err := b.Store.MarkWelcomed(ctx, ev.ID, p.ID, b.WelcomeCooldown)
		if err != nil {
			b.Log.Warn("welcome dedup write failed", "err", err, "user", p.ID)
			continue
		}
		if !ok {
			b.Log.Debug("welcome suppressed by cooldown", "chat", ev.ID, "user", p.ID)
			continue
		}
		if err := b.sendWelcome(ctx, ev.ID, p.ID); err != nil {
			b.Log.Error("welcome send failed", "err", err, "user", p.ID)
		}
	}
	return nil
}

// OnPollVote is wired for the weekly-poll workflow. When a vote arrives on
// a poll the bot remembers, log the choice — actual auto-request behaviour
// lands in the scheduler/poll handler in a follow-up.
func (b *Bot) OnPollVote(ctx context.Context, ev waha.PollVoteEvent) error {
	for _, idx := range ev.SelectedOptions {
		opt, err := b.Store.LookupPoll(ctx, ev.PollID, idx)
		if err != nil {
			continue // not our poll
		}
		b.Log.Info("poll vote", "poll_id", ev.PollID, "voter", ev.VoterID, "title", opt.Title)
	}
	return nil
}

// reply sends a text message back to the same chat the trigger came from.
// Replies don't include @mentions back at the user — too noisy.
func (b *Bot) reply(ctx context.Context, ev waha.MessageEvent, text string) error {
	_, err := b.WAHA.SendText(ctx, ev.From, text, nil)
	if err != nil {
		b.Log.Error("reply failed", "err", err, "chat", ev.From)
	}
	return err
}

// truncate clips long titles/queries in WhatsApp replies.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// fmtBytes turns 4_294_967_296 into "4.0 GB".
func fmtBytes(n int64) string {
	const u = 1024
	if n < u {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(u), 0
	for nn := n / u; nn >= u; nn /= u {
		div *= u
		exp++
	}
	unit := "KMGTPE"[exp : exp+1]
	return fmtFloat(float64(n)/float64(div), 1) + " " + unit + "B"
}

func fmtFloat(f float64, dec int) string {
	return strconv.FormatFloat(f, 'f', dec, 64)
}

// fmtPercent computes "(size-left / size)" inverted as a 0..100 progress
// string like "73%". Returns "" when size is zero (unknown).
func fmtPercent(size, sizeLeft int64) string {
	if size <= 0 {
		return ""
	}
	pct := int(100 * (size - sizeLeft) / size)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return strconv.Itoa(pct) + "%"
}

// trimQuery normalises query-style arguments before hitting upstream APIs.
func trimQuery(s string) string { return strings.TrimSpace(s) }
