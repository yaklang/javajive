package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeConstructorLocalOwnedFixture = `
interface CtorScopeRead{long read();Object owner();Object token();}
class CtorScopeBase{static String trace="";static final RuntimeException failure=new IllegalArgumentException("original");CtorScopeBase(int n){trace+="B";if(n==991)throw failure;}}
class CtorScopeRoot extends CtorScopeBase{
 long mutable;CtorScopeRead view;
 CtorScopeRoot(final Object captured,final long wide,final int delta){
  super(delta);mutable=17;
  class View implements CtorScopeRead{public long read(){return mutable+wide+delta;}public Object owner(){return CtorScopeRoot.this;}public Object token(){return captured;}}
  view=new View();CtorScopeBase.trace+="C";
 }
}
class CtorScopeDriver{public static void main(String[]args)throws Exception{
 int rows=0;
 for(long wide:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object captured:new Object[]{null,new Object()}){
 CtorScopeBase.trace="";CtorScopeRoot root=new CtorScopeRoot(captured,wide,delta);if(!CtorScopeBase.trace.equals("BC")||root.view.owner()!=root||root.view.token()!=captured)throw new AssertionError("constructor scope or original capture");
 for(long state:new long[]{Long.MIN_VALUE,-7,Long.MAX_VALUE}){root.mutable=state;if(root.view.read()!=state+wide+delta)throw new AssertionError("live field/captured words");}
 Class<?>local=root.view.getClass();if(!local.isLocalClass()||local.getEnclosingConstructor().getDeclaringClass()!=CtorScopeRoot.class||local.getEnclosingConstructor().getParameterTypes().length!=3)throw new AssertionError("original constructor-local lexical binding");rows++;
 }
 CtorScopeBase.trace="";try{new CtorScopeRoot(new Object(),3,991);throw new AssertionError("missing original failure");}catch(RuntimeException e){if(e!=CtorScopeBase.failure||!CtorScopeBase.trace.equals("B"))throw new AssertionError("pre-local failure order",e);}
 System.out.println(rows+":ctor:local:live:wide:scope:failure");
}}
`

func TestAdversarialConstructorLocalRetainsOriginalInitializedScope(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeConstructorLocalOwnedFixture, []string{"CtorScopeRoot"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialMemberConstructorLocalRetainsPhysicalEnclosingDescriptor(t *testing.T) {
	source := strings.Replace(nativeConstructorLocalOwnedFixture, "class CtorScopeRoot extends CtorScopeBase{", "class CtorScopeContainer{class CtorScopeRoot extends CtorScopeBase{", 1)
	source = strings.Replace(source, "class CtorScopeDriver", "}class CtorScopeDriver", 1)
	source = strings.ReplaceAll(source, "new CtorScopeRoot(", "new CtorScopeContainer().new CtorScopeRoot(")
	source = strings.ReplaceAll(source, "CtorScopeRoot root=", "CtorScopeContainer.CtorScopeRoot root=")
	source = strings.Replace(source, "getDeclaringClass()!=CtorScopeRoot.class", "getDeclaringClass()!=CtorScopeContainer.CtorScopeRoot.class", 1)
	source = strings.Replace(source, "getParameterTypes().length!=3", "getParameterTypes().length!=4", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeContainer"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}

func TestNativeConstructorLocalScopeRequiresActualMethodAndInitializedSites(t *testing.T) {
	original := nativeCompileClasses(t, nativeConstructorLocalOwnedFixture)
	for _, variant := range []string{"original", "wrong owner", "wrong method role", "wrong physical descriptor", "copied declaration", "changed actual method name", "duplicate declaration", "duplicate body", "missing body", "wrong new PC", "wrong invoke PC", "changed local capture flags", "changed lexical method", "uninitialized owner at allocation", "missing allocation site", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range original {
				files[n] = append([]byte(nil), b...)
			}
			if variant == "uninitialized owner at allocation" {
				nativeConstructorLocalMoveNewBeforeDelegation(t, files)
				_, java := t04Tools(t)
				dir := t.TempDir()
				for n, b := range files {
					if err := os.WriteFile(filepath.Join(dir, n), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "CtorScopeDriver").CombinedOutput()
				if err != nil || string(out) != "30:ctor:local:live:wide:scope:failure\n" {
					t.Fatalf("original verifier/oracle:%v\n%s", err, out)
				}
			}
			root, err := Parse(files["CtorScopeRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			local, err := Parse(files["CtorScopeRoot$1View.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			owner, known := originalMethodLocalOwner(local, root, nil)
			if !known {
				t.Fatal("original scope")
			}
			ctor, known := originalMethodLocalSourceConstructor(local, root, nil)
			if !known {
				t.Fatal("original local constructor")
			}
			sites, known := d.nativeMethodLocalAllocationFacts(local, owner, ctor, true)
			if !known {
				t.Fatal("original exact allocation facts")
			}
			switch variant {
			case "wrong owner":
				owner.owner = "DifferentRoot"
			case "wrong method role":
				owner.method = "ordinary"
			case "wrong physical descriptor":
				owner.descriptor = "()V"
			case "copied declaration":
				copy := *owner.declaration
				owner.declaration = &copy
			case "changed actual method name":
				root.ConstantPool[owner.declaration.NameIndex-1].(*ConstantUtf8Info).Value = "ordinary"
			case "duplicate declaration":
				root.Methods = append(root.Methods, owner.declaration)
			case "duplicate body":
				for _, a := range owner.declaration.Attributes {
					if _, ok := a.(*CodeAttribute); ok {
						owner.declaration.Attributes = append(owner.declaration.Attributes, a)
						break
					}
				}
			case "missing body":
				owner.declaration.Attributes = nil
			case "wrong new PC":
				for pc, site := range sites {
					delete(sites, pc)
					site.newPC++
					sites[site.newPC] = site
					break
				}
			case "wrong invoke PC":
				for pc, site := range sites {
					site.invokePC++
					sites[pc] = site
					break
				}
			case "changed local capture flags":
				local.Fields[0].AccessFlags &^= 0x10
			case "changed lexical method":
				for _, a := range local.Attributes {
					if x, ok := a.(*UnparsedAttribute); ok && x.Name == "EnclosingMethod" {
						x.Info[3] = 0
						x.Info[2] = 0
					}
				}
			case "missing allocation site":
				sites = map[int]nativeMethodLocalAllocation{}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.nativeMethodLocalConstructorScopeReady(local, owner, sites, nil)
			if got != (variant == "original") {
				t.Fatalf("constructor declaration phase=%v", got)
			}
		})
	}
}

// This valid JVM constructor allocates the local object before SUPER, retains
// its uninitialized value in a distinct local, then invokes it after SUPER.
// A Java-8 local declaration cannot move that allocation past the parent call.
func nativeConstructorLocalMoveNewBeforeDelegation(t *testing.T, files map[string][]byte) {
	t.Helper()
	root, err := Parse(files["CtorScopeRoot.class"])
	if err != nil {
		t.Fatal(err)
	}
	var code *CodeAttribute
	for _, m := range root.Methods {
		name, _ := sourceBridgeUTF8(root, m.NameIndex)
		if name != "<init>" {
			continue
		}
		for _, a := range m.Attributes {
			if c, ok := a.(*CodeAttribute); ok {
				code = c
			}
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 || code.MaxLocals > 254 {
		t.Fatal("straight-line authored constructor")
	}
	decoder := core.NewDecompiler(code.Code, nil)
	if decoder.ParseOpcode() != nil {
		t.Fatal("original bytecode")
	}
	pc := -1
	var operand []byte
	for _, op := range decoder.Opcodes() {
		if op.Instr.OpCode == core.OP_NEW {
			name, _ := sourceBridgeClassName(root, core.Convert2bytesToInt(op.Data))
			if name == "CtorScopeRoot$1View" {
				pc = int(op.CurrentOffset)
				operand = append([]byte(nil), op.Data...)
			}
		}
	}
	if pc < 0 || len(operand) != 2 {
		t.Fatal("one local NEW")
	}
	slot := byte(code.MaxLocals)
	code.MaxLocals++
	nativeForeignAnonymousShiftDebugRanges(t, code, pc+3, -1)
	code.Code = append(append(append([]byte(nil), code.Code[:pc]...), byte(core.OP_ALOAD), slot), code.Code[pc+3:]...)
	prefix := []byte{byte(core.OP_NEW), operand[0], operand[1], byte(core.OP_ASTORE), slot}
	nativeForeignAnonymousShiftDebugRanges(t, code, 0, len(prefix))
	code.Code = append(prefix, code.Code...)
	code.AttrLen += 4
	files["CtorScopeRoot.class"] = root.Bytes()
}

func nativeConstructorLocalNullPortFixture() string {
	return strings.Replace(nativeConstructorLocalOwnedFixture, "Class<?>local=root.view.getClass();", `Class<?>local=root.view.getClass();int ports=0;
 for(java.lang.reflect.Field f:local.getDeclaredFields())if(f.isSynthetic()&&f.getType()==CtorScopeRoot.class){f.setAccessible(true);f.set(root.view,null);ports++;if(root.view.owner()!=null||root.view.token()!=captured)throw new AssertionError("distinct nullable enclosing/captured ports");try{root.view.read();throw new AssertionError("missing original enclosing NPE");}catch(NullPointerException expected){}f.set(root.view,root);}
 if(ports!=1||root.view.owner()!=root)throw new AssertionError("original enclosing storage identity");`, 1)
}
func nativeConstructorLocalNamedFixture(source string) string {
	source = strings.Replace(source, "class CtorScopeRoot extends CtorScopeBase{", "class CtorScopeContainer{class CtorScopeRoot extends CtorScopeBase{", 1)
	source = strings.Replace(source, "class CtorScopeDriver", "}class CtorScopeDriver", 1)
	source = strings.ReplaceAll(source, "new CtorScopeRoot(", "new CtorScopeContainer().new CtorScopeRoot(")
	source = strings.ReplaceAll(source, "CtorScopeRoot root=", "CtorScopeContainer.CtorScopeRoot root=")
	source = strings.ReplaceAll(source, "CtorScopeRoot.class", "CtorScopeContainer.CtorScopeRoot.class")
	return strings.Replace(source, "getParameterTypes().length!=3", "getParameterTypes().length!=4", 1)
}
func TestAdversarialConstructorLocalNullableStorageAndOriginalIdentity(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeConstructorLocalNullPortFixture(), []string{"CtorScopeRoot"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalPackagedDeclarations(t *testing.T) {
	testNativeIndependentFamilyFixture(t, "package ctor.scope;\n"+nativeConstructorLocalNullPortFixture(), []string{"ctor/scope/CtorScopeRoot"}, "ctor.scope.CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalRenamedDeclarations(t *testing.T) {
	source := strings.NewReplacer("CtorScopeRoot", "DifferentConstructorOwner", "CtorScopeBase", "DifferentConstructorParent", "CtorScopeRead", "DifferentReadPort", "View", "DeferredReading").Replace(nativeConstructorLocalNullPortFixture())
	testNativeIndependentFamilyFixture(t, source, []string{"DifferentConstructorOwner"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalPrivateFieldRemainsLive(t *testing.T) {
	source := strings.Replace(nativeConstructorLocalNullPortFixture(), "long mutable;CtorScopeRead view;", "private long mutable;void change(long n){mutable=n;}CtorScopeRead view;", 1)
	source = strings.ReplaceAll(source, "root.mutable=state;", "root.change(state);")
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeRoot"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalKeepsMethodFormalDeclaration(t *testing.T) {
	source := strings.Replace(nativeConstructorLocalNullPortFixture(), "CtorScopeRoot(final Object captured,", "<T> CtorScopeRoot(final T captured,", 1)
	source = strings.Replace(source, "getParameterTypes().length!=3", "getParameterTypes().length!=3||local.getEnclosingConstructor().getTypeParameters().length!=1", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeRoot"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalsAcrossOverloadedThisDelegation(t *testing.T) {
	source := nativeConstructorLocalOwnedFixture
	extra := ` CtorScopeRoot(final Object captured,final long wide,final int delta,boolean unused){this(captured,wide,delta);
 class View implements CtorScopeRead{public long read(){return mutable+wide+delta;}public Object owner(){return CtorScopeRoot.this;}public Object token(){return captured;}}
 view=new View();}
`
	marker := "\n}\nclass CtorScopeDriver"
	if !strings.Contains(source, marker) {
		t.Fatal("root class boundary")
	}
	source = strings.Replace(source, marker, "\n"+extra+"}\nclass CtorScopeDriver", 1)
	source = strings.Replace(source, "for(Object captured:new Object[]{null,new Object()}){", "for(Object captured:new Object[]{null,new Object()})for(boolean alternate:new boolean[]{false,true}){", 1)
	source = strings.Replace(source, "CtorScopeRoot root=new CtorScopeRoot(captured,wide,delta);", "CtorScopeRoot root=alternate?new CtorScopeRoot(captured,wide,delta,true):new CtorScopeRoot(captured,wide,delta);", 1)
	source = strings.Replace(source, "getParameterTypes().length!=3", "getParameterTypes().length!=(alternate?4:3)", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeRoot"}, "CtorScopeDriver", "60:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorLocalOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, named := range []bool{false, true} {
		t.Run(fmt.Sprint(named), func(t *testing.T) {
			source := nativeConstructorLocalNullPortFixture()
			root := "CtorScopeRoot"
			if named {
				source = nativeConstructorLocalNamedFixture(source)
				root = "CtorScopeContainer"
			}
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, root, debug) }, NativeJavac8, javac, []string{root}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}
