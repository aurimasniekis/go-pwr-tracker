package normalize

import "testing"

func TestDisplay(t *testing.T) {
	cases := map[string]string{
		"  Leon Somov   &  Kaia ":   "Leon Somov & Kaia",
		"Tab\tand\nnewline":         "Tab and newline",
		"Bell\u0007char":            "Bellchar",
		"Z\u030celvis":              "Želvis", // decomposed Ž → composed
		"Song (Extended Mix) ":      "Song (Extended Mix)",
		"\u00a0non-breaking\u00a0 ": "non-breaking",
	}
	for in, want := range cases {
		if got := Display(in); got != want {
			t.Errorf("Display(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyMatchesCaseAndUnicodeForms(t *testing.T) {
	pairs := [][2]string{
		{"Leon Somov & Kaia", "  leon somov &  KAIA"},
		{"Ąžuolas", "ĄŽUOLAS"},
		{"Z\u030cemaitija", "Žemaitija"},
		{"Straße", "STRASSE"}, // full case folding
	}
	for _, p := range pairs {
		if Key(p[0]) != Key(p[1]) {
			t.Errorf("Key(%q)=%q != Key(%q)=%q", p[0], Key(p[0]), p[1], Key(p[1]))
		}
	}
}

func TestKeyKeepsMeaningfulDifferences(t *testing.T) {
	distinct := [][2]string{
		{"Song", "Song (Remix)"},
		{"Song", "Song - Radio Edit"},
		{"Artist", "Artist feat. Other"},
		{"A & B", "A and B"},
		{"Žemaitija", "Zemaitija"}, // diacritics are significant
	}
	for _, p := range distinct {
		if Key(p[0]) == Key(p[1]) {
			t.Errorf("Key(%q) unexpectedly equals Key(%q)", p[0], p[1])
		}
	}
}

func TestTrackKeyDoesNotCollideAcrossFields(t *testing.T) {
	if TrackKey("A B", "C") == TrackKey("A", "B C") {
		t.Fatal("track keys collide when words move between artist and title")
	}
}
