package javaclassparser

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Introduced standard intrinsics need the same owner/descriptor proof as an
// original invokestatic. The original target contains only finite constants;
// patching its CP supplies independently observed NaN words without a wrapper
// reference that could accidentally pre-populate collision metadata.
func TestAdversarialNaNIntrinsicKeepsTypeAndPackageNamespaces(t *testing.T) {
	javac, javaTool := t04Tools(t)
	for _, scope := range []string{"method_formals", "class_formals", "package_root_type"} {
		for _, kind := range []string{"float", "double"} {
			t.Run(scope+"/"+kind, func(t *testing.T) {
				literal, rawAPI, expected := "1.25F", "Float.floatToRawIntBits", uint64(0xffc01234)
				if kind == "double" {
					literal, rawAPI, expected = "1.25D", "Double.doubleToRawLongBits", 0xfff8000012345678
				}
				header, method, prelude := "class NaNTypeOwner", "static <Float,Double,java> "+kind+" value()", ""
				call := "NaNTypeOwner.value()"
				if scope == "class_formals" {
					header = "class NaNTypeOwner<Float,Double,java>"
					method = kind + " value()"
					call = "new NaNTypeOwner<Object,Object,Object>().value()"
				}
				if scope == "package_root_type" {
					method = "static <Float,Double> " + kind + " value()"
					prelude = "class java{static class lang{static class Float{static float intBitsToFloat(int v){throw new AssertionError(\"wrong float owner\");}}static class Double{static double longBitsToDouble(long v){throw new AssertionError(\"wrong double owner\");}}}}"
				}
				source := prelude + header + "{" + method + "{return " + literal + ";}}"
				driver := "import java.lang.Float;import java.lang.Double;class NaNTypeDriver{public static void main(String[]x){System.out.println(" + map[string]string{"float": "Integer", "double": "Long"}[kind] + ".toHexString(" + rawAPI + "(" + call + ")));}}"
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(debug, func(t *testing.T) {
						files := nativeCompileSourceReleaseClasses(t, map[string]string{"NaNTypeOwner.java": source, "NaNTypeDriver.java": driver}, debug, "8")
						obj, e := Parse(files["NaNTypeOwner.class"])
						if e != nil {
							t.Fatal(e)
						}
						count := 0
						for _, constant := range obj.ConstantPool {
							switch c := constant.(type) {
							case *ConstantFloatInfo:
								if kind == "float" && math.Float32bits(c.Value) == 0x3fa00000 {
									c.Value = math.Float32frombits(uint32(expected))
									count++
								}
							case *ConstantDoubleInfo:
								if kind == "double" && math.Float64bits(c.Value) == 0x3ff4000000000000 {
									c.Value = math.Float64frombits(expected)
									count++
								}
							}
						}
						if count != 1 {
							t.Fatal("original word count", count)
						}
						original := t.TempDir()
						for name, b := range files {
							if name == "NaNTypeOwner.class" {
								b = obj.Bytes()
							}
							if e := os.WriteFile(filepath.Join(original, name), b, 0600); e != nil {
								t.Fatal(e)
							}
						}
						want := fmt.Sprintf("%x\n", expected)
						if got := t04RunJava(t, javaTool, original, "NaNTypeDriver"); got != want {
							t.Fatal("original-first word oracle", got, want)
						}
						t.Log("ORIGINAL_FIRST", scope, kind, want)
						for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
							t.Run(string(mode), func(t *testing.T) {
								resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
								var output string
								var err error
								if mode == "legacy" {
									output, err = DecompileWithResolver(obj.Bytes(), resolve)
								} else {
									r, e := DecompileWithOptions(obj.Bytes(), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
									output, err = r.Source, e
								}
								if err != nil {
									t.Fatal("candidate binding", err)
								}
								if strings.Contains(output, DecompileStubMarker) {
									t.Fatal("placeholder", output)
								}
								candidate := t.TempDir()
								for name, b := range files {
									if name == "NaNTypeOwner.class" {
										continue
									}
									if e := os.WriteFile(filepath.Join(candidate, name), b, 0600); e != nil {
										t.Fatal(e)
									}
								}
								p := filepath.Join(candidate, "NaNTypeOwner.java")
								if e := os.WriteFile(p, []byte(output), 0600); e != nil {
									t.Fatal(e)
								}
								if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", candidate, "-d", candidate, p).CombinedOutput(); e != nil {
									t.Fatalf("candidate compile %v %s\n%s", e, out, output)
								}
								if got := t04RunJava(t, javaTool, candidate, "NaNTypeDriver"); got != want {
									t.Fatal("compiled candidate differs", got, want)
								}
							})
						}
					})
				}
			})
		}
	}
}

// A JVM ldc does not call either same-spelled method. When both type paths
// are obscured and an own/inherited method prevents a proved static import,
// emitting a compilable decoy would change the original word or throw.
func TestAdversarialNaNIntrinsicRejectsConflictingMethodNamespace(t *testing.T) {
	_, javaTool := t04Tools(t)
	for _, scope := range []string{"own", "ancestor"} {
		for _, kind := range []string{"float", "double"} {
			t.Run(scope+"/"+kind, func(t *testing.T) {
				literal, member, descriptor, word, rawAPI := "1.25F", "intBitsToFloat", "()F", uint64(0xffc01234), "Integer.toHexString(Float.floatToRawIntBits(NaNReject.value()))"
				arg := "int x"
				if kind == "double" {
					literal, member, descriptor, word, rawAPI = "1.25D", "longBitsToDouble", "()D", 0xfff8000012345678, "Long.toHexString(Double.doubleToRawLongBits(NaNReject.value()))"
					arg = "long x"
				}
				decoy := "static " + kind + " " + member + "(" + arg + "){throw new AssertionError(\"decoy invocation\");}"
				header, body, prelude := "class NaNReject", decoy, ""
				if scope == "ancestor" {
					header, body, prelude = "class NaNReject extends NaNRejectParent", "", "class NaNRejectParent{"+decoy+"}"
				}
				source := prelude + header + "{" + body + "static <Float,Double,java> " + kind + " value(){return " + literal + ";}}class NaNRejectDriver{public static void main(String[]x){System.out.println(" + rawAPI + ");}}"
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(debug, func(t *testing.T) {
						files := nativeCompileSourceReleaseClasses(t, map[string]string{"NaNReject.java": source}, debug, "8")
						obj, err := Parse(files["NaNReject.class"])
						if err != nil {
							t.Fatal(err)
						}
						patched := 0
						for _, entry := range obj.ConstantPool {
							switch c := entry.(type) {
							case *ConstantFloatInfo:
								if kind == "float" && math.Float32bits(c.Value) == 0x3fa00000 {
									c.Value = math.Float32frombits(uint32(word))
									patched++
								}
							case *ConstantDoubleInfo:
								if kind == "double" && math.Float64bits(c.Value) == 0x3ff4000000000000 {
									c.Value = math.Float64frombits(word)
									patched++
								}
							}
						}
						if patched != 1 {
							t.Fatal("original word count", patched)
						}
						files["NaNReject.class"] = obj.Bytes()
						original := t.TempDir()
						for name, raw := range files {
							if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						if got, want := t04RunJava(t, javaTool, original, "NaNRejectDriver"), fmt.Sprintf("%x\n", word); got != want {
							t.Fatal("original-first oracle", got, want)
						}
						resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
						for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
							t.Run(string(mode), func(t *testing.T) {
								if mode == "legacy" {
									text, err := DecompileWithResolver(files["NaNReject.class"], resolve)
									if err == nil || text != "" {
										t.Fatalf("legacy invented intrinsic: %v %s", err, text)
									}
									return
								}
								result, err := DecompileWithOptions(files["NaNReject.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
								if err == nil || result.Source != "" || result.Status != "unsupported" || len(result.StubMethods) != 0 {
									t.Fatalf("unproved call published: %v %+v", err, result)
								}
								found := false
								for _, d := range result.Diagnostics {
									if d.Code == "static_owner_binding_unknown" && d.Method == "NaNReject.value"+descriptor {
										found = true
									}
								}
								if !found {
									t.Fatalf("missing original caller diagnosis: %+v", result.Diagnostics)
								}
								for _, m := range result.Members {
									if m.State != "unsupported" || !strings.Contains(m.Evidence, "source unit rejected") {
										t.Fatalf("rejected unit retained positive witness: %+v", m)
									}
								}
							})
						}
					})
				}
			})
		}
	}
}
