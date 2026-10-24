package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMethodParameterNamesEnumGeneratedValueOfBoundary(t *testing.T) {
	fixture := methodParameterNameCase{name: "enum generated factory", declaration: `enum NamedOwner{A,B;int read(final int wanted){return wanted+ordinal();}}`, check: `if(NamedOwner.B.read(7)!=8||NamedOwner.valueOf("B")!=NamedOwner.B)throw new AssertionError("enum factory identity");java.lang.reflect.Parameter factory=NamedOwner.class.getDeclaredMethod("valueOf",String.class).getParameters()[0];if(!factory.getName().equals("name")||factory.getModifiers()!=32768)throw new AssertionError("generated mandated name");`, parameterTypes: `int.class`, names: []string{"wanted"}, parameters: true}
	methodParameterNameRoundTrip(t, fixture, "none")
	files := methodParameterBoundaryCompile(t, `enum GuardOwner{A,B;}`, "GuardOwner")
	object, err := Parse(files["GuardOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name != "valueOf" {
			continue
		}
		descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if method.AccessFlags&0x1000 != 0 {
			t.Fatal("fixture must use nonsynthetic generated valueOf")
		}
		mandated := false
		for _, a := range method.Attributes {
			if table, ok := a.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
				mandated = len(table.Info) == 5 && binary.BigEndian.Uint16(table.Info[3:]) == 0x8000
			}
		}
		if !mandated {
			t.Fatal("original compiler's mandated factory parameter")
		}
		if generated, err := nativeEnumRegeneratedMethod(object, method, name, descriptor, nil); err != nil || !generated {
			t.Fatalf("existing enum generation protocol: generated=%v err=%v", generated, err)
		}
		d := NewClassObjectDumper(object)
		if _, known := d.originalOrdinaryMethodParameterNames(method, descriptor); known {
			t.Fatal("ordinary name reader must not waive mandated flags")
		}
		return
	}
	t.Fatal("missing original valueOf")
}

const methodParameterBoundaryFixture = `class GuardOwner{static int read(int originalNamed){return originalNamed;}static int next(int given){return given+1;}static int local(int var2,int other){int sum=next(var2+other);return sum+sum;}static int explode(int originalNamed){return Integer.valueOf(originalNamed).intValue();}}class GuardDriver{public static void main(String[]args)throws Exception{if(GuardOwner.read(7)!=7||GuardOwner.next(7)!=8||GuardOwner.local(1,2)!=8||GuardOwner.explode(7)!=7||!GuardOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getName().equals("originalNamed"))throw new AssertionError("original method boundary");System.out.println("boundary:original");}}`

func methodParameterBoundaryCompile(t *testing.T, source, owner string) map[string][]byte {
	t.Helper()
	javac, java := t04Tools(t)
	dir := t.TempDir()
	input := filepath.Join(dir, "Authored.java")
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:none", "-d", dir, input).CombinedOutput(); err != nil {
		t.Fatalf("authored compile: %v\n%s", err, log)
	}
	files := map[string][]byte{}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".class") {
			return err
		}
		name, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		files[filepath.ToSlash(name)], e = os.ReadFile(path)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if owner == "GuardOwner" && files["GuardDriver.class"] != nil {
		got, err := exec.Command(java, "-Xverify:all", "-cp", dir, "GuardDriver").CombinedOutput()
		if err != nil || string(got) != "boundary:original\n" {
			t.Fatalf("original JVM: %v\n%s", err, got)
		}
		t.Logf("original JVM: %q", got)
	}
	return files
}

func methodParameterBoundaryDumper(t *testing.T, raw []byte) *ClassObjectDumper {
	t.Helper()
	object, err := Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(object)
	if source, err := d.DumpClass(); err != nil || strings.Contains(source, DecompileStubMarker) {
		t.Fatalf("prime source context: %v\n%s", err, source)
	}
	d.dumpedMethodsSet = map[string]*dumpedMethods{}
	id := coreutils.NewRootVariableId()
	id.SetName("sentinel")
	d.FuncCtx.LocalNames = map[*coreutils.VariableId]string{id: "keptLocal"}
	d.FuncCtx.Arguments = []string{"keptArgument"}
	return d
}

func methodParameterBoundaryScopes(t *testing.T, d *ClassObjectDumper, locals map[*coreutils.VariableId]string) {
	t.Helper()
	if !reflect.DeepEqual(d.FuncCtx.Arguments, []string{"keptArgument"}) || !reflect.DeepEqual(d.FuncCtx.LocalNames, locals) {
		t.Errorf("method source scope leaked: args=%v local names=%v want=%v", d.FuncCtx.Arguments, d.FuncCtx.LocalNames, locals)
	}
}

func TestAdversarialMethodParameterNamesNamespaceFailureRetriesFreshBody(t *testing.T) {
	files := methodParameterBoundaryCompile(t, methodParameterBoundaryFixture, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	locals := d.FuncCtx.LocalNames
	d.nativeCaptureFields = map[string]string{"reserved": "originalNamed"}
	if _, err := d.DumpMethod("read", "(I)I"); err == nil || !strings.Contains(err.Error(), "parameter namespace") {
		t.Fatalf("expected original name/capture namespace refusal, got %v", err)
	}
	methodParameterBoundaryScopes(t, d, locals)
	key := "name:read,desc:(I)I"
	if d.dumpedMethodsSet[key] != nil {
		t.Error("failed method left a successful-cache placeholder")
	}
	d.nativeCaptureFields = nil
	result, err := d.DumpMethod("read", "(I)I")
	if err != nil || result == nil || !strings.Contains(result.code, "int originalNamed") {
		t.Errorf("same-method fresh retry: err=%v result=%+v", err, result)
	}
	next, err := d.DumpMethod("next", "(I)I")
	if err != nil || next == nil || !strings.Contains(next.code, "int given") {
		t.Errorf("next-method source binder: err=%v result=%+v", err, next)
	}
	methodParameterBoundaryScopes(t, d, locals)
}

func TestAdversarialMethodParameterNamesSuccessfulLocalNamespaceDoesNotLeak(t *testing.T) {
	files := methodParameterBoundaryCompile(t, methodParameterBoundaryFixture, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	locals := map[*coreutils.VariableId]string{}
	for id, name := range d.FuncCtx.LocalNames {
		locals[id] = name
	}
	result, err := d.DumpMethod("local", "(II)I")
	if err != nil || result == nil || !strings.Contains(result.code, "int var2") {
		t.Fatalf("fixed parameter/local source namespace: %v %+v", err, result)
	}
	methodParameterBoundaryScopes(t, d, locals)
	next, err := d.DumpMethod("next", "(I)I")
	if err != nil || next == nil || !strings.Contains(next.code, "int given") {
		t.Fatalf("next method after local source namespace: %v %+v", err, next)
	}
	methodParameterBoundaryScopes(t, d, locals)
	if cached, err := d.DumpMethod("local", "(II)I"); err != nil || cached != result || cached.code != result.code {
		t.Fatal("successful immutable method cache changed")
	}
}

func TestAdversarialMethodParameterNamesPanicDropsOnlyOwnedCacheEntry(t *testing.T) {
	files := methodParameterBoundaryCompile(t, methodParameterBoundaryFixture, "GuardOwner")
	d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
	locals := map[*coreutils.VariableId]string{}
	for id, name := range d.FuncCtx.LocalNames {
		locals[id] = name
	}
	prior := d.FuncCtx.SourceValueNameShadow
	sentinel := &struct{ message string }{"authored source panic"}
	nested := false
	d.FuncCtx.SourceValueNameShadow = func(name string) bool {
		if name == "Integer" {
			d.FuncCtx.SourceValueNameShadow = prior
			inProgress := d.dumpedMethodsSet["name:explode,desc:(I)I"]
			recursive, err := d.DumpMethod("explode", "(I)I")
			if err != nil || inProgress == nil || recursive != inProgress {
				panic("in-progress recursive cache entry changed")
			}
			result, err := d.DumpMethod("next", "(I)I")
			if err != nil || result == nil || !strings.Contains(result.code, "int given") {
				panic("nested independent method failed")
			}
			nested = true
			panic(sentinel)
		}
		return prior != nil && prior(name)
	}
	var caught any
	func() { defer func() { caught = recover() }(); _, _ = d.DumpMethod("explode", "(I)I") }()
	if caught != sentinel || !nested {
		t.Fatalf("original authored panic: %v nested=%v", caught, nested)
	}
	d.FuncCtx.SourceValueNameShadow = prior
	if d.dumpedMethodsSet["name:explode,desc:(I)I"] != nil {
		t.Error("panic left its incomplete method in the cache")
	}
	if result := d.dumpedMethodsSet["name:next,desc:(I)I"]; result == nil || !strings.Contains(result.code, "int given") {
		t.Error("failed method removed an independent completed packet")
	}
	methodParameterBoundaryScopes(t, d, locals)
	result, err := d.DumpMethod("explode", "(I)I")
	if err != nil || result == nil || !strings.Contains(result.code, "int originalNamed") {
		t.Errorf("panic retry did not rebuild a fresh body: %v %+v", err, result)
	}
	methodParameterBoundaryScopes(t, d, locals)
}

func TestAdversarialMethodParameterNamesNamespaceBudgetFailureIsAtomic(t *testing.T) {
	raw := methodParameterNameGuardClass(t)
	d, method, _, header := methodParameterNameGuardInput(t, raw)
	names, known := d.originalOrdinaryMethodParameterNames(method, "(JI)J")
	if !known {
		t.Fatal("original table")
	}
	binding, known := d.bindOrdinaryMethodParameterNames(method, "(JI)J", names, header)
	if !known {
		t.Fatal("original entry names")
	}
	d.FuncCtx.Arguments = []string{"keptArgument"}
	locals := map[*coreutils.VariableId]string{}
	for id, name := range d.FuncCtx.LocalNames {
		locals[id] = name
	}
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
	if d.prepareOrdinaryMethodParameterNamespace(binding, nil, header) {
		t.Fatal("namespace budget exhaustion accepted")
	}
	if d.Work.Used(workbudget.CounterOutputBytes) != 0 {
		t.Fatal("failed namespace published source")
	}
	methodParameterBoundaryScopes(t, d, locals)
	d.Work = nil
	if !d.prepareOrdinaryMethodParameterNamespace(binding, nil, header) {
		t.Fatal("fresh budget retry rejected original named formals")
	}
	for i, value := range header {
		if value.(*values.JavaRef).String(d.FuncCtx) != names[i] {
			t.Fatal("budget retry lost original name priority")
		}
	}
	methodParameterBoundaryScopes(t, d, locals)
}

func TestAdversarialMethodParameterNamesEnclosingLambdaNamespace(t *testing.T) {
	cases := []methodParameterNameCase{
		{name: "arrow parameter", declaration: `class NamedOwner{static class Anchor{}static java.util.function.IntUnaryOperator read(int l0){return value->l0+value;}}`, check: `if(NamedOwner.read(7).applyAsInt(3)!=10)throw new AssertionError("entry versus arrow identity");`, parameterTypes: `int.class`, names: []string{"l0"}, parameters: true},
		{name: "reserved suffix", declaration: `class NamedOwner{static class Anchor{}static java.util.function.IntUnaryOperator read(int l0,int jdec$local$l0){return value->l0+jdec$local$l0+value;}}`, check: `if(NamedOwner.read(7,2).applyAsInt(3)!=12)throw new AssertionError("three distinct declarations");`, parameterTypes: `int.class,int.class`, names: []string{"l0", "jdec$local$l0"}, parameters: true},
		{name: "lambda local", declaration: `class NamedOwner{static class Anchor{}static java.util.function.IntUnaryOperator read(int var1){return value->{int sum=value*2;return var1+sum+sum;};}}`, check: `if(NamedOwner.read(7).applyAsInt(3)!=19)throw new AssertionError("entry versus lambda local identity");`, parameterTypes: `int.class`, names: []string{"var1"}, parameters: true},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							methodParameterNameRoundTrip(t, fixture, debug)
						})
					}
				})
			}
		})
	}
}

func TestAdversarialMethodParameterNamesPre52MetadataKept(t *testing.T) {
	for _, major := range []uint16{49, 50, 51} {
		t.Run(fmt.Sprint(major), func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							methodParameterNameRoundTrip(t, methodParameterNameCase{
								name: "pre52 original names and flags", declaration: `class NamedOwner{static int read(final int originalNamed){return originalNamed;}}`,
								check:          `if(NamedOwner.read(7)!=7||NamedOwner.class.getDeclaredMethod("read",int.class).getParameters()[0].getModifiers()!=16)throw new AssertionError("older-format original entry metadata");`,
								parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true,
								// This authored version protocol is first run on the original
								// JVM. Its logical table, code and flag words stay intact.
								mutate: func(t *testing.T, object *ClassObject) { object.MajorVersion = major },
							}, debug)
						})
					}
				})
			}
		})
	}
}
