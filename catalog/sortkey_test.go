package catalog

import "testing"

func TestSortKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"The Cure", "cure"},
		{"A Copland Celebration", "copland celebration"},
		{"An Ending", "ending"},
		{"Dvořák: Symphony No. 9", "dvorak: symphony no. 9"},
		{`"Academy of Ancient Music" - Baroque`, `academy of ancient music" - baroque`},
		{"  Beethoven  ", "beethoven"},
		{"The", "the"},           // an article alone is kept, not emptied
		{"Theodora", "theodora"}, // only a whole leading word is an article
		{"", ""},
		{"Øresund", "oresund"},
		{"Łódź Philharmonic", "lodz philharmonic"},
		{"Straße", "strasse"},
		{"!!!", "!!!"}, // all punctuation falls back rather than emptying
	}
	for _, tt := range tests {
		if got := SortKey(tt.in); got != tt.want {
			t.Errorf("SortKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
