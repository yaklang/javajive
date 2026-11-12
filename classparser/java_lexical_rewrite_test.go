package javaclassparser

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestJavaCodeRewriteLiteralBoundaries(t *testing.T) {
	cases := []string{
		`"Integer::intValue"`,
		`"escaped \\\" Integer::intValue this.getMatchers()"`,
		"\"\"\"\nInteger::intValue\nthis.getMatchers()\n\"\"\"",
		"// Integer::intValue this.getMatchers()\r\n",
		"/* Integer::intValue\nthis.getMatchers() */",
		`'\''`,
		`"__JDEC_PROTECTED_0__ Integer::intValue"`,
	}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			src := "class X { Object f = Integer::intValue; Object match = this.getMatchers(); Object v = " + data + "; }"
			got := applyMockitoShapes(src)
			if !strings.Contains(got, data) {
				t.Fatalf("protected data changed:\n%s", got)
			}
			if !strings.Contains(got, "Object match = (List)(this.getMatchers());") {
				t.Fatalf("code was not rewritten:\n%s", got)
			}
			if !strings.Contains(got, "Object f = Integer::intValue;") {
				t.Fatalf("text-only rewrite changed a method-reference factory:\n%s", got)
			}
			if twice := applyMockitoShapes(got); twice != got {
				t.Fatalf("rule not idempotent:\n%s", twice)
			}
		})
	}
}
func TestMockitoPackageGateUsesCode(t *testing.T) {
	for _, source := range []string{
		`package org.mockitofake; class X {Object x = Integer::intValue;}`,
		`package ordinary; class X {String x="package org.mockito";Object y=Integer::intValue;}`,
		"package ordinary; class X {String x=\"\"\"\npackage org.mockito;\n\"\"\";Object y=Integer::intValue;}",
	} {
		if got := fixHardjarShapes(source); got != source {
			t.Fatalf("false package gate:\n%s", got)
		}
	}
}

// legacyRewriteJavaSourceSegments is the previous implementation, kept as an
// independent oracle for the marker-restoration rewrite. It must not be used by
// production code.
func legacyRewriteJavaSourceSegments(source string, rewrite func(string) string, protectComments bool) string {
	prefix := "__JDEC_PROTECTED_"
	for strings.Contains(source, prefix) {
		prefix = "_" + prefix
	}
	var out strings.Builder
	type protected struct{ marker, text string }
	var spans []protected
	for i := 0; i < len(source); {
		start := i
		kind := ""
		switch {
		case strings.HasPrefix(source[i:], "//"):
			kind = "line"
			i += 2
			for i < len(source) && source[i] != '\n' && source[i] != '\r' {
				i++
			}
		case strings.HasPrefix(source[i:], "/*"):
			kind = "block"
			i += 2
			for i < len(source) && !strings.HasPrefix(source[i:], "*/") {
				i++
			}
			if i < len(source) {
				i += 2
			}
		case strings.HasPrefix(source[i:], `"""`):
			kind = "text"
			i += 3
			for i < len(source) {
				if source[i] == '\\' {
					i++
					if i < len(source) {
						i++
					}
					continue
				}
				if strings.HasPrefix(source[i:], `"""`) {
					i += 3
					break
				}
				i++
			}
		case source[i] == '"' || source[i] == '\'':
			quote := source[i]
			kind = "string"
			if quote == '\'' {
				kind = "char"
			}
			i++
			for i < len(source) {
				b := source[i]
				i++
				if b == '\\' {
					if i < len(source) {
						i++
					}
				} else if b == quote {
					break
				}
			}
		default:
			out.WriteByte(source[i])
			i++
			continue
		}
		if !protectComments && (kind == "line" || kind == "block") {
			out.WriteString(source[start:i])
			continue
		}
		marker := fmt.Sprintf("%s%d__", prefix, len(spans))
		switch kind {
		case "line":
			marker = "//" + marker
		case "block":
			marker = "/*" + marker + "*/"
		case "text", "string":
			marker = `"` + marker + `"`
		case "char":
			marker = "'" + marker + "'"
		}
		spans = append(spans, protected{marker, source[start:i]})
		out.WriteString(marker)
	}
	result := rewrite(out.String())
	for _, span := range spans {
		result = strings.ReplaceAll(result, span.marker, span.text)
	}
	if strings.Contains(result, prefix) {
		return source
	}
	return result
}

func TestRewriteJavaSourceSegmentsMatchesLegacy(t *testing.T) {
	sources := []string{
		`class X { String a = "one"; // two
	char c = '3'; String b = """
	text block
	"""; Object d = Integer::intValue; }`,
		`class X { String s = "__JDEC_PROTECTED_0__"; }`,
		`class X { String a = "x"; String b = "y"; }`,
		`class X { char a = '\''; char b = '\\'; }`,
		`class X { /* a */ String s = "a\"b"; }`,
		`class X { String s = "\u1234"; }`,
		`class X { int i = 1 / 2; }`,
		``,
	}
	rewrites := map[string]func(string) string{
		"identity":  func(s string) string { return s },
		"duplicate": func(s string) string { return s + "\n" + s },
		"drop-half": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return s[:len(s)/2]
		},
		"reverse": func(s string) string {
			b := []byte(s)
			for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
				b[i], b[j] = b[j], b[i]
			}
			return string(b)
		},
		"inject-unknown": func(s string) string { return s + `"__JDEC_PROTECTED_999__"` },
		"rename-prefix": func(s string) string {
			return strings.ReplaceAll(s, "__JDEC_PROTECTED_", "__JDEC_PROTECTED_X")
		},
		"rewrap-first": func(s string) string {
			const prefix = "__JDEC_PROTECTED_"
			at := strings.Index(s, prefix)
			if at < 0 {
				return s
			}
			end := at
			for end < len(s) && (s[end] >= '0' && s[end] <= '9' || s[end] == '_') {
				end++
			}
			inner := s[at:end]
			return strings.Replace(s, inner, "'"+inner+"'", 1)
		},
		"swap-quoted": func(s string) string {
			parts := strings.Split(s, `"`)
			if len(parts) < 5 {
				return s
			}
			parts[1], parts[3] = parts[3], parts[1]
			return strings.Join(parts, `"`)
		},
	}
	for si, source := range sources {
		for _, protectComments := range []bool{true, false} {
			for name, rewrite := range rewrites {
				t.Run(fmt.Sprintf("src%d/%s/protectComments=%v", si, name, protectComments), func(t *testing.T) {
					got := rewriteJavaSourceSegments(source, rewrite, protectComments)
					want := legacyRewriteJavaSourceSegments(source, rewrite, protectComments)
					if got != want {
						t.Fatalf("rewrite mismatch\ngot:  %q\nwant: %q", got, want)
					}
				})
			}
		}
	}
}

func legacyRestoreProtectedSpans(result, prefix string, spans []protectedSpan) (string, bool) {
	for _, span := range spans {
		result = strings.ReplaceAll(result, span.marker, span.text)
	}
	if strings.Contains(result, prefix) {
		return result, false
	}
	return result, true
}

func TestRestoreProtectedSpansMatchesSequentialReplace(t *testing.T) {
	const prefix = "__JDEC_PROTECTED_"
	spans := []protectedSpan{
		{marker: `"` + prefix + `0__"`, text: `"alpha"`, inner: 1},
		{marker: "//" + prefix + `1__`, text: "// note", inner: 2},
		{marker: "/*" + prefix + `2__*/`, text: "/* block */", inner: 2},
		{marker: `'` + prefix + `3__'`, text: "'c'", inner: 1},
	}
	results := []string{
		"",
		"abc",
		`"` + prefix + `0__"`,
		`x "` + prefix + `0__" y //` + prefix + `1__`,
		`"` + prefix + `0__" "` + prefix + `0__"`,
		`'` + prefix + `0__'`,
		prefix + `0__`,
		`"` + prefix + `9__"`,
		`"` + prefix + `0___"`,
		`"` + prefix + `00__"`,
		`"` + prefix + `0__x`,
		`"` + prefix + `0__"` + `"` + prefix + `0__"`,
	}
	for _, result := range results {
		got, ok := restoreProtectedSpans(result, prefix, spans)
		want, wantOK := legacyRestoreProtectedSpans(result, prefix, spans)
		if ok != wantOK || (ok && got != want) {
			t.Fatalf("restore mismatch for %q: got (%q,%v) want (%q,%v)", result, got, ok, want, wantOK)
		}
	}
}

func TestRestoreProtectedSpansRandomized(t *testing.T) {
	const prefix = "__JDEC_PROTECTED_"
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 1000; trial++ {
		n := 1 + rng.Intn(5)
		spans := make([]protectedSpan, n)
		for i := range spans {
			inner := prefix + fmt.Sprintf("%d__", i)
			switch rng.Intn(4) {
			case 0:
				spans[i] = protectedSpan{marker: `"` + inner + `"`, text: fmt.Sprintf(`"t%d"`, i), inner: 1}
			case 1:
				spans[i] = protectedSpan{marker: "//" + inner, text: fmt.Sprintf("//t%d", i), inner: 2}
			case 2:
				spans[i] = protectedSpan{marker: "/*" + inner + "*/", text: fmt.Sprintf("/*t%d*/", i), inner: 2}
			default:
				spans[i] = protectedSpan{marker: "'" + inner + "'", text: "'c'", inner: 1}
			}
		}
		var b strings.Builder
		for k, parts := 0, 1+rng.Intn(8); k < parts; k++ {
			switch rng.Intn(7) {
			case 0:
				fmt.Fprintf(&b, "x%d", k)
			case 1, 2:
				b.WriteString(spans[rng.Intn(n)].marker)
			case 3:
				b.WriteString(prefix + fmt.Sprintf("%d__", rng.Intn(n)))
			case 4:
				b.WriteString(`"` + prefix + fmt.Sprintf("%d__", rng.Intn(n)) + `'`)
			case 5:
				b.WriteString(`"` + prefix + fmt.Sprintf("%d__", n+rng.Intn(3)) + `"`)
			default:
				b.WriteString(spans[rng.Intn(n)].marker)
				b.WriteString(spans[rng.Intn(n)].marker)
			}
		}
		result := b.String()
		got, ok := restoreProtectedSpans(result, prefix, spans)
		want, wantOK := legacyRestoreProtectedSpans(result, prefix, spans)
		if ok != wantOK || (ok && got != want) {
			t.Fatalf("trial %d: restore mismatch for %q\nspans: %+v\ngot:  (%q,%v)\nwant: (%q,%v)",
				trial, result, spans, got, ok, want, wantOK)
		}
	}
}
