package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeRootBridgeLambdaRequiresArchiveAndConsumedSource(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootBridgeHandleSource("lambda"))
	for _, variant := range []string{"original", "foreign reference", "foreign handle", "missing consumption", "missing body", "copied method body", "stub", "checked escape", "foreign dumper"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["BridgeHandleRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if strings.HasPrefix(n, "lambda$") {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original lambda implementation")
			}
			name, _ := sourceBridgeUTF8(root, method.NameIndex)
			desc, _ := sourceBridgeUTF8(root, method.DescriptorIndex)
			input := map[string][]byte{}
			for n, raw := range files {
				input[n] = append([]byte(nil), raw...)
			}
			if variant == "foreign reference" || variant == "foreign handle" {
				foreign, err := Parse(input["BridgeHandleDriver.class"])
				if err != nil {
					t.Fatal(err)
				}
				owner := nativeMemberLambdaTestUTF8(foreign, root.GetClassName())
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantClassInfo{NameIndex: owner})
				ci := uint16(len(foreign.ConstantPool))
				ni := nativeMemberLambdaTestUTF8(foreign, name)
				di := nativeMemberLambdaTestUTF8(foreign, desc)
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: ni, DescriptorIndex: di})
				nt := uint16(len(foreign.ConstantPool))
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: ci, NameAndTypeIndex: nt}})
				if variant == "foreign handle" {
					foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: uint16(len(foreign.ConstantPool))})
				}
				input["BridgeHandleDriver.class"] = foreign.Bytes()
			}
			z := nativeArchive(t, input)
			defer z.Close()
			index := z.originalMemberIndex()
			d := z.nativeMemberReader(root)
			source := &nativeMemberClass{object: root, lambdaContext: nativeLambdaImplementationContext{resolve: d.nativeAnnotationDeclarationResolver(), metadata: d.buildInvocationMetadata()}}
			binding := nativeRootBridgeMethodHandlesClosed(root, index, source, nil)
			if variant == "foreign handle" {
				if binding {
					t.Fatal("foreign private lookup admitted")
				}
				return
			}
			if !binding || len(source.lambdaImplementations) != 1 {
				t.Fatal("original exclusive implementation proof")
			}
			archive := z.nativeMemberLambdaArchiveClosed(source, index, nil)
			if variant == "foreign reference" {
				if archive {
					t.Fatal("foreign physical lambda reference consumed")
				}
				return
			}
			if !archive {
				t.Fatal("original archive closure")
			}
			body := &dumpedMethods{member: method, code: "return 3;", bodyCode: "return 3;"}
			key := registerMethodSourceBodyFixture(t, d, name, desc, 0, body)
			d.lambdaMethods[name] = []string{desc}
			switch variant {
			case "missing consumption":
				delete(d.lambdaMethods, name)
			case "missing body":
				delete(d.dumpedMethodsSet, key)
			case "copied method body":
				copy := *method
				body.member = &copy
			case "stub":
				body.bodyCode = "stub"
			case "checked escape":
				body.checkedEscape = true
			case "foreign dumper":
				d.obj = &ClassObject{}
			}
			if got := nativeMemberLambdaSourceClosed(source, d, nil); got != (variant == "original") {
				t.Fatalf("source closed=%v", got)
			}
		})
	}
}

const nativeRootBridgeHandlesFixture = `class BridgeHandleRoot {
 final long value;private BridgeHandleRoot(long value){this.value=value;}
 public static Object echo(Object value){return value;}
 static java.util.function.Function<Object,Object> reference(){return BridgeHandleRoot::echo;}
 static java.util.function.IntUnaryOperator lambda(){return value->value+3;}
 static class Factory{static BridgeHandleRoot make(long value){return new BridgeHandleRoot(value);}}
}
class BridgeHandleDriver {public static void main(String[]args){
 Object token=new Object();for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  BridgeHandleRoot root=BridgeHandleRoot.Factory.make(value);
  if(root.value!=value||CALLS)throw new AssertionError("bridge/handle value and identity");
 }System.out.println("root-bridge:unrelated-handles:verified");}}
`

func nativeRootBridgeHandleSource(variant string) string {
	source := strings.Replace(nativeRootBridgeHandlesFixture, "CALLS", `BridgeHandleRoot.reference().apply(token)!=token||BridgeHandleRoot.reference().apply(null)!=null||BridgeHandleRoot.lambda().applyAsInt(Integer.MAX_VALUE)!=Integer.MIN_VALUE+2`, 1)
	switch variant {
	case "reference":
		source = strings.Replace(source, "static java.util.function.IntUnaryOperator lambda(){return value->value+3;}", "", 1)
		source = strings.Replace(source, "||BridgeHandleRoot.lambda().applyAsInt(Integer.MAX_VALUE)!=Integer.MIN_VALUE+2", "", 1)
	case "lambda":
		source = strings.Replace(source, "static java.util.function.Function<Object,Object> reference(){return BridgeHandleRoot::echo;}", "", 1)
		source = strings.Replace(source, "BridgeHandleRoot.reference().apply(token)!=token||BridgeHandleRoot.reference().apply(null)!=null||", "", 1)
	case "renamed wide receiver":
		source = strings.Replace(source, "static java.util.function.IntUnaryOperator lambda(){return value->value+3;}", "java.util.function.LongSupplier delayed(long offset){return ()->value+offset;}", 1)
		source = strings.Replace(source, "BridgeHandleRoot.lambda().applyAsInt(Integer.MAX_VALUE)!=Integer.MIN_VALUE+2", "root.delayed(3L).getAsLong()!=value+3L", 1)
		source = strings.NewReplacer("echo", "relay", "reference", "function", "lambda", "closure").Replace(source)
	}
	return source
}

// The JVM runs the authored original first. Rebuilt source must preserve the
// observable values, null/object identity, overflow, ABI and generic Signature
// under both compiler profiles, both debug modes and all cleanup policies.
func TestNativeRootBridgeUnrelatedHandlesRoundTrip(t *testing.T) {
	for _, variant := range []string{"reference", "lambda", "both", "renamed wide receiver"} {
		t.Run(variant, func(t *testing.T) {
			source := nativeRootBridgeHandleSource(variant)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{"BridgeHandleRoot"}, "BridgeHandleDriver", "root-bridge:unrelated-handles:verified\n", nativeLexicalExactSignatures)
			})
			t.Run("native8", func(t *testing.T) {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"BridgeHandleRoot.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"BridgeHandleRoot"}, "BridgeHandleDriver", "root-bridge:unrelated-handles:verified\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}

func TestNativeRootBridgeMethodHandleBindingsAreDisjointAndOriginal(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootBridgeHandleSource("reference"))
	for _, variant := range []string{"original", "repeated", "constructor", "constructor name", "initializer", "field tag", "special", "interface", "static mismatch", "unknown target", "generic", "private", "duplicate declaration", "missing targets", "invalid index", "foreign source object", "nil source", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["BridgeHandleRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			original := z.originalMemberIndex()
			owner := root.GetClassName()
			targets := append([]nativeMemberHandleTarget(nil), original.handleTargets[owner]...)
			if !original.valid || len(targets) != 1 {
				t.Fatal("original method-handle index")
			}
			index := &nativeMemberIndex{valid: true, handles: map[string]bool{owner: true}, handleTargets: map[string][]nativeMemberHandleTarget{owner: targets}}
			source := &nativeMemberClass{object: root}
			var method *MemberInfo
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if n == "echo" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original target")
			}
			var work *workbudget.Budget
			switch variant {
			case "repeated":
				index.handleTargets[owner] = append(targets, targets[0])
			case "constructor":
				targets[0].kind = 8
			case "constructor name":
				targets[0].name = "<init>"
			case "initializer":
				targets[0].name = "<clinit>"
			case "field tag":
				targets[0].methodRef = false
			case "special":
				targets[0].kind = 7
			case "interface":
				targets[0].kind = 9
				targets[0].methodRef = false
			case "static mismatch":
				targets[0].kind = 5
			case "unknown target":
				targets[0].descriptor = "()Ljava/lang/Object;"
			case "generic":
				method.Attributes = append(method.Attributes, &SignatureAttribute{})
			case "private":
				method.AccessFlags = 0x000a
			case "duplicate declaration":
				root.Methods = append(root.Methods, method)
			case "missing targets":
				delete(index.handleTargets, owner)
			case "invalid index":
				index.valid = false
			case "foreign source object":
				source.object = &ClassObject{}
			case "nil source":
				source = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeRootBridgeMethodHandlesClosed(root, index, source, work); got != (variant == "original" || variant == "repeated") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

// An unused constructor handle is still an original symbolic observer. Append
// it without modifying executable bytecode; the JVM original remains valid,
// while the family must refuse to rewrite either private or bridge descriptor.
func TestNativeRootBridgeHandlesRejectUnusedConstructorObservers(t *testing.T) {
	fixture := nativeRootBridgeHandleSource("both")
	_, java := t04Tools(t)
	for _, bridge := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "access bridge"}[bridge], func(t *testing.T) {
			files := nativeCompileClasses(t, fixture)
			root, err := Parse(files["BridgeHandleRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			var descriptor string
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if n == "<init>" && (m.AccessFlags&0x1000 != 0) == bridge {
					descriptor, _ = sourceBridgeUTF8(root, m.DescriptorIndex)
				}
			}
			if descriptor == "" {
				t.Fatal("original constructor")
			}
			ni := nativeMemberLambdaTestUTF8(root, "<init>")
			di := nativeMemberLambdaTestUTF8(root, descriptor)
			root.ConstantPool = append(root.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: ni, DescriptorIndex: di})
			nt := uint16(len(root.ConstantPool))
			root.ConstantPool = append(root.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: root.ThisClass, NameAndTypeIndex: nt}})
			root.ConstantPool = append(root.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(len(root.ConstantPool))})
			files["BridgeHandleRoot.class"] = root.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "BridgeHandleDriver"); got != "root-bridge:unrelated-handles:verified\n" {
				t.Fatalf("original JVM=%q", got)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			if entry := z.nativeMemberEntry(root); entry != nil && entry.family != nil {
				t.Fatal("constructor observer erased")
			}
		})
	}
}
