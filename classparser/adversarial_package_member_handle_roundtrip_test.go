package javaclassparser

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The retained binary caller keeps its original bootstrap lookup. Package
// methods have the same physical declaration after lexical reconstruction;
// bound, unbound and static references must keep identity and null timing.
func TestAdversarialPackageMemberHandlesKeepOriginalLookup(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, pkg := range []string{"", "handle.boundary"} {
			namespace := pkg
			if namespace == "" {
				namespace = "unnamed"
			}
			for _, layout := range []string{"bound", "unbound", "static"} {
				t.Run(compiler.name+"/"+namespace+"/"+layout, func(t *testing.T) {
					source := strings.Replace(nativeOrdinaryHandleFixture, "public Object echo(Object value)", "Object echo(Object value)", 1)
					if layout == "unbound" {
						source = strings.Replace(source, "java.util.function.Function<Object,Object> echo=child::echo;", "java.util.function.BiFunction<HandleOwner.Child,Object,Object> echo=HandleOwner.Child::echo;", 1)
						source = strings.Replace(source, "echo.apply(value)", "echo.apply(child,value)", 1)
					}
					if layout == "static" {
						source = strings.Replace(source, "class Child extends HandleParent", "static class StaticValue{static Object echo(Object value){return value;}}class Child extends HandleParent", 1)
						source = strings.Replace(source, "echo=child::echo;", "echo=HandleOwner.StaticValue::echo;", 1)
					}
					owner, driver := "HandleOwner", "HandleOracle"
					if pkg != "" {
						source = "package " + pkg + ";\n" + source
						owner = strings.ReplaceAll(pkg, ".", "/") + "/" + owner
						driver = pkg + "." + driver
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile != NativeJavac8 {
							return nativeCompileDebugClasses(t, source, debug)
						}
						dir := t.TempDir()
						file := filepath.Join(dir, "HandleOwner.java")
						if e := os.WriteFile(file, []byte(source), 0600); e != nil {
							t.Fatal(e)
						}
						if out, e := exec.Command(compiler.javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, file).CombinedOutput(); e != nil {
							t.Fatal("original compile", e, string(out))
						}
						files := map[string][]byte{}
						if e := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, e error) error {
							if e != nil {
								return e
							}
							if !entry.IsDir() && strings.HasSuffix(path, ".class") {
								rel, e := filepath.Rel(dir, path)
								if e != nil {
									return e
								}
								raw, e := os.ReadFile(path)
								if e != nil {
									return e
								}
								files[filepath.ToSlash(rel)] = raw
							}
							return nil
						}); e != nil {
							t.Fatal(e)
						}
						return files
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, driver, "ordinary:handle:capture:identity:null\n", nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
