package waha

import "testing"

func TestParsePhoneFromJID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"41791112233@c.us", "41791112233"},
		{"41791112233@s.whatsapp.net", "41791112233"},
		{"120363@g.us", ""},
		{"", ""},
		{"not-a-jid", ""},
		{"@c.us", ""},
	}
	for _, c := range cases {
		got := ParsePhoneFromJID(c.in)
		if got != c.want {
			t.Errorf("ParsePhoneFromJID(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestIsGroup(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"120363@g.us", true},
		{"41791112233@c.us", false},
		{"41791112233@s.whatsapp.net", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsGroup(c.in); got != c.want {
			t.Errorf("IsGroup(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestFormatMention(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"41791112233@c.us", "@41791112233"},
		{"41791112233@s.whatsapp.net", "@41791112233"},
		{"120363@g.us", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := FormatMention(c.in); got != c.want {
			t.Errorf("FormatMention(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
