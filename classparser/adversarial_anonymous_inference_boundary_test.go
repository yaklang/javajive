package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An independent compiler witness distinguishes the anonymous subtype from its
// parent: inference retains members that only the anonymous class declares.
// Casting that value to Parent must lose the member even though erasure agrees.
func TestAdversarialAnonymousInferenceRetainsAnonymousSubtype(t *testing.T) {
	javac, java := t04Tools(t)
	for _, compiler := range []struct{ name, path string }{{"release8", javac}, {"javac8", os.Getenv("JAVA8_JAVAC")}} {
		t.Run(compiler.name, func(t *testing.T) {
			if compiler.path == "" {
				t.Skip("JAVA8_JAVAC required for actual compiler")
			}
			for _, widen := range []bool{false, true} {
				name := "original"
				if widen {
					name = "parent equality mutant"
				}
				t.Run(name, func(t *testing.T) {
					source := `public class AnonymousInferenceBoundary {static class Parent{}static class Box<T>{final T value;Box(T v){value=v;}}static <T extends Parent> Box<T> wrap(T value){return new Box<T>(value);}public static void main(String[] args){int value=wrap(new Parent(){public int marker(){return 7;}}).value.marker();if(value!=7)throw new AssertionError();System.out.println("anonymous:subtype:7");}}`
					if widen {
						source = strings.Replace(source, "wrap(new Parent()", "wrap((Parent)new Parent()", 1)
					}
					out := t.TempDir()
					file := filepath.Join(out, "AnonymousInferenceBoundary.java")
					if err := os.WriteFile(file, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"-proc:none", "-XDrawDiagnostics", "-g:none", "-d", out}
					if compiler.name == "release8" {
						args = append(args, "--release", "8")
					}
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					output, err := exec.CommandContext(ctx, compiler.path, append(args, file)...).CombinedOutput()
					interrupted := ctx.Err() != nil
					cancel()
					if widen {
						if interrupted || err == nil || !strings.Contains(string(output), "marker") || !strings.Contains(string(output), "compiler.err.cant.resolve.location") {
							t.Fatalf("compiler failed to reject parent equality: %v %s", err, output)
						}
						return
					}
					if interrupted || err != nil {
						t.Fatalf("original anonymous subtype: %v %s", err, output)
					}
					for _, assertions := range []string{"-ea", "-da"} {
						ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
						output, err := exec.CommandContext(ctx, java, assertions, "-Xverify:all", "-cp", out, "AnonymousInferenceBoundary").CombinedOutput()
						interrupted := ctx.Err() != nil
						cancel()
						if interrupted || err != nil || string(output) != "anonymous:subtype:7\n" {
							t.Fatalf("original subtype driver: %v %s", err, output)
						}
					}
				})
			}
		})
	}
}
