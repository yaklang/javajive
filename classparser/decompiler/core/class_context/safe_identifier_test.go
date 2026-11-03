package class_context

import "testing"

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
