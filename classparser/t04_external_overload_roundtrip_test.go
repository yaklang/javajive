package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestT04ExternalOverloadBindingRoundTrip keeps the class named by the invoke
// instruction outside the decompiler's resolver. javac compiled the call using
// pick(Object), while the visible String overload would steal the call if the
// decompiler dropped the descriptor cast. The runtime output is an independent
// semantic oracle; the unsupported status is the required proof boundary.
func TestT04ExternalOverloadBindingRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	cases := []struct {
		name, libraryFile, library, caller string
	}{
		{
			name: "static", libraryFile: "Lib.java",
			library: `package ext;
public class Lib {
  public static String pick(Object value) { return "Object"; }
  public static String pick(String value) { return "String"; }
}`,
			caller: `public class Caller {
  public static void main(String[] args) {
    String payload = "x";
    System.out.println(ext.Lib.pick((Object) payload));
    System.out.println(ext.Lib.pick((Object) null));
  }
}`,
		},
		{
			name: "virtual", libraryFile: "Base.java",
			library: `package ext;
public class Base {
  public String pick(Object value) { return "Object"; }
  public String pick(String value) { return "String"; }
}`,
			caller: `public class Caller {
  public static void main(String[] args) {
    String payload = "x";
    ext.Base receiver = new ext.Base();
    System.out.println(receiver.pick((Object) payload));
    System.out.println(receiver.pick((Object) null));
  }
}`,
		},
		{
			name: "interface", libraryFile: "Api.java",
			library: `package ext;
public interface Api {
  String pick(Object value);
  default String pick(String value) { return "String"; }
  static Api instance() { return new Impl(); }
  class Impl implements Api {
    public String pick(Object value) { return "Object"; }
  }
}`,
			caller: `public class Caller {
  public static void main(String[] args) {
    String payload = "x";
    ext.Api receiver = ext.Api.instance();
    System.out.println(receiver.pick((Object) payload));
    System.out.println(receiver.pick((Object) null));
  }
}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, debug := range []string{"-g", "-g:none"} {
				t.Run(debug, func(t *testing.T) {
					root := t.TempDir()
					originalDir := filepath.Join(root, "original")
					if err := os.MkdirAll(originalDir, 0o755); err != nil {
						t.Fatal(err)
					}
					libPath := filepath.Join(root, tc.libraryFile)
					callerPath := filepath.Join(root, "Caller.java")
					for path, source := range map[string]string{libPath: tc.library, callerPath: tc.caller} {
						if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					compileOriginal := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", originalDir, libPath, callerPath)
					if out, err := compileOriginal.CombinedOutput(); err != nil {
						t.Fatalf("compile original overload fixture: %v\n%s", err, out)
					}
					want := t04RunJava(t, java, originalDir, "Caller")
					if want != "Object\nObject\n" {
						t.Fatalf("independent original oracle changed: got %q", want)
					}
					callerBytes, err := os.ReadFile(filepath.Join(originalDir, "Caller.class"))
					if err != nil {
						t.Fatal(err)
					}

					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, err := DecompileWithOptions(callerBytes, DecompileOptions{Mode: mode})
							if err != nil {
								t.Fatalf("decompile caller: %v", err)
							}
							if result.Status != "unsupported" {
								t.Fatalf("unknown external overload family must not be reported complete: status=%q diagnostics=%+v", result.Status, result.Diagnostics)
							}
							if !strings.Contains(result.Source, "(Object)") {
								t.Fatalf("invoke descriptor pin was lost from source:\n%s", result.Source)
							}
							foundDiagnostic := false
							for _, diagnostic := range result.Diagnostics {
								foundDiagnostic = foundDiagnostic || diagnostic.Code == "overload_family_unknown"
							}
							if !foundDiagnostic {
								t.Fatalf("unknown overload family lacks its explicit diagnostic: %+v", result.Diagnostics)
							}

							decompiledPath := filepath.Join(root, string(mode), "Caller.java")
							rebuiltDir := filepath.Dir(decompiledPath)
							if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
								t.Fatal(err)
							}
							if err := os.WriteFile(decompiledPath, []byte(result.Source), 0o644); err != nil {
								t.Fatal(err)
							}
							compileRebuilt := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-cp", originalDir, "-d", rebuiltDir, decompiledPath)
							if out, err := compileRebuilt.CombinedOutput(); err != nil {
								t.Fatalf("recompile decompiled caller (%s/%s): %v\n%s\n----- source -----\n%s", mode, debug, err, out, result.Source)
							}
							classpath := rebuiltDir + string(os.PathListSeparator) + originalDir
							if got := t04RunJava(t, java, classpath, "Caller"); got != want {
								t.Fatalf("rebuilt overload binding changed behavior (%s/%s): got %q want %q\n%s", mode, debug, got, want, result.Source)
							}
						})
					}
				})
			}
		})
	}
}
