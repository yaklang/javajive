package utils

import (
	"math/rand"
	"regexp"
	"testing"
)

// TestWordScanMatchesRegexpSemantics pins the literal scanner to Go regexp's
// \b semantics for every name shape the decompiler passes, including $, digits
// and the degenerate empty name (`\b\b`).
func TestWordScanMatchesRegexpSemantics(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	alphabet := []byte("abAB01_$ .()=;:<>")
	names := []string{"", "a", "ab", "var1", "var1_2", "$foo", "a$", "_x", "0", "A1_$"}
	for _, name := range names {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		for iter := 0; iter < 2000; iter++ {
			n := r.Intn(40)
			buf := make([]byte, n)
			for i := range buf {
				buf[i] = alphabet[r.Intn(len(alphabet))]
			}
			text := string(buf)
			want := re.FindAllStringIndex(text, -1)
			got := WordSpans(text, name, nil)
			if len(got) != len(want) {
				t.Fatalf("spans %q in %q: got %v want %v", name, text, got, want)
			}
			for i := range got {
				if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
					t.Fatalf("spans %q in %q: got %v want %v", name, text, got, want)
				}
			}
			if WordMatches(text, name) != re.MatchString(text) {
				t.Fatalf("match %q in %q: got %v want %v", name, text, WordMatches(text, name), re.MatchString(text))
			}
			if WordCount(text, name) != len(want) {
				t.Fatalf("count %q in %q: got %d want %d", name, text, WordCount(text, name), len(want))
			}
		}
	}
}
