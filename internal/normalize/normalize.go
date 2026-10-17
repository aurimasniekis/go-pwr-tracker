// Package normalize derives conservative identity keys for artists and
// tracks. Display values are preserved elsewhere; keys are only used to decide
// whether two observations refer to the same artist or track.
//
// The rules are intentionally minimal:
//   - Unicode is normalized to NFC so composed and decomposed forms match.
//   - Leading/trailing whitespace is trimmed and internal runs collapsed.
//   - Case is folded with Unicode full case folding.
//
// Nothing else is removed or rewritten: remix/edit/version suffixes,
// "feat." credits, punctuation and diacritics all remain part of the key, and
// collaborative artist strings ("A & B", "A x B") are never split.
package normalize

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var folder = cases.Fold()

// Display trims, NFC-normalizes and collapses whitespace without changing
// case or content. It is used for the stored display value.
func Display(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// Key returns the identity key for a single name or title.
func Key(s string) string {
	// Folding can produce decomposed sequences, so normalize again afterwards.
	return norm.NFC.String(folder.String(Display(s)))
}

// TrackKey combines artist and title keys into a track identity.
func TrackKey(artist, title string) string {
	return Key(artist) + "\x1f" + Key(title)
}
