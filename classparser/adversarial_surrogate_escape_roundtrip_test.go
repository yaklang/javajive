package javaclassparser

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

// The original fixture uses a deliberately separate, numeric source encoder.
// Its unchanged driver checks numeric words, and is the only original class
// copied into the candidate directory. No original implementation is available
// to either candidate javac or its JVM.
func TestAdversarialSurrogateEscapeLiteralProductionRoundTrip(t *testing.T) {
	javac, java := requireJDK(t)
	var source, driver, expected strings.Builder
	source.WriteString("public class SurrogateWordProbe {\n")
	driver.WriteString("public class SurrogateWordDriver {static void word(String s,int...u){if(s.length()!=u.length)throw new AssertionError(\"length\");for(int i=0;i<u.length;i++){if(s.charAt(i)!=u[i])throw new AssertionError(\"word:\"+i);System.out.println((int)s.charAt(i));}}public static void main(String[]args){\n")
	rows := 0
	for _, high := range []uint16{0xD800, 0xD801, 0xDBFE, 0xDBFF} {
		for _, prefix := range [][]uint16{{high}, {high, 0xDC00}, {high, 0xDBFF}, {0xDC00, high}} {
			for _, tail := range []string{"\\", "\\u000A", "\\u0G00", "\\\\uFFFF", "\\uu000D", "\\u000", "\\z", "\\\"\n"} {
				units := append([]uint16(nil), prefix...)
				for _, b := range []byte(tail) {
					units = append(units, uint16(b))
				}
				fmt.Fprintf(&source, "public static String word%d(){return \"", rows)
				fmt.Fprintf(&driver, "word(SurrogateWordProbe.word%d()", rows)
				for _, u := range units {
					if u <= 255 {
						fmt.Fprintf(&source, "\\%03o", u)
					} else {
						fmt.Fprintf(&source, "\\u%04X", u)
					}
					fmt.Fprintf(&driver, ",%d", u)
					fmt.Fprintf(&expected, "%d\n", u)
				}
				source.WriteString("\";}\n")
				driver.WriteString(");\n")
				rows++
			}
		}
	}
	if rows != 128 {
		t.Fatalf("incomplete boundary product: %d", rows)
	}
	source.WriteString("}\n")
	driver.WriteString("}}\n")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	compile := func(t *testing.T, dir, debug string, files ...string) {
		t.Helper()
		args := []string{"-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-cp", dir, "-d", dir}
		args = append(args, files...)
		if out, err := exec.CommandContext(ctx, javac, args...).CombinedOutput(); err != nil {
			t.Fatalf("literal production javac: %v\n%s", err, out)
		}
	}
	run := func(t *testing.T, dir string) {
		t.Helper()
		out, err := exec.CommandContext(ctx, java, "-Xverify:all", "-cp", dir, "SurrogateWordDriver").CombinedOutput()
		if err != nil || string(out) != expected.String() {
			t.Fatalf("numeric literal oracle: %v\n%s", err, out)
		}
	}
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			original := t.TempDir()
			probe := filepath.Join(original, "SurrogateWordProbe.java")
			driverPath := filepath.Join(original, "SurrogateWordDriver.java")
			for path, text := range map[string]string{probe: source.String(), driverPath: driver.String()} {
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			compile(t, original, debug, probe, driverPath)
			run(t, original) // Independent original JVM must pass before reconstruction.
			raw, err := os.ReadFile(filepath.Join(original, "SurrogateWordProbe.class"))
			if err != nil {
				t.Fatal(err)
			}
			driverBytes, err := os.ReadFile(filepath.Join(original, "SurrogateWordDriver.class"))
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					res, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Context: ctx})
					if err != nil || len(res.StubMethods) != 0 {
						t.Fatalf("literal production decompile: %v stubs=%v", err, res.StubMethods)
					}
					candidate := t.TempDir()
					path := filepath.Join(candidate, "SurrogateWordProbe.java")
					if err := os.WriteFile(path, []byte(res.Source), 0600); err != nil {
						t.Fatal(err)
					}
					compile(t, candidate, "-g:none", path)
					if err := os.WriteFile(filepath.Join(candidate, "SurrogateWordDriver.class"), driverBytes, 0600); err != nil {
						t.Fatal(err)
					}
					run(t, candidate)
				})
			}
		})
	}
}
