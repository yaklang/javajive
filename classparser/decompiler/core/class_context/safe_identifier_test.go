package class_context

import (
	"slices"
	"testing"
)

func TestSafeIdentifierKeepsBareDollar(t *testing.T) {
	if got := SafeIdentifier("$"); got != "$" {
		t.Fatalf("bare dollar: %q", got)
	}
	if got := SafeIdentifier("ENUM$VALUES"); got != "ENUM$VALUES" {
		t.Fatalf("embedded dollar changed: %q", got)
	}
	if got := SafeIdentifier("$$"); got != "$$" {
		t.Fatalf("two dollars changed: %q", got)
	}
}

func TestSafePackageSegmentAndName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"com.example", "com.example"},
		{"µ.个", "µ.个"},
		{"♫", "_u266b"},
		{"♫.a-b.1x.class", "_u266b.a_u002db._1x.class_"},
		{"?", "_u003f"},
	}
	for _, tc := range cases {
		if got := SafePackageName(tc.in); got != tc.want {
			t.Fatalf("SafePackageName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Binary lookup keys must stay raw so same-package matching keeps working while
	// emitted source prefixes (imports & FQ type refs) use the safe package spelling.
	f := &ClassContext{PackageName: "\u266b"}
	if got := f.ShortTypeName("\u266b.\u4e2a"); got != "\u4e2a" {
		t.Fatalf("same-package raw match failed: %q", got)
	}

	caller := &ClassContext{PackageName: "com.caller"}
	if got := caller.ShortTypeName("\u266b.\u4e2a"); got != "\u4e2a" {
		t.Fatalf("cross-package imported ref = %q, want \u4e2a", got)
	}
	imports := caller.GetAllImported()
	if !slices.Equal(imports, []string{"_u266b.\u4e2a"}) {
		t.Fatalf("GetAllImported() = %#v, want [_u266b.\u4e2a]", imports)
	}

	shadowed := &ClassContext{
		PackageName:      "com.caller",
		LexicalTypeNames: map[string]bool{"\u4e2a": true},
	}
	if got := shadowed.ShortTypeName("\u266b.\u4e2a"); got != "_u266b.\u4e2a" {
		t.Fatalf("cross-package safe FQ ref = %q, want _u266b.\u4e2a", got)
	}
}
