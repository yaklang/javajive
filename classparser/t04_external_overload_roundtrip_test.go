package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestT04ExternalOverloadBindingRoundTrip supplies only the external declaring
// classfile as a resolver witness. javac compiled the call using pick(Object),
// while the visible String overload would steal the call if the decompiler
// dropped the descriptor cast. Runtime output is the independent oracle.
func TestT04ExternalOverloadBindingRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	cases := []struct {
		name, ownerInternal, libraryFile, library, caller string
	}{
		{
			name: "static", ownerInternal: "ext/Lib", libraryFile: "Lib.java",
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
			name: "virtual", ownerInternal: "ext/Base", libraryFile: "Base.java",
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
			name: "interface", ownerInternal: "ext/Api", libraryFile: "Api.java",
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
					ownerBytes, err := os.ReadFile(filepath.Join(originalDir, filepath.FromSlash(tc.ownerInternal)+".class"))
					if err != nil {
						t.Fatalf("read resolved overload owner %s: %v", tc.ownerInternal, err)
					}

					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, err := DecompileWithOptions(callerBytes, DecompileOptions{
								Mode: mode,
								Resolve: func(internalName string) ([]byte, bool) {
									if internalName == tc.ownerInternal {
										return ownerBytes, true
									}
									return nil, false
								},
							})
							if err != nil {
								t.Fatalf("decompile caller: %v", err)
							}
							if !strings.Contains(result.Source, "(Object)") {
								t.Fatalf("invoke descriptor pin was lost from source:\n%s", result.Source)
							}
							foundUnknownDiagnostic := false
							for _, diagnostic := range result.Diagnostics {
								foundUnknownDiagnostic = foundUnknownDiagnostic || diagnostic.Code == "overload_family_unknown"
							}
							if foundUnknownDiagnostic {
								t.Fatalf("resolved overload family remained unknown: %+v", result.Diagnostics)
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

// TestT04UnknownExternalGenericReceiverPreservesSourceTyping exercises the
// opposite side of overload pinning: an erased Object descriptor can also be
// a generic T formal. Without the declaring class bytes, forcing an Object cast
// may preserve an overload while making a parameterized receiver ill-typed.
func TestT04UnknownExternalGenericReceiverPreservesSourceTyping(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		t.Run(debug, func(t *testing.T) {
			root := t.TempDir()
			originalDir := filepath.Join(root, "original")
			if err := os.MkdirAll(originalDir, 0o755); err != nil {
				t.Fatal(err)
			}
			libraryPath := filepath.Join(root, "Box.java")
			callerPath := filepath.Join(root, "Caller.java")
			library := `package ext;
public class Box<T> {
  private T value;
  public void set(T value) { this.value = value; }
  public T get() { return this.value; }
}`
			caller := `public class Caller {
  private static final ext.Box<String> box = new ext.Box<String>();
  public static void main(String[] args) {
    box.set("bound");
    System.out.println(box.get());
  }
}`
			for path, source := range map[string]string{libraryPath: library, callerPath: caller} {
				if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			compileOriginal := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-d", originalDir, libraryPath, callerPath)
			if out, err := compileOriginal.CombinedOutput(); err != nil {
				t.Fatalf("compile original generic fixture: %v\n%s", err, out)
			}
			want := t04RunJava(t, java, originalDir, "Caller")
			callerBytes, err := os.ReadFile(filepath.Join(originalDir, "Caller.class"))
			if err != nil {
				t.Fatal(err)
			}

			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					// Deliberately omit ext.Box from Resolve: this models an external
					// dependency whose generic source signature is unavailable.
					result, err := DecompileWithOptions(callerBytes, DecompileOptions{Mode: mode})
					if err != nil {
						t.Fatalf("decompile generic caller: %v", err)
					}
					if result.Status != "unsupported" {
						t.Fatalf("unknown external method family must remain unsupported: status=%q diagnostics=%+v", result.Status, result.Diagnostics)
					}
					foundDiagnostic := false
					for _, diagnostic := range result.Diagnostics {
						foundDiagnostic = foundDiagnostic || diagnostic.Code == "overload_family_unknown"
					}
					if !foundDiagnostic {
						t.Fatalf("unknown external generic family lacks its diagnostic: %+v", result.Diagnostics)
					}

					rebuiltDir := filepath.Join(root, string(mode), "rebuilt")
					if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
						t.Fatal(err)
					}
					decompiledPath := filepath.Join(rebuiltDir, "Caller.java")
					if err := os.WriteFile(decompiledPath, []byte(result.Source), 0o644); err != nil {
						t.Fatal(err)
					}
					compileRebuilt := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", debug, "-cp", originalDir, "-d", rebuiltDir, decompiledPath)
					if out, err := compileRebuilt.CombinedOutput(); err != nil {
						t.Fatalf("recompile generic caller (%s/%s): %v\n%s\n----- source -----\n%s", mode, debug, err, out, result.Source)
					}
					classpath := rebuiltDir + string(os.PathListSeparator) + originalDir
					if got := t04RunJava(t, java, classpath, "Caller"); got != want {
						t.Fatalf("generic call behavior changed (%s/%s): got %q want %q\n%s", mode, debug, got, want, result.Source)
					}
				})
			}
		})
	}
}
