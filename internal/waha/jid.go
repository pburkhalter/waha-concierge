package waha

import "strings"

// JID suffix constants from WhatsApp/Signal-Protocol's addressing scheme.
// `@c.us` is WAHA-style for personal contacts, `@s.whatsapp.net` is the
// upstream wire form — both surface in the same WAHA install depending on
// engine and event source.
const (
	suffixContact = "@c.us"
	suffixSignal  = "@s.whatsapp.net"
	suffixGroup   = "@g.us"
)

// ParsePhoneFromJID returns the digits before "@c.us" / "@s.whatsapp.net".
// Returns "" if the jid isn't a personal phone (e.g. groups, broadcasts).
func ParsePhoneFromJID(jid string) string {
	for _, suf := range []string{suffixContact, suffixSignal} {
		if rest, ok := strings.CutSuffix(jid, suf); ok {
			return rest
		}
	}
	return ""
}

func IsGroup(jid string) bool {
	return strings.HasSuffix(jid, suffixGroup)
}

// FormatMention returns "@<phone>" — the literal token to put inside
// SendText's `text` for WhatsApp to render as a ping when the same jid
// appears in the `mentions` slice. Returns "" for non-personal jids so
// callers can skip silently rather than render garbage.
func FormatMention(jid string) string {
	phone := ParsePhoneFromJID(jid)
	if phone == "" {
		return ""
	}
	return "@" + phone
}

// FormatJID turns a digits-only phone number into a WAHA-style personal
// jid ("<phone>@c.us"). Inverse of ParsePhoneFromJID.
func FormatJID(phone string) string {
	if phone == "" {
		return ""
	}
	return phone + suffixContact
}
