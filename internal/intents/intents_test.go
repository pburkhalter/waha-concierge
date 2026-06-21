package intents

import "testing"

func TestParse(t *testing.T) {
	const mention = "@4179111"

	cases := []struct {
		name string
		body string
		want Command
	}{
		{"empty", "", Command{}},
		{"unaddressed", "hello world", Command{}},
		{"bare mention -> help", "@4179111", Command{Kind: KindHelp, Mention: mention}},
		{"help verb", "@4179111 help", Command{Kind: KindHelp, Mention: mention}},
		{"hilfe alias", "@4179111 hilfe", Command{Kind: KindHelp, Mention: mention}},
		{"suche with query", "@4179111 suche dune", Command{Kind: KindSuche, Arg: "dune", Mention: mention}},
		{"search alias", "@4179111 search Inception", Command{Kind: KindSuche, Arg: "Inception", Mention: mention}},
		{"request alias", "@4179111 request Mortal Kombat II", Command{Kind: KindRequest, Arg: "Mortal Kombat II", Mention: mention}},
		{"status", "@4179111 status", Command{Kind: KindStatus, Mention: mention}},
		{"neu", "@4179111 neu", Command{Kind: KindNeu, Mention: mention}},
		{"library", "@4179111 library", Command{Kind: KindLibrary, Mention: mention}},
		{"wer hat", "@4179111 wer hat Sintel", Command{Kind: KindWerHat, Arg: "Sintel", Mention: mention}},
		{"stats", "@4179111 stats", Command{Kind: KindStats, Mention: mention}},
		{"ich", "@4179111 ich", Command{Kind: KindIch, Mention: mention}},
		{"unknown verb -> suche fallback", "@4179111 Dune", Command{Kind: KindSuche, Arg: "Dune", Mention: mention}},
		{"numeric reply 1", "1", Command{Kind: KindNumericReply, Arg: "1"}},
		{"numeric reply with paren", "(2)", Command{Kind: KindNumericReply, Arg: "2"}},
		{"numeric reply too large", "42", Command{}},
		{"case-insensitive mention", "@4179111 SUCHE foo", Command{Kind: KindSuche, Arg: "foo", Mention: mention}},
		{"extra whitespace", "  @4179111   help   ", Command{Kind: KindHelp, Mention: mention}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.body, mention, false)
			if got != tc.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tc.body, got, tc.want)
			}
		})
	}
}

func TestKindString(t *testing.T) {
	if KindSuche.String() != "suche" {
		t.Fatal("Kind.String broken")
	}
	if KindNone.String() != "none" {
		t.Fatal("KindNone.String broken")
	}
}
