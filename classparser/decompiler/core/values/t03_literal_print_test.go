package values

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestTaskT03_PrinterCapacityOverflowSafe(t *testing.T) {
	if printerCap(0) != 2 {
		t.Fatalf("printerCap(0)=%d", printerCap(0))
	}
	if printerCap(4) != 2+24 {
		t.Fatalf("printerCap(4)=%d", printerCap(4))
	}
	if printerCap((math.MaxInt-2)/6+1) != math.MaxInt {
		t.Fatal("printerCap must saturate at MaxInt")
	}
}

func TestTaskT03C03_CharLiterals(t *testing.T) {
	cases := []struct {
		name string
		unit uint16
	}{
		{"single_quote", '\''},
		{"double_quote", '"'},
		{"backslash", '\\'},
		{"NUL", 0},
		{"D800", 0xD800},
	}
	ctx := &class_context.ClassContext{}
	javac, java := requireJDKValues(t)
	dir := t.TempDir()

	var body strings.Builder
	body.WriteString("public class T03C03 {\n  public static void main(String[] args) {\n")
	for i, tc := range cases {
		lit := JavaUnitToCharLiteral(tc.unit)
		if !strings.HasPrefix(lit, "'") || !strings.HasSuffix(lit, "'") || len(lit) < 3 {
			t.Fatalf("T03-C03 %s: not a char literal: %q", tc.name, lit)
		}
		if strings.HasPrefix(lit, "\"") || strings.Contains(lit, `"""`) {
			t.Fatalf("T03-C03 %s: printed as String: %q", tc.name, lit)
		}
		inner := lit[1 : len(lit)-1]
		if strings.Contains(inner, "'") && tc.unit != '\'' {
			t.Fatalf("T03-C03 %s: extra quote in %q", tc.name, lit)
		}
		jl := &JavaLiteral{
			JavaType: types.NewJavaPrimer(types.JavaChar),
			Units:    []uint16{tc.unit},
			Data:     int(tc.unit),
		}
		if got := jl.String(ctx); got != lit {
			t.Fatalf("T03-C03 %s: JavaLiteral.String=%q want %q", tc.name, got, lit)
		}
		fmt.Fprintf(&body, "    char c%d = %s;\n    System.out.println((int)c%d);\n", i, lit, i)
	}
	// Data-only extraction (int / uint16 / int32)
	for _, data := range []any{int(0xD800), uint16(0xD800), int32(0xD800)} {
		jl := NewJavaLiteral(data, types.NewJavaPrimer(types.JavaChar))
		got := jl.String(ctx)
		if got != JavaUnitToCharLiteral(0xD800) {
			t.Fatalf("T03-C03 Data %T: %q", data, got)
		}
	}
	body.WriteString("  }\n}\n")
	src := body.String()
	if err := os.WriteFile(filepath.Join(dir, "T03C03.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir, "T03C03.java")
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("T03-C03 javac failed: %v\n%s\n%s", err, out, src)
	}
	run := exec.Command(java, "-Xverify:all", "-cp", dir, "T03C03")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("T03-C03 java failed: %v\n%s", err, out)
	}
	got := parseIntLinesValues(t, string(out))
	if len(got) != len(cases) {
		t.Fatalf("T03-C03 runtime lines=%d want %d (%q)", len(got), len(cases), out)
	}
	for i, tc := range cases {
		if got[i] != int(tc.unit) {
			t.Errorf("T03-C03 %s: runtime %d want %d", tc.name, got[i], tc.unit)
		}
	}
}

func TestTaskT03C05_Determinism(t *testing.T) {
	units := []uint16{0, 10, 13, '"', '\'', '\\', 'u', 0xD800, 0xDE00, 0xFFFF, 'A', 0x4E2D, 0x000C, 0x0008}
	hashOnce := func() string {
		h := sha256.New()
		_, _ = h.Write([]byte(JavaUnitsToStringLiteral(units)))
		for _, u := range units {
			_, _ = h.Write([]byte(JavaUnitToCharLiteral(u)))
		}
		lit := &JavaLiteral{JavaType: types.NewJavaPrimer(types.JavaString), Units: append([]uint16(nil), units...)}
		_, _ = h.Write([]byte(lit.String(&class_context.ClassContext{})))
		return hex.EncodeToString(h.Sum(nil))
	}
	first := hashOnce()
	for i := 1; i < 10; i++ {
		if got := hashOnce(); got != first {
			t.Fatalf("T03-C05 hash changed on print %d: %s vs %s", i+1, got, first)
		}
	}
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")
	if got := hashOnce(); got != first {
		t.Fatalf("T03-C05 LANG=C changed output hash %s vs %s", got, first)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	if got := hashOnce(); got != first {
		t.Fatalf("T03-C05 UTF-8 locale changed output hash %s vs %s", got, first)
	}
	t.Logf("T03-C05 unique hashes=1 value=%s", first)
}

func TestTaskT03_PrinterEscapesAndNearMiss(t *testing.T) {
	eq := func(got, want string) {
		t.Helper()
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	eq(JavaUnitsToStringLiteral(nil), `""`)
	eq(JavaUnitsToStringLiteral([]uint16{}), `""`)
	eq(JavaUnitsToStringLiteral([]uint16{'A'}), `"A"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\n'}), `"\n"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\r'}), `"\r"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\t'}), `"\t"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\b'}), `"\b"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\f'}), `"\f"`)
	eq(JavaUnitsToStringLiteral([]uint16{0}), `"\000"`)
	eq(JavaUnitsToStringLiteral([]uint16{'"'}), `"\""`)
	eq(JavaUnitsToStringLiteral([]uint16{'\\'}), `"\\"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\\', 'u'}), `"\\u"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\\', 'n'}), `"\\n"`)
	eq(JavaUnitsToStringLiteral([]uint16{'\\', '\\'}), `"\\\\"`)
	eq(JavaUnitsToStringLiteral([]uint16{0xD800}), `"\uD800"`)
	eq(JavaUnitToCharLiteral('\''), `'\''`)
	eq(JavaUnitToCharLiteral('"'), `'"'`)
	eq(JavaUnitToCharLiteral('\\'), `'\\'`)
	eq(JavaUnitToCharLiteral(0), `'\000'`)
	eq(JavaUnitToCharLiteral(0xD800), `'\uD800'`)

	near := []uint16{'\\', 'u', '0', '0', '0', 'a'}
	got := JavaUnitsToStringLiteral(near)
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") {
		t.Fatalf("near-miss \\u000a became a line terminator: %q", got)
	}
	if strings.Contains(got, `"""`) {
		t.Fatalf("emitted text block: %q", got)
	}
	// Must not represent the data as a Unicode-escape newline.
	if strings.Contains(strings.ToUpper(got), `\U000A`) || strings.Contains(got, `\u000a`) || strings.Contains(got, `\u000A`) {
		t.Fatalf("near-miss emitted \\u000A form: %q", got)
	}

	ctx := &class_context.ClassContext{}
	strLit := &JavaLiteral{JavaType: types.NewJavaPrimer(types.JavaString), Data: "ignore", Units: []uint16{'A', 0, 0xD800}}
	if g := strLit.String(ctx); g != JavaUnitsToStringLiteral([]uint16{'A', 0, 0xD800}) {
		t.Fatalf("Units path: %q", g)
	}
	compat := JavaStringToLiteral("A")
	if compat != `"A"` {
		t.Fatalf("compat string path: %q", compat)
	}
	if JavaStringToLiteral([]uint16{0xD800}) != `"\uD800"` {
		t.Fatalf("compat units path lost surrogate: %s", JavaStringToLiteral([]uint16{0xD800}))
	}
	// Compatibility string path loses unpaired surrogates (utf16.Encode([]rune(s))).
	lossy := string(rune(0xD800))
	if JavaStringToLiteral(lossy) == `"\uD800"` {
		t.Log("unexpected: Go string path preserved surrogate")
	} else if JavaStringToLiteral(lossy) != JavaUnitsToStringLiteral(utf16.Encode([]rune(lossy))) {
		t.Fatalf("compat path not using utf16.Encode")
	}

	boolLit := NewJavaLiteral(0, types.NewJavaPrimer(types.JavaBoolean))
	if boolLit.String(ctx) != "false" {
		t.Fatalf("boolean rendering changed: %s", boolLit.String(ctx))
	}
	longLit := NewJavaLiteral(int64(7), types.NewJavaPrimer(types.JavaLong))
	if longLit.String(ctx) != "7L" {
		t.Fatalf("long rendering changed: %s", longLit.String(ctx))
	}
}

func TestJavaUnits_AllUnitsLexicalSafety(t *testing.T) {
	for u := 0; u < 65536; u++ {
		unit := uint16(u)
		s := JavaUnitsToStringLiteral([]uint16{unit})
		c := JavaUnitToCharLiteral(unit)
		if !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
			t.Fatalf("unit %d string not quoted: %q", u, s)
		}
		if !strings.HasPrefix(c, `'`) || !strings.HasSuffix(c, `'`) {
			t.Fatalf("unit %d char not quoted: %q", u, c)
		}
		if strings.Contains(s, "\n") || strings.Contains(s, "\r") || strings.Contains(c, "\n") || strings.Contains(c, "\r") {
			t.Fatalf("unit %d leaked line terminator s=%q c=%q", u, s, c)
		}
		if strings.Contains(s, `"""`) {
			t.Fatalf("unit %d emitted text block: %q", u, s)
		}
		interior := s[1 : len(s)-1]
		if hasUnescapedQuote(interior, '"') {
			t.Fatalf("unit %d leaked quote in string %q", u, s)
		}
		if hasUnescapedQuote(c[1:len(c)-1], '\'') {
			t.Fatalf("unit %d leaked quote in char %q", u, c)
		}
		switch unit {
		case 0x000A:
			if strings.Contains(strings.ToUpper(s), `\U000A`) || strings.Contains(s, `\u000A`) || strings.Contains(s, `\u000a`) {
				t.Fatalf("unit LF used unicode escape: %q", s)
			}
		case 0x000D:
			if strings.Contains(strings.ToUpper(s), `\U000D`) || strings.Contains(s, `\u000D`) || strings.Contains(s, `\u000d`) {
				t.Fatalf("unit CR used unicode escape: %q", s)
			}
		case 0x0022:
			if strings.Contains(strings.ToUpper(s), `\U0022`) || strings.Contains(s, `\u0022`) {
				t.Fatalf("unit quote used unicode escape: %q", s)
			}
		case 0x0027:
			if strings.Contains(strings.ToUpper(c), `\U0027`) || strings.Contains(c, `\u0027`) {
				t.Fatalf("unit apostrophe used unicode escape: %q", c)
			}
		case 0x005C:
			if strings.Contains(strings.ToUpper(s), `\U005C`) || strings.Contains(s, `\u005C`) || strings.Contains(s, `\u005c`) {
				t.Fatalf("unit backslash used unicode escape: %q", s)
			}
		}
	}
}

func hasUnescapedQuote(s string, quote byte) bool {
	esc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if esc {
			esc = false
			continue
		}
		if b == '\\' {
			esc = true
			continue
		}
		if b == quote {
			return true
		}
	}
	return false
}

func requireJDKValues(t *testing.T) (javac, java string) {
	t.Helper()
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Fatalf("T03 infrastructure failure: javac missing: %v", err)
	}
	java, err = exec.LookPath("java")
	if err != nil {
		t.Fatalf("T03 infrastructure failure: java missing: %v", err)
	}
	ver, _ := exec.Command(javac, "-version").CombinedOutput()
	t.Logf("javac %s", strings.TrimSpace(string(ver)))
	return javac, java
}

func parseIntLinesValues(t *testing.T, out string) []int {
	t.Helper()
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil
	}
	var got []int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("parse int line %q: %v", line, err)
		}
		got = append(got, n)
	}
	return got
}
