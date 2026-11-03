package values

import "testing"

// A single-operand ++/-- prints its target without a redundant parenthesis
// layer, because the target is an lvalue and because at least one downstream
// Java frontend rejects `++(x)`. Exactly one outer layer is dropped, and only
// for shapes that are unambiguously a single lvalue: an identifier, a dotted
// member chain, or a chain with balanced subscripts. Every other shape keeps
// its text so no precedence can change.
func TestIncrementOperandDropsOnlyRedundantLvalueParens(t *testing.T) {
	cases := []struct{ text, want string }{
		{"x", "x"},
		{"(x)", "x"},
		{"(_local$1)", "_local$1"},
		{"(this.field)", "this.field"},
		{"(Outer.Inner.VALUE)", "Outer.Inner.VALUE"},
		{"(a.b[i].c)", "a.b[i].c"},
		{"(arr[idx])", "arr[idx]"},
		{"(a[i][j])", "a[i][j]"},
		{"((x))", "((x))"},
		{"(a[0])[1]", "(a[0])[1]"},
		// Not a single lvalue: keep the author's text.
		{"(a + b)", "(a + b)"},
		{"(a - b)", "(a - b)"},
		{"(a[1] + b)", "(a[1] + b)"},
		{"(a[)", "(a[)"},
		{"(x.)", "(x.)"},
		{"(1)", "(1)"},
		{"(int) x", "(int) x"},
		{"()", "()"},
		{"", ""},
		{"x", "x"},
	}
	for _, c := range cases {
		if got := incrementOperandText(c.text); got != c.want {
			t.Fatalf("incrementOperandText(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
