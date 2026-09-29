package catalog

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// leadingArticles are dropped from sort keys so "The Cure" files under C.
// They are English only; "A" also opens titles in other languages, which then
// sort by their second word. That is accepted until M3 tunes sorting.
var leadingArticles = []string{"the ", "a ", "an "}

// unfoldedLetters are Latin letters with no Unicode decomposition, so
// stripping combining marks leaves them in place and they would sort after z.
var unfoldedLetters = strings.NewReplacer(
	"ø", "o", "Ø", "o", "ł", "l", "Ł", "l", "đ", "d", "Đ", "d",
	"ß", "ss", "æ", "ae", "Æ", "ae", "œ", "oe", "Œ", "oe", "þ", "th", "Þ", "th",
)

// SortKey is the one normalization every catalog writer uses for sort_title,
// sort_name and sort_artist, so lists from different providers interleave
// consistently: case and diacritics are folded ("Dvořák" → "dvorak"), leading
// punctuation is dropped, and one leading English article is removed. A
// title that is all punctuation keeps it rather than sorting as empty.
func SortKey(s string) string {
	folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s)
	if err != nil {
		folded = s
	}
	lower := strings.ToLower(unfoldedLetters.Replace(strings.TrimSpace(folded)))
	key := strings.TrimLeftFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if key == "" {
		return lower
	}
	for _, article := range leadingArticles {
		if rest, ok := strings.CutPrefix(key, article); ok && rest != "" {
			return rest
		}
	}
	return key
}
