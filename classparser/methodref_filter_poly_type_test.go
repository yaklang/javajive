package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// DirectoryStream.Filter is a nested JDK functional interface outside the
// java.util.function package. Its (Path)Z instantiated SAM signature must
// survive a method-reference snapshot so javac does not target raw Filter.accept(Object).
func TestDirectoryStreamFilterKeepsInstantiatedMethodRefTypeRoundTrip(t *testing.T) {
	want, classes := t04CompileRun(t, "17", "DirectoryFilterMethodRefMain", map[string]string{
		"DirectoryFilterMethodRefMain.java": `import java.io.IOException;
import java.nio.file.DirectoryStream;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.function.Predicate;
public class DirectoryFilterMethodRefMain {
  static DirectoryStream.Filter<Path> asFilter(Predicate<Path> predicate) {
    return predicate::test;
  }
  static boolean accepts(DirectoryStream.Filter<Path> filter, Path path) throws IOException {
    return filter.accept(path);
  }
  public static void main(String[] args) throws IOException {
    DirectoryStream.Filter<Path> filter = asFilter(path -> path.getFileName().toString().startsWith("ok"));
    System.out.println(accepts(filter, Paths.get("okay.txt")));
    System.out.println(accepts(filter, Paths.get("no.txt")));
  }
}`,
	})
	if want != "true\nfalse\n" {
		t.Fatalf("independent Java oracle changed: got %q", want)
	}

	javac, java := t04Tools(t)
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			resolver := func(internalName string) ([]byte, bool) {
				base := internalName
				if i := strings.LastIndexByte(internalName, '/'); i >= 0 {
					base = internalName[i+1:]
				}
				b, ok := classes[base]
				return b, ok
			}
			sourceDir, outDir := t.TempDir(), t.TempDir()
			var files []string
			var combined strings.Builder
			for name, raw := range classes {
				result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolver})
				if err != nil {
					t.Fatalf("decompile %s: %v", name, err)
				}
				combined.WriteString(result.Source)
				combined.WriteByte('\n')
				path := filepath.Join(sourceDir, name+".java")
				if err := os.WriteFile(path, []byte(result.Source), 0o644); err != nil {
					t.Fatal(err)
				}
				files = append(files, path)
			}
			if !strings.Contains(combined.String(), "DirectoryStream.Filter<Path>") {
				t.Fatalf("instantiated Filter<Path> type was lost:\n%s", combined.String())
			}
			args := append([]string{"-proc:none", "-encoding", "UTF-8", "--release", "17", "-d", outDir}, files...)
			if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
				t.Fatalf("recompile %s source: %v\n%s\n----- source -----\n%s", mode, err, out, combined.String())
			}
			if got := t04RunJava(t, java, outDir, "DirectoryFilterMethodRefMain"); got != want {
				t.Fatalf("%s runtime mismatch: got %q want %q", mode, got, want)
			}
		})
	}
}
