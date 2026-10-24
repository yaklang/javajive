package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type methodParameterNameCase struct {
	name, declaration, check, parameterTypes string
	names                                    []string
	parameters                               bool
	refuseSource                             bool
	mutate                                   func(*testing.T, *ClassObject)
}

// Each driver is compiled once with the original class and retained unchanged.
// The original JVM oracle runs before asking for source. MethodParameters is
// compared independently, including table presence, names and every flag word.
func TestAdversarialAuthoredMethodParameterNamesRoundTrip(t *testing.T) {
	cases := []methodParameterNameCase{
		{name: "static", declaration: `class NamedOwner{static class Anchor{}static int read(int originalNamed){return originalNamed;}}`, check: `if(NamedOwner.read(7)!=7)throw new AssertionError("entry value");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true},
		{name: "instance-wide", declaration: `class NamedOwner{static class Anchor{}long read(final long count,double ratio,Object marker){return count+(long)ratio+(marker==null?0:1);}}`, check: `if(new NamedOwner().read(9L,2D,new Object())!=12L)throw new AssertionError("wide entry identity");`, parameterTypes: `long.class,double.class,Object.class`, names: []string{"count", "ratio", "marker"}, parameters: true},
		{name: "generated-local-collision", declaration: `class NamedOwner{static class Anchor{}static int calls;static int next(int value){calls++;return value;}static int read(int var3,int var0,int var1){int sum=next(var3+var0+var1);return sum+sum;}}`, check: `if(NamedOwner.read(1,2,3)!=12||NamedOwner.calls!=1)throw new AssertionError("distinct declarations and once-only effect");`, parameterTypes: `int.class,int.class,int.class`, names: []string{"var3", "var0", "var1"}, parameters: true},
		{name: "field-shadow", declaration: `class NamedOwner{static class Anchor{}static int value=11;static int read(int value){return NamedOwner.value+value;}}`, check: `if(NamedOwner.read(7)!=18)throw new AssertionError("field identity");`, parameterTypes: `int.class`, names: []string{"value"}, parameters: true},
		{name: "type-package-shadow", declaration: `class NamedOwner{static class Anchor{}static long read(int Long,int java){return Long+java+((java.lang.Long)null).valueOf(8L).longValue();}}`, check: `if(NamedOwner.read(2,3)!=13L)throw new AssertionError("static owner namespace");`, parameterTypes: `int.class,int.class`, names: []string{"Long", "java"}, parameters: true},
		{name: "generic-shadow", declaration: `class NamedOwner<T>{static class Anchor{}<T>T read(T T){return T;}}`, check: `String token=new String("same");if(new NamedOwner<Integer>().read(token)!=token)throw new AssertionError("generic value identity");java.lang.reflect.TypeVariable<?> v=NamedOwner.class.getDeclaredMethod("read",Object.class).getTypeParameters()[0];if(!v.getName().equals("T"))throw new AssertionError("generic declaration");`, parameterTypes: `Object.class`, names: []string{"T"}, parameters: true},
		{name: "abstract", declaration: `abstract class NamedOwner{static class Anchor{}abstract int read(final long chosen,int unused,final Object token);}`, parameterTypes: `long.class,int.class,Object.class`, names: []string{"chosen", "unused", "token"}, parameters: true},
		{name: "annotations", declaration: `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @java.lang.annotation.Target({java.lang.annotation.ElementType.PARAMETER,java.lang.annotation.ElementType.TYPE_USE}) @interface NamedMark{}class NamedOwner{static class Anchor{}long read(final long amount,@NamedMark int value){return amount+value;}}`, check: `if(new NamedOwner().read(9L,2)!=11L)throw new AssertionError("annotated value");java.lang.reflect.Parameter marked=NamedOwner.class.getDeclaredMethod("read",long.class,int.class).getParameters()[1];if(marked.getAnnotations().length!=1||marked.getAnnotatedType().getAnnotations().length!=1)throw new AssertionError("annotation positions");`, parameterTypes: `long.class,int.class`, names: []string{"amount", "value"}, parameters: true},
		{name: "varargs", declaration: `class NamedOwner{static class Anchor{}static int read(String prefix,int... numbers){return prefix.length()+numbers.length;}}`, check: `if(NamedOwner.read("ab",1,2,3)!=5)throw new AssertionError("varargs shape");`, parameterTypes: `String.class,int[].class`, names: []string{"prefix", "numbers"}, parameters: true},
		{name: "final-slot-reuse", declaration: `class NamedOwner{static class Anchor{}static int read(final int originalNamed){int later=7;return later;}}`, check: `for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(NamedOwner.read(n)!=7)throw new AssertionError("later slot definition");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true, mutate: func(t *testing.T, obj *ClassObject) {
			// Audited, same-length replacement: the final entry is unused and
			// a later int definition occupies its JVM slot. Both are JVM-valid.
			finalParameterReadCode(t, obj).Code = []byte{0x10, 7, 0x3b, 0x1a, 0xac}
		}},
		{name: "final-iinc", declaration: `class NamedOwner{static class Anchor{}static int read(int originalNamed){originalNamed++;return originalNamed;}}`, check: `for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})if(NamedOwner.read(n)!=n+1)throw new AssertionError("entry versus mutable storage");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true, mutate: func(t *testing.T, obj *ClassObject) {
			finalParameterMarkReadFinal(t, obj, 0)
		}},
		{name: "instance-field-shadow", declaration: `class NamedOwner{static class Anchor{}int value=11;int read(int value){return this.value+value;}}`, check: `if(new NamedOwner().read(7)!=18)throw new AssertionError("instance field identity");`, parameterTypes: `int.class`, names: []string{"value"}, parameters: true},
		{name: "lambda-capture", declaration: `class NamedOwner{static class Anchor{}static java.util.function.IntSupplier read(int originalNamed){return ()->originalNamed;}}`, check: `if(NamedOwner.read(7).getAsInt()!=7)throw new AssertionError("lambda capture identity");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true},
		{name: "lvt-name-collision", declaration: `class NamedOwner{static class Anchor{}static int calls;static int next(int value){calls++;return value;}static int read(int var3,int var0,int var1){int sum=next(var3+var0+var1);return sum+sum;}}`, check: `if(NamedOwner.read(1,2,3)!=12||NamedOwner.calls!=1)throw new AssertionError("LVT is not entry identity");`, parameterTypes: `int.class,int.class,int.class`, names: []string{"var3", "var0", "var1"}, parameters: true, mutate: func(t *testing.T, obj *ClassObject) {
			// This authored debug-protocol variant gives distinct local slots
			// one LVT spelling. The MethodParameters table and all Code bytes
			// remain original; that logical table still defines entry names.
			index := sourceBridgePoolString(t, obj, "var3")
			for _, a := range finalParameterReadCode(t, obj).Attributes {
				if table, ok := a.(*UnparsedAttribute); ok && table.Name == "LocalVariableTable" {
					if len(table.Info) < 2 || len(table.Info) != 2+10*int(binary.BigEndian.Uint16(table.Info)) {
						t.Fatal("authored local-variable table")
					}
					for offset := 2; offset < len(table.Info); offset += 10 {
						binary.BigEndian.PutUint16(table.Info[offset+4:offset+6], index)
					}
				}
			}
		}},
		{name: "absent-table", declaration: `class NamedOwner{static class Anchor{}static int read(int originalNamed){return originalNamed;}}`, check: `if(NamedOwner.read(7)!=7)throw new AssertionError("entry value");`, parameterTypes: `int.class`, names: []string{"arg0"}},
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

func methodParameterNameRoundTrip(t *testing.T, fixture methodParameterNameCase, debug string) {
	t.Helper()
	javac, java := t04Tools(t)
	wantedNames := make([]string, len(fixture.names))
	for i, name := range fixture.names {
		wantedNames[i] = fmt.Sprintf("%q", name)
	}
	driver := `class NamedDriver{public static void main(String[]args)throws Exception{` + fixture.check + `java.lang.reflect.Parameter[]p=NamedOwner.class.getDeclaredMethod("read",` + fixture.parameterTypes + `).getParameters();String[]names={` + strings.Join(wantedNames, ",") + `};if(p.length!=names.length)throw new AssertionError("parameter count");for(int i=0;i<p.length;i++)if(!p[i].getName().equals(names[i])||p[i].isNamePresent()!=` + fmt.Sprint(fixture.parameters) + `)throw new AssertionError("MethodParameters["+i+"]="+p[i].getName()+" present="+p[i].isNamePresent());System.out.println("names:original");}}`
	original := methodParameterNameDirectory(t, "original")
	input := filepath.Join(original, "Authored.java")
	write := func(path string, raw []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(input, []byte(fixture.declaration+driver))
	compileArgs := []string{"-proc:none", "--release", "8", "-g:" + debug, "-d", original}
	if fixture.parameters {
		compileArgs = append(compileArgs, "-parameters")
	}
	if log, err := exec.Command(javac, append(compileArgs, input)...).CombinedOutput(); err != nil {
		t.Fatalf("original compile: %v\n%s", err, log)
	}
	files := map[string][]byte{}
	if err := filepath.Walk(original, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".class") {
			return err
		}
		name, err := filepath.Rel(original, path)
		if err == nil {
			files[filepath.ToSlash(name)], err = os.ReadFile(path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if fixture.mutate != nil {
		obj, err := Parse(files["NamedOwner.class"])
		if err != nil {
			t.Fatal(err)
		}
		fixture.mutate(t, obj)
		files["NamedOwner.class"] = obj.Bytes()
		write(filepath.Join(original, "NamedOwner.class"), files["NamedOwner.class"])
	}
	run := func(dir, stage string) {
		t.Helper()
		got, err := exec.Command(java, "-Xverify:all", "-cp", dir, "NamedDriver").CombinedOutput()
		t.Logf("%s JVM: %q", stage, got)
		write(filepath.Join(dir, "jvm-oracle.txt"), got)
		if err != nil || string(got) != "names:original\n" {
			t.Fatalf("%s JVM: %v\n%s", stage, err, got)
		}
	}
	run(original, "original")
	z := nativeArchive(t, files)
	defer z.Close()
	if fixture.refuseSource {
		text, err := z.ReadFile("NamedOwner.class")
		t.Logf("conservative source refusal: %v\n%s", err, text)
		if err == nil && !strings.Contains(string(text), DecompileStubMarker) && !strings.Contains(string(text), "// decompile dump failed") {
			t.Fatalf("unproved ordinary body was accepted as source\n%s", text)
		}
		object, parseErr := Parse(files["NamedOwner.class"])
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if entry := z.nativeMemberEntry(object); entry != nil && entry.family != nil {
			t.Fatal("unproved source published a complete family")
		}
		return
	}
	rebuilt := methodParameterNameDirectory(t, "rebuilt")
	var paths []string
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := files[name]
		if name == "NamedOwner.class" || strings.HasPrefix(name, "NamedOwner$") {
			text, err := z.ReadFile(name)
			if err != nil || strings.Contains(string(text), DecompileStubMarker) || strings.Contains(string(text), "// decompile dump failed") {
				t.Fatalf("source %s: %v\n%s", name, err, text)
			}
			name = strings.TrimSuffix(name, ".class") + ".java"
			raw = text
			paths = append(paths, filepath.Join(rebuilt, name))
			t.Logf("source %s:\n%s", name, raw)
		}
		write(filepath.Join(rebuilt, name), raw)
	}
	recompile := []string{"-proc:none", "--release", "8", "-g:" + debug, "-cp", rebuilt, "-d", rebuilt}
	if fixture.parameters {
		recompile = append(recompile, "-parameters")
	}
	if log, err := exec.Command(javac, append(recompile, paths...)...).CombinedOutput(); err != nil {
		t.Fatalf("rebuilt compile: %v\n%s", err, log)
	}
	run(rebuilt, "rebuilt")
	for _, name := range names {
		if name != "NamedOwner.class" && !strings.HasPrefix(name, "NamedOwner$") {
			continue
		}
		next, err := os.ReadFile(filepath.Join(rebuilt, name))
		if err != nil {
			t.Fatal(err)
		}
		if before, after := methodParameterNamesIndependent(t, files[name]), methodParameterNamesIndependent(t, next); !reflect.DeepEqual(before, after) {
			t.Fatalf("original MethodParameters changed %s: %#v -> %#v", name, before, after)
		}
		if before, after := nativeBinaryShape(t, files[name]), nativeBinaryShape(t, next); !reflect.DeepEqual(before, after) {
			t.Fatalf("original binary shape changed %s: %#v -> %#v", name, before, after)
		}
		if before, after := methodParameterNamesAllMethodDeclarations(t, files[name]), methodParameterNamesAllMethodDeclarations(t, next); !reflect.DeepEqual(before, after) {
			t.Fatalf("complete original method declarations changed %s: %#v -> %#v", name, before, after)
		}
		nativeLexicalExactSignatures(t, name, files[name], next)
	}
}

type independentMethodParameterDeclaration struct {
	Flags     uint16
	Signature string
}

// Include compiler-private helpers as well as public ABI: an ordinary target
// must not vanish after an earlier lambda request, and regeneration must keep
// the original method set, access/varargs flags and generic signatures.
func methodParameterNamesAllMethodDeclarations(t *testing.T, raw []byte) map[string]independentMethodParameterDeclaration {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	declarations := map[string]independentMethodParameterDeclaration{}
	for _, method := range object.Methods {
		name, named := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, typed := sourceBridgeUTF8(object, method.DescriptorIndex)
		if !named || !typed {
			t.Fatal("original method declaration identity")
		}
		row := independentMethodParameterDeclaration{Flags: method.AccessFlags}
		for _, attribute := range method.Attributes {
			if signature, ok := attribute.(*SignatureAttribute); ok {
				row.Signature, typed = sourceBridgeUTF8(object, signature.SignatureIndex)
				if !typed {
					t.Fatal("original method Signature")
				}
			}
		}
		key := name + descriptor
		if _, duplicate := declarations[key]; duplicate {
			t.Fatal("duplicate original method declaration")
		}
		declarations[key] = row
	}
	return declarations
}

func methodParameterNameDirectory(t *testing.T, stage string) string {
	t.Helper()
	if evidence := os.Getenv("JDEC_PARAMETER_NAMES_EVIDENCE"); evidence != "" {
		path := filepath.Join(evidence, filepath.FromSlash(t.Name()), stage)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	return t.TempDir()
}

type independentMethodParameterName struct {
	Name    string
	Present bool
	Flags   uint16
}

func methodParameterNamesIndependent(t *testing.T, raw []byte) map[string][]independentMethodParameterName {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]independentMethodParameterName{}
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		descriptor, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		for _, a := range method.Attributes {
			table, ok := a.(*UnparsedAttribute)
			if !ok || table.Name != "MethodParameters" {
				continue
			}
			key := name + descriptor
			if _, duplicate := out[key]; duplicate || len(table.Info) < 1 || len(table.Info) != 1+4*int(table.Info[0]) {
				t.Fatal("invalid independent MethodParameters table")
			}
			parameters := make([]independentMethodParameterName, int(table.Info[0]))
			for i := range parameters {
				entry := table.Info[1+4*i : 5+4*i]
				index := binary.BigEndian.Uint16(entry[:2])
				parameters[i].Present = index != 0
				parameters[i].Flags = binary.BigEndian.Uint16(entry[2:])
				if index != 0 {
					var known bool
					parameters[i].Name, known = sourceBridgeUTF8(obj, index)
					if !known {
						t.Fatal("independent parameter name CP kind")
					}
				}
			}
			out[key] = parameters
		}
	}
	return out
}
