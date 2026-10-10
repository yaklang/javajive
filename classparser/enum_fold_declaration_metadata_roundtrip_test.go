package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Folding a constant body changes source placement, not its original declaration
// graph. A checked exception and an inherited helper-name collision distinguish
// complete original metadata from a CP-only or isolated child view.
func TestNativeEnumFoldKeepsOriginalDeclarationMetadata(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `
class EnumMetadataEffects {
 static String trace="";
 static final ClassNotFoundException failure=new ClassNotFoundException("original");
 static Class<?> load(String name)throws ClassNotFoundException {trace+="L";if(name==null)throw failure;return Class.forName(name);}
}
enum EnumMetadataChoice {
 A { public Class<?> read(String name)throws ClassNotFoundException{return EnumMetadataEffects.load(name);} };
 public abstract Class<?> read(String name)throws ClassNotFoundException;
 public static RuntimeException jdec$rethrow$0(Throwable failure){EnumMetadataEffects.trace+="BAD";return new IllegalStateException(failure);}
 public final RuntimeException jdec$rethrow$1(Throwable failure){EnumMetadataEffects.trace+="BAD1";return new IllegalStateException(failure);}
}
class EnumMetadataDriver {
 public static void main(String[]args)throws Exception {
  if(EnumMetadataChoice.A.read("java.lang.String")!=String.class)throw new AssertionError("result");
  try {EnumMetadataChoice.A.read(null);throw new AssertionError("missing failure");}catch(Throwable e){if(e!=EnumMetadataEffects.failure)throw new AssertionError("identity",e);}
  if(EnumMetadataChoice.A.getClass().getDeclaredMethod("read",String.class).getExceptionTypes().length!=0)throw new AssertionError("original throws metadata");
  if(!EnumMetadataEffects.trace.equals("LL"))throw new AssertionError("trace:"+EnumMetadataEffects.trace);
  System.out.println("String:identity:LL");
 }
}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			obj, e := Parse(files["EnumMetadataChoice$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			removed := 0
			for _, method := range obj.Methods {
				name, _ := obj.getUtf8(method.NameIndex)
				if name != "read" {
					continue
				}
				attrs := method.Attributes[:0]
				for _, attr := range method.Attributes {
					if _, ok := attr.(*ExceptionsAttribute); ok {
						removed++
						continue
					}
					attrs = append(attrs, attr)
				}
				method.Attributes = attrs
			}
			if removed != 1 {
				t.Fatalf("caller Exceptions removals=%d", removed)
			}
			files["EnumMetadataChoice$1.class"] = obj.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			want := t04RunJava(t, java, original, "EnumMetadataDriver")
			if want != "String:identity:LL\n" {
				t.Fatalf("independent original JVM=%q", want)
			}
			for _, declarations := range []string{"archive", "external-declarations"} {
				t.Run(declarations, func(t *testing.T) {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							archive := map[string][]byte{}
							for name, raw := range files {
								if declarations == "external-declarations" && name == "EnumMetadataEffects.class" {
									continue
								}
								archive[name] = raw
							}
							z := nativeArchive(t, archive)
							if declarations == "external-declarations" {
								z.declarationResolver = func(name string) ([]byte, bool) {
									if name != "EnumMetadataEffects" {
										return nil, false
									}
									return files["EnumMetadataEffects.class"], true
								}
							}
							defer z.Close()
							src, e := z.ReadFile("EnumMetadataChoice.class")
							if e != nil {
								t.Fatal(e)
							}
							if strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("folded source has stub:\n%s", src)
							}
							rebuilt := t.TempDir()
							for name, raw := range files {
								if strings.HasPrefix(name, "EnumMetadataChoice") {
									continue
								}
								if e := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							file := filepath.Join(rebuilt, "EnumMetadataChoice.java")
							if e := os.WriteFile(file, src, 0600); e != nil {
								t.Fatal(e)
							}
							if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt, file).CombinedOutput(); e != nil {
								t.Fatalf("rebuilt: %v\n%s\n%s", e, out, src)
							}
							if got := t04RunJava(t, java, rebuilt, "EnumMetadataDriver"); got != want {
								t.Fatalf("JVM=%q want%q", got, want)
							}
						})
					}
				})
			}
		})
	}
}
