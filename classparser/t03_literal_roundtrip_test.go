package javaclassparser

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

var t03C01Units = []struct {
	name  string
	units []uint16
}{
	{"UnicodePair", []uint16{65, 55357, 56832, 90}},
	{"UnicodeLone", []uint16{65, 55296, 90}},
	{"UnicodeNul", []uint16{65, 0, 90}},
	{"UnicodeLiteral", []uint16{65, 0, 55357, 56832, 55296, 90}},
}

// LiteralEscapes runtime units from the reviewed fixture (quote, slash, slash-u, NL/CR/TAB/BS/FF, pair, lone low).
var t03C02LiteralEscapesUnits = []uint16{
	'q', 'u', 'o', 't', 'e', '=', '"', ' ',
	's', 'l', 'a', 's', 'h', '=', '\\', ' ',
	's', 'l', 'a', 's', 'h', 'u', '=', '\\', 'u', '0', '0', '4', '1',
	'\n', '\r', '\t', '\b', '\f',
	0xD800, 0xDC00, 0xDC00,
}

func TestTaskT03C01_UnicodePrinterRoundTrip(t *testing.T) {
	javac, java := requireJDK(t)
	for _, tc := range t03C01Units {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			lit := values.JavaUnitsToStringLiteral(tc.units)
			assertSafeLiteral(t, lit)
			got := javacStringRoundTrip(t, javac, java, "T03C01"+tc.name, lit)
			assertUnitsEqual(t, "T03-C01 "+tc.name, got, tc.units)
		})
	}

	t.Run("decompile_wiring", func(t *testing.T) {
		for _, tc := range t03C01Units {
			srcPath := filepath.Join("testdata", "contracts", "t03", tc.name+".java")
			raw, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("T03-C01 missing fixture %s: %v", srcPath, err)
			}
			for _, debug := range []string{"-g", "-g:none"} {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, tc.name+".java"), raw, 0o644); err != nil {
					t.Fatal(err)
				}
				compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", dir, tc.name+".java")
				compile.Dir = dir
				if out, err := compile.CombinedOutput(); err != nil {
					t.Fatalf("T03-C01 fixture javac %s %s: %v\n%s", tc.name, debug, err, out)
				}
				classPath := filepath.Join(dir, tc.name+".class")
				classBytes, err := os.ReadFile(classPath)
				if err != nil {
					t.Fatal(err)
				}
				for _, mode := range []DecompileMode{Precision, Compatibility} {
					res, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode})
					if err != nil {
						t.Fatalf("T03-C01 DecompileWithOptions(%s,%s,%s): %v", tc.name, debug, mode, err)
					}
					outDir := t.TempDir()
					if err := os.WriteFile(filepath.Join(outDir, tc.name+".java"), []byte(res.Source), 0o644); err != nil {
						t.Fatal(err)
					}
					re := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", outDir, tc.name+".java")
					re.Dir = outDir
					if out, err := re.CombinedOutput(); err != nil {
						t.Fatalf("T03-C01 decompiled %s mode=%s did not recompile:\n%s\n--- source ---\n%s", tc.name, mode, t03Head(string(out), 800), t03Head(res.Source, 2000))
					}
					run := exec.Command(java, "-Xverify:all", "-cp", outDir, tc.name)
					out, err := run.CombinedOutput()
					if err != nil {
						t.Fatalf("T03-C01 decompiled %s mode=%s run failed: %v\n%s", tc.name, mode, err, out)
					}
					got := parseIntLines(t, string(out))
					if !unitsEqual(got, tc.units) {
						t.Fatalf("T03-C01 decompiled %s %s mode=%s units=%v want %v", tc.name, debug, mode, got, tc.units)
					}
				}
			}
		}
	})
}

func TestTaskT03C02_LiteralEscapes(t *testing.T) {
	javac, java := requireJDK(t)
	lit := values.JavaUnitsToStringLiteral(t03C02LiteralEscapesUnits)
	assertSafeLiteral(t, lit)
	got := javacStringRoundTrip(t, javac, java, "T03C02Escapes", lit)
	assertUnitsEqual(t, "T03-C02 LiteralEscapes", got, t03C02LiteralEscapesUnits)

	near := []uint16{'\\', 'u', '0', '0', '0', 'a'}
	nearLit := values.JavaUnitsToStringLiteral(near)
	if strings.Contains(nearLit, "\n") || strings.Contains(nearLit, "\r") {
		t.Fatalf("T03-C02 near-miss became a line terminator: %q", nearLit)
	}
	if strings.Contains(nearLit, `\u000a`) || strings.Contains(nearLit, `\u000A`) {
		t.Fatalf("T03-C02 near-miss used \\u000A: %q", nearLit)
	}
	gotNear := javacStringRoundTrip(t, javac, java, "T03C02NearMiss", nearLit)
	assertUnitsEqual(t, "T03-C02 near-miss \\\\u000a data", gotNear, near)

	srcPath := filepath.Join("testdata", "contracts", "t03", "LiteralEscapes.java")
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("T03-C02 fixture copy missing: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "LiteralEscapes.java"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir, "LiteralEscapes.java")
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("T03-C02 original fixture javac: %v\n%s", err, out)
	}
	run := exec.Command(java, "-Xverify:all", "-cp", dir, "LiteralEscapes")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("T03-C02 original fixture run: %v\n%s", err, out)
	}
	orig := parseIntLines(t, string(out))
	assertUnitsEqual(t, "T03-C02 original fixture stdout", orig, t03C02LiteralEscapesUnits)
	classBytes, err := os.ReadFile(filepath.Join(dir, "LiteralEscapes.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		res, err := DecompileWithOptions(classBytes, DecompileOptions{Mode: mode})
		if err != nil {
			t.Fatalf("T03-C02 DecompileWithOptions(%s): %v", mode, err)
		}
		outDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(outDir, "LiteralEscapes.java"), []byte(res.Source), 0o644); err != nil {
			t.Fatal(err)
		}
		re := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", outDir, "LiteralEscapes.java")
		re.Dir = outDir
		if out, err := re.CombinedOutput(); err != nil {
			t.Fatalf("T03-C02 decompiled LiteralEscapes mode=%s did not recompile:\n%s\n%s", mode, out, t03Head(res.Source, 2000))
		}
		run = exec.Command(java, "-Xverify:all", "-cp", outDir, "LiteralEscapes")
		out, err = run.CombinedOutput()
		if err != nil {
			t.Fatalf("T03-C02 decompiled LiteralEscapes mode=%s run: %v\n%s", mode, err, out)
		}
		got = parseIntLines(t, string(out))
		assertUnitsEqual(t, "T03-C02 decompiled "+string(mode), got, t03C02LiteralEscapesUnits)
	}
}

func TestTaskT03C04_AllUnitsAndPairs(t *testing.T) {
	javac, java := requireJDK(t)
	const chunk = 256
	pairs := t03C04Pairs()
	var b strings.Builder
	b.Grow(512 * 1024)
	b.WriteString("public class T03C04 {\n")
	b.WriteString("  static void dump(String s) { for (int i = 0; i < s.length(); i++) System.out.println((int) s.charAt(i)); }\n")
	b.WriteString("  public static void main(String[] args) {\n")
	for off := 0; off < 65536; off += chunk {
		units := make([]uint16, chunk)
		for i := 0; i < chunk; i++ {
			units[i] = uint16(off + i)
		}
		lit := values.JavaUnitsToStringLiteral(units)
		assertSafeLiteral(t, lit)
		fmt.Fprintf(&b, "    dump(%s);\n", lit)
	}
	for _, p := range pairs {
		lit := values.JavaUnitsToStringLiteral([]uint16{p[0], p[1]})
		assertSafeLiteral(t, lit)
		fmt.Fprintf(&b, "    dump(%s);\n", lit)
	}
	b.WriteString("  }\n}\n")
	src := b.String()
	if strings.Count(src, "public static void main") != 1 {
		t.Fatal("T03-C04 leaked extra main (quote/comment injection)")
	}
	if strings.Contains(src, `"""`) {
		t.Fatal("T03-C04 emitted text block")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "T03C04.java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir, "T03C04.java")
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("T03-C04 javac failed: %v\n%s\nsource head:\n%s", err, out, t03Head(src, 1500))
	}
	run := exec.Command(java, "-Xverify:all", "-cp", dir, "T03C04")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("T03-C04 java failed: %v\n%s", err, t03Head(string(out), 1500))
	}
	got := parseIntLines(t, string(out))
	wantN := 65536 + 2*len(pairs)
	if len(got) != wantN {
		t.Fatalf("T03-C04 runtime length %d want %d (leaked quotes/extra statements?)", len(got), wantN)
	}
	for i := 0; i < 65536; i++ {
		if got[i] != i {
			t.Fatalf("T03-C04 unit %d runtime %d", i, got[i])
		}
	}
	base := 65536
	for i, p := range pairs {
		a, b := got[base+2*i], got[base+2*i+1]
		if a != int(p[0]) || b != int(p[1]) {
			t.Fatalf("T03-C04 pair %d got (%d,%d) want (%d,%d)", i, a, b, p[0], p[1])
		}
	}
}

func t03C04Pairs() [][2]uint16 {
	required := [][2]uint16{
		{'\\', 'u'},
		{'\\', 'n'},
		{'"', '\\'},
		{'\r', '\n'},
		{0xD800, 0xDE00},
		{0xDE00, 0xD800},
		{0xD800, 0xD800},
	}
	rng := rand.New(rand.NewSource(0xC0FFEE))
	pairs := append([][2]uint16(nil), required...)
	for len(pairs) < 64 {
		pairs = append(pairs, [2]uint16{uint16(rng.Intn(65536)), uint16(rng.Intn(65536))})
	}
	return pairs
}

func javacStringRoundTrip(t *testing.T, javac, java, class, lit string) []int {
	t.Helper()
	src := fmt.Sprintf("public class %s {\n  public static void main(String[] args) {\n    String s = %s;\n    for (int i = 0; i < s.length(); i++) System.out.println((int) s.charAt(i));\n  }\n}\n", class, lit)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, class+".java"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	compile := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir, class+".java")
	compile.Dir = dir
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("javac %s failed: %v\n%s\nsource:\n%s", class, err, out, src)
	}
	run := exec.Command(java, "-Xverify:all", "-cp", dir, class)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("java %s failed: %v\n%s", class, err, out)
	}
	return parseIntLines(t, string(out))
}

func requireJDK(t *testing.T) (javac, java string) {
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

func parseIntLines(t *testing.T, out string) []int {
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

func assertUnitsEqual(t *testing.T, label string, got []int, want []uint16) {
	t.Helper()
	if !unitsEqual(got, want) {
		t.Fatalf("%s units mismatch\n got %v\nwant %v", label, got, want)
	}
}

func unitsEqual(got []int, want []uint16) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != int(want[i]) {
			return false
		}
	}
	return true
}

func assertSafeLiteral(t *testing.T, lit string) {
	t.Helper()
	if !strings.HasPrefix(lit, `"`) || !strings.HasSuffix(lit, `"`) {
		t.Fatalf("not a string literal: %q", lit)
	}
	if strings.Contains(lit, "\n") || strings.Contains(lit, "\r") {
		t.Fatalf("literal contains raw line terminator: %q", lit)
	}
	if strings.Contains(lit, `"""`) {
		t.Fatalf("literal looks like a text block: %q", lit)
	}
}

func t03Head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
