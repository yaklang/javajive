package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The erased accessor returns List, although the source field is List<String>.
// Its assignment expression must retain that erasure when it participates
// in generic method argument inference; a cast on the enclosing result is too
// late. All originals are ordinary javac inputs; no Signature edit.
func TestAdversarialSetterOperandKeepsGenericInvocationInference(t *testing.T) {
	javac, java := t04Tools(t)
	for _, storage := range []string{"own instance", "own static", "inherited instance", "inherited static"} {
		for _, prefix := range []string{"Binding", "Cove"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(storage+"/"+prefix+"/"+debug, func(t *testing.T) {
					static := ""
					if strings.Contains(storage, "static") {
						static = "static "
					}
					inherited := strings.HasPrefix(storage, "inherited")
					field := "private " + static + "volatile java.util.List<String> value;"
					extends := ""
					if inherited {
						field = ""
						extends = " extends p.BindingBase"
					}
					args := "BindingOwner owner,java.util.List value"
					lhs := "owner.value"
					call := "writer.<Object>put(owner,token,element)"
					current := "owner.current()"
					if static != "" {
						args = "java.util.List value"
						lhs = "BindingOwner.value"
						call = "writer.<Object>put(token,element)"
						current = "BindingOwner.current()"
					}
					ss := map[string]string{
						"p/BindingBase.java":   `package p;public class BindingBase{protected ` + static + `volatile java.util.List<String> value;}`,
						"q/BindingOwner.java":  `package q;public class BindingOwner` + extends + `{` + field + `public ` + static + `Object current(){return value;}public static <T> java.util.List<T> select(java.util.List<T> list,T token){return list;}public static class Writer{public <T> java.util.List<T> put(` + args + `,T token){return select((java.util.List)(` + lhs + `=value),token);}}}`,
						"q/BindingDriver.java": `package q;public class BindingDriver{public static void main(String[]args)throws Exception{String[]cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);java.io.File target=new java.io.File(cp[0]).getCanonicalFile(),original=new java.io.File(cp[cp.length-1]).getCanonicalFile();for(Class<?> c:new Class<?>[]{BindingOwner.class,BindingOwner.Writer.class,BindingDriver.class,p.BindingBase.class}){java.io.File expected=c==BindingOwner.class||c==BindingOwner.Writer.class?target:original;if(!new java.io.File(c.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(expected))throw new AssertionError("wrong target/producer/driver origin "+c);}if(BindingOwner.Writer.class.getDeclaringClass()!=BindingOwner.class)throw new AssertionError("lost native member scope");BindingOwner owner=new BindingOwner();BindingOwner.Writer writer=new BindingOwner.Writer();java.util.ArrayList<Object> token=new java.util.ArrayList<Object>();Object element=new Object();token.add(element);Object got=` + call + `;if(got!=token||` + current + `!=token||token.get(0)!=element)throw new AssertionError("assignment/result erasure/object identity");System.out.println("setter:erasure:generic:identity:volatile:scope");}}`,
					}
					sources := map[string]string{}
					for n, s := range ss {
						sources[strings.ReplaceAll(n, "Binding", prefix)] = strings.ReplaceAll(s, "Binding", prefix)
					}
					files := nativeCompileSourceReleaseClasses(t, sources, debug, "8")
					write := func(dir, n string, raw []byte) {
						t.Helper()
						p := filepath.Join(dir, n)
						if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(p, raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					original := t.TempDir()
					for n, raw := range files {
						write(original, n, raw)
					}
					run := func(cp string) {
						t.Helper()
						out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "q."+prefix+"Driver").CombinedOutput()
						if e != nil || string(out) != "setter:erasure:generic:identity:volatile:scope\n" {
							t.Fatalf("original-driver:%v\n%s", e, out)
						}
					}
					run(original)
					z := nativeArchive(t, files)
					defer z.Close()
					out := t.TempDir()
					var paths, names []string
					for n := range files {
						if strings.HasPrefix(n, "q/"+prefix+"Owner") {
							names = append(names, n)
						}
					}
					sort.Strings(names)
					for _, n := range names {
						source, e := z.ReadFile(n)
						if e != nil || strings.Contains(string(source), DecompileStubMarker) {
							t.Fatalf("source %s:%v\n%s", n, e, source)
						}
						path := strings.TrimSuffix(n, ".class") + ".java"
						write(out, path, source)
						paths = append(paths, filepath.Join(out, path))
					}
					if log, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", original, "-d", out}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("whole generic source compilation:%v\n%s", e, log)
					}
					for n, raw := range files {
						got, e := os.ReadFile(filepath.Join(original, n))
						if e != nil || !bytes.Equal(got, raw) {
							t.Fatal("original producer/driver bytes changed", n)
						}
					}
					run(out + string(os.PathListSeparator) + original)
				})
			}
		}
	}
}
