package javaclassparser

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/internal/workbudget"
)

const sharedConstructorNameTypeFixture = `
class SharedCtorOwner {
 interface Cursor {Object value();}
 static abstract class View {private View(){SharedCtorLedger.calls++;if(SharedCtorLedger.fail)throw SharedCtorLedger.failure;}abstract Object state();abstract Cursor cursor();Cursor alternate(){return cursor();}}
 static View make(final Object token){return new View(){Object state(){return token;}Cursor cursor(){return new Cursor(){public Object value(){return state();}};}};}
}
class SharedCtorLedger {static int calls;static boolean fail;static final RuntimeException failure=new RuntimeException("original identity");}
class SharedCtorDriver {public static void main(String[]args)throws Exception{int rows=0;for(Object token:new Object[]{null,new Object(),new String("identity"),new String[]{"array"}})for(boolean fail:new boolean[]{false,true}){SharedCtorLedger.calls=0;SharedCtorLedger.fail=fail;try{SharedCtorOwner.View view=SharedCtorOwner.make(token);if(fail||SharedCtorLedger.calls!=1||view.state()!=token)throw new AssertionError("private constructor identity/order");SharedCtorOwner.Cursor first=view.cursor(),second=view.cursor(),alternate=view.alternate();if(first==second||first.value()!=token||second.value()!=token||alternate.value()!=token)throw new AssertionError("lexical receiver/allocation identity");if(!view.getClass().isAnonymousClass()||!first.getClass().isAnonymousClass()||!first.getClass().getEnclosingMethod().getName().equals("cursor"))throw new AssertionError("original scope");}catch(RuntimeException failure){if(!fail||failure!=SharedCtorLedger.failure||SharedCtorLedger.calls!=1)throw new AssertionError("constructor failure identity/order",failure);}rows++;}System.out.println(rows+":shared:constructor:lexical:identity:scope");}}
`

func sharedConstructorNameTypeShape(shape string) (string, string) {
	owner, source := "SharedCtorOwner", sharedConstructorNameTypeFixture
	if shape == "three-edges" {
		source = strings.Replace(source, "Object state(){return token;}", "Cursor alternate(){return new Cursor(){public Object value(){return state();}};}Object state(){return token;}", 1)
	}
	if shape == "renamed" {
		owner = "SharedPacketScope"
		source = strings.ReplaceAll(source, "SharedCtorOwner", owner)
		source = strings.ReplaceAll(source, "state()", "snapshot()")
	}
	return owner, source
}

func TestAdversarialSharedConstructorNameTypeUsesEveryOriginalOwner(t *testing.T) {
	for _, shape := range []string{"two-edges", "three-edges", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, source := sharedConstructorNameTypeShape(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "SharedCtorDriver", "8:shared:constructor:lexical:identity:scope\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialSharedConstructorNameTypeOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"two-edges", "three-edges", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, source := sharedConstructorNameTypeShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, javac, []string{owner}, "SharedCtorDriver", "8:shared:constructor:lexical:identity:scope\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialSharedConstructorNameTypeRejectsUnprovedOriginalEdges(t *testing.T) {
	files := nativeCompileClasses(t, sharedConstructorNameTypeFixture)
	for _, variant := range []string{"original", "missing family", "failed family", "foreign root", "foreign caller", "missing group", "foreign group", "failed group", "missing child", "foreign child", "wrong enclosing owner", "wrong descriptor", "missing bridge", "foreign bridge object", "changed bridge body", "ordinary bridge", "missing marker", "nil marker object", "foreign marker object", "wrong marker ordinal", "foreign owner edge", "field edge", "interface edge", "bridge handle", "anonymous handle", "dynamic tuple", "invokedynamic tuple", "orphan tuple", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["SharedCtorOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete mixed forest")
			}
			forest := p.anonymousForest
			caller := forest.objects["SharedCtorOwner$1"]
			child := forest.units["SharedCtorOwner$1$1"]
			if caller == nil || child == nil {
				t.Fatal("original nested anonymous units")
			}
			index, bridgeRef, anonymousRef := 0, 0, 0
			for i, constant := range caller.ConstantPool {
				ref, ok := constant.(*ConstantMethodrefInfo)
				if !ok || ref == nil {
					continue
				}
				owner, known := sourceBridgeClassName(caller, ref.ClassIndex)
				if !known || owner != "SharedCtorOwner$View" && owner != child.object.GetClassName() {
					continue
				}
				nt, ok := caller.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok {
					continue
				}
				name, _ := sourceBridgeUTF8(caller, nt.NameIndex)
				if name != "<init>" {
					continue
				}
				if index != 0 && index != int(ref.NameAndTypeIndex) {
					t.Fatal("fixture did not share original NameAndType")
				}
				index = int(ref.NameAndTypeIndex)
				if owner == "SharedCtorOwner$View" {
					bridgeRef = i + 1
				} else {
					anonymousRef = i + 1
				}
			}
			if index == 0 || bridgeRef == 0 || anonymousRef == 0 {
				t.Fatal("original two-owner constructor tuple")
			}
			bridgeObject := forest.objects["SharedCtorOwner$View"]
			bridge := p.constructorBridges("SharedCtorOwner$View")[child.descriptor]
			if bridge == nil {
				t.Fatal("original private constructor certificate")
			}
			var work *workbudget.Budget
			checkIndex := index
			switch variant {
			case "missing family":
				forest.members = nil
			case "failed family":
				p.failed = true
			case "foreign root":
				forest.root = "Foreign"
			case "foreign caller":
				copy := *caller
				caller = &copy
			case "missing group":
				delete(forest.groups, caller.GetClassName())
			case "foreign group":
				copy := *forest
				forest.groups[caller.GetClassName()].forest = &copy
			case "failed group":
				forest.groups[caller.GetClassName()].failed = true
			case "missing child":
				delete(forest.units, child.object.GetClassName())
			case "foreign child":
				copy := *child.object
				forest.objects[child.object.GetClassName()] = &copy
			case "wrong enclosing owner":
				cp := NewConstantPoolWithConstant(&child.object.ConstantPool)
				for _, a := range child.object.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("Foreign")))
					}
				}
			case "wrong descriptor":
				child.descriptor = "()V"
			case "missing bridge":
				p.children["SharedCtorOwner$View"].accessBridges = nil
			case "foreign bridge object":
				copy := *bridgeObject
				forest.objects[bridgeObject.GetClassName()] = &copy
			case "changed bridge body":
				for _, a := range bridge.method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[0] = 0x01
					}
				}
			case "ordinary bridge":
				bridge.method.AccessFlags = 0
			case "missing marker":
				delete(forest.units, bridge.marker)
			case "nil marker object":
				forest.units[bridge.marker].object = nil
			case "foreign marker object":
				copy := *forest.units[bridge.marker].object
				forest.objects[bridge.marker] = &copy
			case "wrong marker ordinal":
				forest.units[bridge.marker].ordinal = 2
			case "foreign owner edge":
				copy := *caller.ConstantPool[bridgeRef-1].(*ConstantMethodrefInfo)
				copy.ClassIndex = caller.ThisClass
				caller.ConstantPool = append(caller.ConstantPool, &copy)
			case "field edge":
				ref := caller.ConstantPool[bridgeRef-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[bridgeRef-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "interface edge":
				ref := caller.ConstantPool[anonymousRef-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[anonymousRef-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "bridge handle", "anonymous handle":
				ref := bridgeRef
				if variant == "anonymous handle" {
					ref = anonymousRef
				}
				caller.ConstantPool = append(caller.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(ref)})
			case "dynamic tuple":
				caller.ConstantPool = append(caller.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "invokedynamic tuple":
				caller.ConstantPool = append(caller.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "orphan tuple":
				copy := *caller.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				caller.ConstantPool = append(caller.ConstantPool, &copy)
				checkIndex = len(caller.ConstantPool)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := caller.Bytes()
			if got := nativeAnonymousForestSharedConstructorNameType(caller, checkIndex, forest, work); got != (variant == "original") {
				t.Fatalf("original per-edge coverage %v", got)
			}
			if string(before) != string(caller.Bytes()) || z.sourceOwnership.bytes != 0 {
				t.Fatal("symbol proof modified original pool or published source")
			}
		})
	}
}

func TestAdversarialSharedConstructorNameTypeOracleRejectsSemanticMutants(t *testing.T) {
	_, java := t04Tools(t)
	original := nativeCompileSourceReleaseClasses(t, map[string]string{"SharedCtorOwner.java": sharedConstructorNameTypeFixture}, "none", "8")
	for _, mutation := range []struct{ name, before, after string }{
		{"original", "", ""},
		{"lost-capture", "Object state(){return token;}", "Object state(){return null;}"},
		{"lost-enclosing-value", "public Object value(){return state();}", "public Object value(){return null;}"},
		{"changed-private-effect", "SharedCtorLedger.calls++;", "SharedCtorLedger.calls+=2;"},
		{"changed-failure-identity", "throw SharedCtorLedger.failure;", "throw new RuntimeException();"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.name != "original" {
				if strings.Count(sharedConstructorNameTypeFixture, mutation.before) != 1 {
					t.Fatal("ambiguous semantic mutation")
				}
				files = nativeCompileSourceReleaseClasses(t, map[string]string{"SharedCtorOwner.java": strings.Replace(sharedConstructorNameTypeFixture, mutation.before, mutation.after, 1)}, "none", "8")
			}
			out := t.TempDir()
			for name, raw := range original {
				if name == "SharedCtorOwner.class" || strings.HasPrefix(name, "SharedCtorOwner$") {
					var ok bool
					raw, ok = files[name]
					if !ok {
						t.Fatal("mutant lost original owned class", name)
					}
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				output, err := exec.CommandContext(ctx, java, "-Xverify:all", "-Xmx128m", assertions, "-cp", out, "SharedCtorDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutation.name == "original" {
					if interrupted || err != nil || string(output) != "8:shared:constructor:lexical:identity:scope\n" {
						t.Fatalf("original %s: %v %s", assertions, err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError:") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "NoClassDefFoundError") {
					t.Fatalf("compiled mutant escaped original semantic oracle: %v %s", err, output)
				}
			}
		})
	}
}
