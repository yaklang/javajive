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

// The arithmetic model and immutable binary observer must independently reject
// wrong code, including an arrow whose ordinary outputs are all correct but
// whose compiler-generated implementation has the wrong physical input type.
func TestAdversarialStaticVarargsReferenceOracleRejectsSemanticAndABIMutants(t *testing.T) {
	compiler, java := t04Tools(t)
	original := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, staticVarargsReferenceFixture, "DefaultCompilerCases", "none")
	originalPath := t.TempDir()
	for name, raw := range original {
		if err := os.WriteFile(filepath.Join(originalPath, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	oracle := t04RunJava(t, java, originalPath, "DefaultCompilerDriver")
	if !strings.HasPrefix(oracle, "43:static-varargs:") {
		t.Fatal("independent original arithmetic/ABI observer failed", oracle)
	}
	for _, mutation := range []struct{ name, from, to string }{
		{"correct values wrong implementation port", "static IntToLongFunction wide(){return DefaultCompilerCases::checksum;}", "static IntToLongFunction wide(){return value->checksum(value);}"},
		{"duplicated packed input", "static BiFunction<Long,Integer,String> mixed(){return DefaultCompilerCases::prefix;}", "static BiFunction<Long,Integer,String> mixed(){return (first,second)->prefix(first,second,second);}"},
		{"changed fixed input", "return String.valueOf(seed)+Arrays.deepToString(values);", "return String.valueOf(seed^1L)+Arrays.deepToString(values);"},
		{"nonempty supposedly empty pack", "static Supplier<String> empty(){return DefaultCompilerCases::show;}", "static Supplier<String> empty(){return ()->show((Object)null);}"},
		{"lost null unboxing failure", "static Function<Integer,Long> unboxed(){return DefaultCompilerCases::checksum;}", "static Function<Integer,Long> unboxed(){return value->value==null?0L:checksum(value);}"},
		{"lost void effect", "last=checksum(values);", "last=0;"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if strings.Count(staticVarargsReferenceFixture, mutation.from) != 1 {
				t.Fatal("mutant must change exactly one authored source expression")
			}
			fixture := strings.Replace(staticVarargsReferenceFixture, mutation.from, mutation.to, 1)
			classes := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, fixture, "DefaultCompilerCases", "none")
			out := t.TempDir()
			for name, raw := range classes {
				if name == "DefaultCompilerDriver.class" {
					raw = original[name]
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, java, "-Xverify:all", "-Xmx128m", "-Dfile.encoding=UTF-8", "-cp", out, "DefaultCompilerDriver")
			command.WaitDelay = time.Second
			output, err := command.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("mutant timeout is not a semantic rejection", ctx.Err())
			}
			if mutation.name == "correct values wrong implementation port" {
				if err != nil || !strings.HasPrefix(string(output), "43:static-varargs:") || string(output) == oracle {
					t.Fatalf("ABI observer did not distinguish behaviorally correct arrow: err=%v output=%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), "java.lang.AssertionError") {
				t.Fatalf("compiled semantic mutant escaped immutable original model: err=%v output=%s", err, output)
			}
		})
	}
}
