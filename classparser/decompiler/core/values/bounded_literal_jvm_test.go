package values

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The expected values are numeric UTF-16 words, independently of either Java
// source escaping or Go's Unicode decoder. javac performs JLS Unicode translation
// and tokenization, then the JVM checks the resulting literal's actual words.
// Source is split by class and method to respect classfile pool/Code limits.
func TestJavaUnitsExhaustiveJVMCodeUnitOracle(t *testing.T) {
	javac, java := requireJDKValues(t)
	dir := t.TempDir()
	var driver strings.Builder
	driver.WriteString(`public class LiteralWordOracle {
static int rows;
static void unit(char c,String s,int u){if(c!=u||s.length()!=1||s.charAt(0)!=u)throw new AssertionError("unit:"+u);rows++;}
static void text(String s,int...u){if(s.length()!=u.length)throw new AssertionError("length:"+rows);for(int i=0;i<u.length;i++)if(s.charAt(i)!=u[i])throw new AssertionError("word:"+rows+":"+i);rows++;}
public static void main(String[]args){`)
	paths := []string{}
	checkedLiteral := func(units []uint16) string {
		t.Helper()
		literal := JavaUnitsToStringLiteral(units)
		if n := JavaUnitsStringLiteralLen(units); n != len(literal) {
			t.Fatalf("output budget: units=%v count=%d bytes=%d", units, n, len(literal))
		}
		if cap := JavaUnitsStringLiteralCap(len(units)); cap < len(literal) {
			t.Fatalf("allocation budget: units=%v cap=%d bytes=%d", units, cap, len(literal))
		}
		return literal
	}
	for chunk := 0; chunk < 32; chunk++ {
		name := fmt.Sprintf("LiteralWordChunk%d", chunk)
		var source strings.Builder
		fmt.Fprintf(&source, "final class %s {static void run(){", name)
		for block := 0; block < 16; block++ {
			fmt.Fprintf(&source, "b%d();", block)
		}
		source.WriteString("}\n")
		for block := 0; block < 16; block++ {
			fmt.Fprintf(&source, "static void b%d(){\n", block)
			for offset := 0; offset < 128; offset++ {
				u := uint16(chunk*2048 + block*128 + offset)
				char := JavaUnitToCharLiteral(u)
				if JavaUnitCharLiteralLen(u) != len(char) {
					t.Fatalf("char output budget: unit=%d", u)
				}
				fmt.Fprintf(&source, "LiteralWordOracle.unit(%s,%s,%d);\n", char, checkedLiteral([]uint16{u}), u)
			}
			source.WriteString("}\n")
		}
		source.WriteString("}\n")
		path := filepath.Join(dir, name+".java")
		if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		fmt.Fprintf(&driver, "%s.run();", name)
	}
	// Escape eligibility varies with adjacent backslashes, repeated 'u', a
	// complete/malformed hex tail, and quote/control/surrogate neighbors.
	cases := [][]uint16{}
	for slashes := 1; slashes <= 5; slashes++ {
		for us := 1; us <= 4; us++ {
			for _, tail := range []string{"000A", "000D", "0022", "0027", "005C", "D800", "FFFF", "0G00", "000", ""} {
				for _, neighbor := range []uint16{0, 10, 13, '\'', '"', '\\', 0xD800, 0xDC00, 0x2028} {
					units := []uint16{neighbor}
					for i := 0; i < slashes; i++ {
						units = append(units, '\\')
					}
					for i := 0; i < us; i++ {
						units = append(units, 'u')
					}
					for _, b := range []byte(tail) {
						units = append(units, uint16(b))
					}
					units = append(units, neighbor)
					cases = append(cases, units)
				}
			}
		}
	}
	if len(cases) != 1800 {
		t.Fatalf("incomplete escape grammar: %d", len(cases))
	}
	// Exercise every high surrogate, both unpaired and in neighboring surrogate
	// sequences. These are numeric word sequences; a surrogate pair must remain
	// two charAt words, with no host-language rune conversion in the oracle.
	tails := []string{"\\", "\\u000A", "\\u0G00", "\\\\uFFFF", "\\uu000D", "\\u000", "\\z", "\\\"\n"}
	for high := uint16(0xD800); high <= 0xDBFF; high++ {
		for _, prefix := range [][]uint16{{high}, {high, 0xDC00}, {high, 0xDBFF}} {
			for _, tail := range tails {
				units := append([]uint16(nil), prefix...)
				for _, b := range []byte(tail) {
					units = append(units, uint16(b))
				}
				cases = append(cases, units)
			}
		}
	}
	if len(cases) != 26376 {
		t.Fatalf("incomplete surrogate/escape product: %d", len(cases))
	}
	// Bound each constant pool independently, rather than relying on a single
	// large generated class continuing to fit as the grammar is expanded.
	for start := 0; start < len(cases); start += 1000 {
		end := min(start+1000, len(cases))
		name := fmt.Sprintf("LiteralWordNear%d", start/1000)
		var near strings.Builder
		fmt.Fprintf(&near, "final class %s {static void run(){", name)
		for block := start; block < end; block += 50 {
			fmt.Fprintf(&near, "b%d();", block)
		}
		near.WriteString("}\n")
		for block := start; block < end; block += 50 {
			fmt.Fprintf(&near, "static void b%d(){\n", block)
			for row := block; row < min(block+50, end); row++ {
				units := cases[row]
				fmt.Fprintf(&near, "LiteralWordOracle.text(%s", checkedLiteral(units))
				for _, u := range units {
					fmt.Fprintf(&near, ",%d", u)
				}
				near.WriteString(");\n")
			}
			near.WriteString("}\n")
		}
		near.WriteString("}\n")
		nearPath := filepath.Join(dir, name+".java")
		if err := os.WriteFile(nearPath, []byte(near.String()), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, nearPath)
		fmt.Fprintf(&driver, "%s.run();", name)
	}
	wantRows := 65536 + len(cases)
	fmt.Fprintf(&driver, "if(rows!=%d)throw new AssertionError(\"coverage:\"+rows);System.out.println(rows);}}", wantRows)
	driverPath := filepath.Join(dir, "LiteralWordOracle.java")
	if err := os.WriteFile(driverPath, []byte(driver.String()), 0600); err != nil {
		t.Fatal(err)
	}
	paths = append(paths, driverPath)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir}, paths...)
	if out, err := exec.CommandContext(ctx, javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("all-unit literal compilation failed: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(ctx, java, "-Xverify:all", "-cp", dir, "LiteralWordOracle").CombinedOutput()
	if err != nil || string(out) != fmt.Sprintf("%d\n", wantRows) {
		t.Fatalf("literal word oracle: %v output=%q", err, out)
	}
	t.Logf("65536 UTF-16 unit literals and %d escape-eligibility boundary strings checked by javac and JVM", len(cases))
}
