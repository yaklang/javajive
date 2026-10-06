package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// IRETURN itself narrows B/C/S/Z, even when an optimizer removes the explicit
// source conversion. These authored two-op methods are independently verified
// and run on the real JVM; the unchanged driver supplies every expected word.
func readOnlyNarrowReturnOracle(t *testing.T) (map[string][]byte, []string) {
	t.Helper()
	source := `final class NarrowEvidence {final byte b(int x){return (byte)x;}final short s(int x){return(short)x;}final char c(int x){return(char)x;}final boolean z(int x){return (x&1)!=0;}final int i(int x){return x;}}
 final class NarrowPublication {static Object saved;NarrowPublication(){if(small(128)<0)saved=this;}final byte small(int x){return(byte)x;}}
 public class NarrowReturnDriver{public static void main(String[]args){NarrowEvidence e=new NarrowEvidence();for(int x:new int[]{Integer.MIN_VALUE,-65537,-32769,-129,-128,-1,0,1,2,3,127,128,32768,65536,Integer.MAX_VALUE})System.out.println(x+":"+e.b(x)+":"+e.s(x)+":"+(int)e.c(x)+":"+(e.z(x)?1:0)+":"+e.i(x));NarrowPublication p=new NarrowPublication();if(NarrowPublication.saved!=p)throw new AssertionError("narrowing hid receiver publication");System.out.println("publication:true");}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"NarrowReturnDriver.java": source}, "none", "8")
	java, _ := exec.LookPath("java")
	if java == "" {
		t.Fatal("JVM missing")
	}
	run := func(inputs map[string][]byte) string {
		dir := t.TempDir()
		for name, b := range inputs {
			if e := os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
		out, e := exec.Command(java, "-Xverify:all", "-cp", dir, "NarrowReturnDriver").CombinedOutput()
		if e != nil {
			t.Fatalf("reviewed original JVM: %v\n%s", e, out)
		}
		return string(out)
	}
	original := run(files)
	for _, name := range []string{"NarrowEvidence.class", "NarrowPublication.class"} {
		obj, e := Parse(bytes.Clone(files[name]))
		if e != nil {
			t.Fatal(e)
		}
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if n == "<init>" || n == "<clinit>" {
				continue
			}
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					code.Code = []byte{byte(core.OP_ILOAD_1), byte(core.OP_IRETURN)}
					code.MaxStack = 1
					code.MaxLocals = 2
					code.Attributes = nil
					code.ExceptionTable = nil
					code.AttrLen = 14
				}
			}
		}
		files[name] = obj.Bytes()
		if _, e := Parse(bytes.Clone(files[name])); e != nil {
			t.Fatalf("serialized original packet: %v", e)
		}
	}
	optimized := run(files)
	if optimized != original {
		t.Fatalf("two-op original protocol changed JVM semantics\n%s\n%s", original, optimized)
	}
	rows := strings.Split(strings.TrimSpace(optimized), "\n")
	if len(rows) != 16 || rows[15] != "publication:true" {
		t.Fatalf("incomplete JVM oracle %q", rows)
	}
	return files, rows[:15]
}
func TestAdversarialConstructorReadOnlyReturnFactsMatchOriginalJVM(t *testing.T) {
	files, rows := readOnlyNarrowReturnOracle(t)
	obj, e := Parse(bytes.Clone(files["NarrowEvidence.class"]))
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		words := strings.Split(row, ":")
		if len(words) != 6 {
			t.Fatal(row)
		}
		input, e := strconv.ParseInt(words[0], 10, 32)
		if e != nil {
			t.Fatal(e)
		}
		for column, view := range []struct{ name, desc string }{{"b", "B"}, {"s", "S"}, {"c", "C"}, {"z", "Z"}, {"i", "I"}} {
			t.Run(words[0]+"/"+view.desc, func(t *testing.T) {
				want, e := strconv.ParseInt(words[column+1], 10, 32)
				if e != nil {
					t.Fatal(e)
				}
				remaining := 512
				reader := &ClassObjectDumper{obj: obj}
				member := &values.JavaClassMember{Name: obj.GetClassName(), Member: view.name, Description: "(I)" + view.desc}
				actual, ok := reader.constructorReceiverReadOnlyMethod(obj, member, core.OP_INVOKEVIRTUAL, map[string]bool{}, &remaining, constructorEffectValue{kind: 'I', knownInt: true, intWord: int32(input)})
				if !ok || actual.kind != 'I' || !actual.knownInt || actual.intWord != int32(want) {
					t.Fatalf("original JVM=%d proof=%+v accepted=%v", want, actual, ok)
				}
			})
		}
	}
}
func TestAdversarialConstructorReadOnlyReturnCannotHideReceiverPublication(t *testing.T) {
	files, _ := readOnlyNarrowReturnOracle(t)
	obj, e := Parse(bytes.Clone(files["NarrowPublication.class"]))
	if e != nil {
		t.Fatal(e)
	}
	resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
	reader := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
	reader.options.TargetSourceVersion = 8
	if reader.constructorCaptureChainDoesNotObserve(obj.GetClassName(), "()V", map[string]bool{}) {
		t.Fatal("original JVM publishes THIS after byte-return narrowing; a pre-initialization capture must not move")
	}
}

// Rebuild the optimized method bodies through the production API as well. The
// untouched original driver stays on the classpath, after candidate classes,
// so javac and the JVM cannot silently borrow a selected original method.
func TestAdversarialConstructorReadOnlyOptimizedReturnRoundTrip(t *testing.T) {
	files, rows := readOnlyNarrowReturnOracle(t)
	javac, java := t04Tools(t)
	for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
		t.Run(string(mode), func(t *testing.T) {
			original, rebuilt := t.TempDir(), t.TempDir()
			for name, raw := range files {
				if name == "NarrowEvidence.class" || name == "NarrowPublication.class" {
					continue
				}
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			var sources []string
			for _, name := range []string{"NarrowEvidence", "NarrowPublication"} {
				var source string
				var err error
				if mode == "legacy" {
					source, err = DecompileWithResolver(bytes.Clone(files[name+".class"]), resolve)
				} else {
					var result DecompileResult
					result, err = DecompileWithOptions(bytes.Clone(files[name+".class"]), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					source = result.Source
				}
				if err != nil {
					t.Fatal(err)
				}
				file := filepath.Join(rebuilt, name+".java")
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				sources = append(sources, file)
			}
			args := append([]string{"-proc:none", "--release", "8", "-d", rebuilt, "-cp", original}, sources...)
			if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
				for _, s := range sources {
					b, _ := os.ReadFile(s)
					t.Log(string(b))
				}
				t.Fatalf("candidate compile: %v\n%s", err, out)
			}
			out, err := exec.Command(java, "-Xverify:all", "-cp", rebuilt+string(os.PathListSeparator)+original, "NarrowReturnDriver").CombinedOutput()
			if err != nil {
				t.Fatalf("candidate JVM: %v\n%s", err, out)
			}
			want := strings.Join(append(append([]string{}, rows...), "publication:true"), "\n") + "\n"
			if string(out) != want {
				t.Fatalf("original JVM=%q candidate=%q", want, out)
			}
		})
	}
}
