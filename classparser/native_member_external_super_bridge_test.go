package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"strings"
	"testing"
)

func TestNativeForeignSuperOwnBridgeRequiresExactOriginalPacket(t *testing.T) {
	fixture := strings.Replace(crossFamilyEnclosingFixture, "Leaf(int n)throws java.io.IOException{super(n);}", "private Leaf(int n)throws java.io.IOException{super(n);}", 1)
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "foreign outer parameter", "missing parent", "static parent", "private parent", "wrong parent descriptor", "missing ancestor", "wrong ancestor identity", "cyclic ancestor", "static child", "wrong capture flags", "wrong enclosing slot", "wrong call PC", "wrong call descriptor", "own bridge marker used", "own bridge foreign target", "own bridge extra effect", "own bridge wrong synthetic flags", "ordinary THIS delegate", "own bridge duplicate", "graph budget", "memory", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			selected := files
			if variant == "foreign outer parameter" {
				f := strings.Replace(fixture, "Leaf(int n)throws java.io.IOException{super(n);}", "Leaf(BindingBase other,int n)throws java.io.IOException{other.super(n);}", 1)
				f = strings.Replace(f, "new Leaf(n)", "new Leaf(this,n)", 1)
				selected = nativeCompileClasses(t, f)
			}
			z := nativeArchive(t, selected)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), selected["BindingBase.class"]...))
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original parent proof")
			}
			obj, e := Parse(append([]byte(nil), selected["BindingCurrent$Leaf.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(obj)
			var method *MemberInfo
			var code *CodeAttribute
			pc := -1
			desc := ""
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n != "<init>" || m.AccessFlags&0x1000 != 0 {
					continue
				}
				method = m
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original constructor")
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("original body")
			}
			for _, op := range decoder.Opcodes() {
				member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
				if member != nil && member.Name == "BindingBase$Member" {
					pc = int(op.CurrentOffset)
					desc = member.Description
				}
			}
			if pc < 0 {
				t.Fatal("original SUPER call")
			}
			parent := p.children["BindingBase$Member"]
			switch variant {
			case "missing parent":
				delete(p.children, "BindingBase$Member")
			case "static parent":
				parent.static = true
			case "private parent":
				for _, m := range parent.object.Methods {
					n, _ := sourceBridgeUTF8(parent.object, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "wrong parent descriptor":
				parent.constructors = map[string]*nativeMemberConstructor{}
			case "missing ancestor", "wrong ancestor identity", "cyclic ancestor":
				old := d.foldSiblingResolver
				d.foldSiblingResolver = func(n string) ([]byte, bool) {
					if n == "BindingCurrent" {
						if variant == "missing ancestor" {
							return nil, false
						}
						if variant == "wrong ancestor identity" {
							return selected["BindingBase.class"], true
						}
						o, _ := Parse(selected["BindingCurrent.class"])
						o.SuperClass = o.ThisClass
						return o.Bytes(), true
					}
					return old(n)
				}
			case "static child":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == obj.GetClassName() {
								row.InnerClassAccessFlags |= 8
							}
						}
					}
				}
			case "wrong capture flags":
				for _, f := range obj.Fields {
					n, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if n == "this$0" {
						f.AccessFlags &^= 0x10
					}
				}
			case "wrong enclosing slot":
				code.Code[6] = byte(core.OP_ALOAD_2)
			case "wrong call PC":
				pc++
			case "wrong call descriptor":
				desc = "(LBindingBase;J)V"
			case "own bridge marker used", "own bridge foreign target", "own bridge extra effect", "own bridge wrong synthetic flags", "ordinary THIS delegate", "own bridge duplicate":
				for _, m := range obj.Methods {
					if m.AccessFlags != 0x1000 {
						continue
					}
					for _, attr := range m.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							switch variant {
							case "own bridge marker used":
								code.Code = append(code.Code[:len(code.Code)-1], byte(core.OP_ALOAD_3), byte(core.OP_POP), byte(core.OP_RETURN))
							case "own bridge extra effect":
								code.Code = append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_POP)}, code.Code...)
							case "own bridge wrong synthetic flags":
								m.AccessFlags = 0x1001
							case "ordinary THIS delegate":
								// Ordinary THIS delegation retains a source
								// parameter; this physical SUPER proof does
								// not erase it or certify its source family.
								m.AccessFlags = 0
							case "own bridge duplicate":
								obj.Methods = append(obj.Methods, m)
							case "own bridge foreign target":
								decoder := core.NewDecompiler(code.Code, nil)
								if decoder.ParseOpcode() != nil {
									t.Fatal("original bridge")
								}
								for _, op := range decoder.Opcodes() {
									if op.Instr.OpCode == core.OP_INVOKESPECIAL {
										index := core.Convert2bytesToInt(op.Data)
										ref := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
										ref.ClassIndex = obj.SuperClass
									}
								}
							}
						}
					}
					break
				}
			case "graph budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.nativeMemberForeignOriginalSuper(p, method, "BindingBase$Member", desc, pc)
			if got != (variant == "original" || variant == "ordinary THIS delegate") {
				t.Fatalf("original SUPER closure=%v", got)
			}
		})
	}
}

const nativeForeignNamedInAnonymousFixture = `
class ForeignSlotRoot {
 class Entry {
  final int value;final Object observed;
  Entry(int value){this.value=value;observed=scope();}
  Object scope(){return ForeignSlotRoot.this;}
  Object enclosing(){return ForeignSlotRoot.this;}
 }
 Entry make(int n){return new Entry(n);}
}
class ForeignSlotCaller extends ForeignSlotRoot {
 ForeignSlotRoot wrap(){return new ForeignSlotCaller(){
  class Decorated extends Entry {
   Decorated(int n){super(n);}
   Object scope(){return ForeignSlotCaller.this;}
  }
  Entry make(int n){return new Decorated(n);}
 };}
}
class ForeignSlotDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   ForeignSlotRoot plain=new ForeignSlotRoot();ForeignSlotRoot.Entry direct=plain.make(n);
   if(direct.value!=n||direct.observed!=plain||direct.enclosing()!=plain)throw new AssertionError("direct scope");
   for(boolean cleared:new boolean[]{false,true}){
    ForeignSlotCaller caller=new ForeignSlotCaller();ForeignSlotRoot wrapped=caller.wrap();
    if(cleared){int fields=0;for(java.lang.reflect.Field f:wrapped.getClass().getDeclaredFields())if(f.getType()==ForeignSlotCaller.class){f.setAccessible(true);f.set(wrapped,null);fields++;}if(fields!=1)throw new AssertionError("hidden caller port");}
    ForeignSlotRoot.Entry entry=wrapped.make(n);Object expected=cleared?null:caller;
    if(entry.value!=n||entry.observed!=expected||entry.scope()!=expected||entry.enclosing()!=wrapped)throw new AssertionError("capture, callback and distinct enclosing roles");
    if(entry.getClass().getDeclaringClass()!=wrapped.getClass()||!wrapped.getClass().isAnonymousClass())throw new AssertionError("original foreign lexical metadata");
    rows++;
   }
  }
  System.out.println(rows+":foreign:named:anonymous:capture:callback");
 }
}`

func TestAdversarialForeignNamedInAnonymousSuperKeepsPhysicalParentBinding(t *testing.T) {
	// Only the parent family is replaced. The foreign anonymous owner, its
	// named child and the driver remain the original class files, so accepting
	// their physical SUPER packet must not borrow their source ownership.
	testNativeIndependentFamilyFixture(t, nativeForeignNamedInAnonymousFixture, []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", nativeLexicalExactSignatures)
}

func nativeForeignNamedStaticContextFixture() string {
	return strings.NewReplacer("ForeignSlotRoot wrap()", "static ForeignSlotRoot wrap(final ForeignSlotCaller captured)", "return ForeignSlotCaller.this;", "return captured;", "caller.wrap()", "ForeignSlotCaller.wrap(caller)").Replace(nativeForeignNamedInAnonymousFixture)
}

func TestAdversarialForeignNamedAnonymousSuperKeepsStaticFactoryContext(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeForeignNamedStaticContextFixture(), []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignNamedAnonymousSuperRenamedContext(t *testing.T) {
	source := strings.NewReplacer("ForeignSlotRoot", "ExternalEntryBase", "ForeignSlotCaller", "HiddenScopeCaller", "Decorated", "DifferentInner").Replace(nativeForeignNamedInAnonymousFixture)
	testNativeIndependentFamilyFixture(t, source, []string{"ExternalEntryBase"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignNamedSuperKeepsNestedMemberContext(t *testing.T) {
	source := strings.NewReplacer(
		"ForeignSlotRoot wrap(){return new ForeignSlotCaller(){", "ForeignSlotRoot wrap(){return new Box();}class Box extends ForeignSlotCaller{",
		" };}", " }",
		"!wrapped.getClass().isAnonymousClass()", "!wrapped.getClass().isMemberClass()",
	).Replace(nativeForeignNamedInAnonymousFixture)
	testNativeIndependentFamilyFixture(t, source, []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignNamedAnonymousSuperOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, row := range []struct{ name, source string }{{"instance", nativeForeignNamedInAnonymousFixture}, {"static", nativeForeignNamedStaticContextFixture()}} {
		t.Run(row.name, func(t *testing.T) {
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, row.source, "ForeignSlotRoot", debug)
			}, NativeJavac8, javac, []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestNativeForeignNamedAnonymousSuperRequiresOriginalPhysicalContext(t *testing.T) {
	files := nativeCompileClasses(t, nativeForeignNamedInAnonymousFixture)
	for _, variant := range []string{"original", "caller changes while resolving owner", "stale parent definition", "copied method", "duplicate method", "missing lexical owner", "wrong lexical owner", "missing enclosing method", "wrong reciprocal row", "duplicate reciprocal row", "static context contradiction", "wrong own enclosing slot", "wrong capture flag", "wrong capture name", "wrong call PC", "wrong call descriptor", "parent absent", "parent private", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for _, raw := range files {
				o, err := Parse(bytes.Clone(raw))
				if err != nil {
					t.Fatal(err)
				}
				objects[o.GetClassName()] = o
			}
			z := nativeArchive(t, files)
			defer z.Close()
			p := z.nativeMemberReader(objects["ForeignSlotRoot"]).planNativeMemberFamily()
			if p == nil {
				t.Fatal("parent packet")
			}
			child := objects["ForeignSlotCaller$1$Decorated"]
			owner := objects["ForeignSlotCaller$1"]
			factory := objects["ForeignSlotCaller"]
			reader := z.nativeMemberReader(child)
			reader.foldSiblingResolver = func(name string) ([]byte, bool) {
				o := objects[name]
				if o == nil {
					return nil, false
				}
				return o.Bytes(), true
			}
			var method *MemberInfo
			var code *CodeAttribute
			pc := -1
			desc := ""
			for _, m := range child.Methods {
				n, _ := sourceBridgeUTF8(child, m.NameIndex)
				if n == "<init>" {
					method = m
					for _, a := range m.Attributes {
						if body, ok := a.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("child constructor")
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("original body")
			}
			for _, op := range decoder.Opcodes() {
				call := constructorMotionMember(child, op, core.OP_INVOKESPECIAL)
				if call != nil && call.Name == "ForeignSlotRoot$Entry" {
					pc = int(op.CurrentOffset)
					desc = call.Description
				}
			}
			if pc < 0 {
				t.Fatal("parent call")
			}
			switch variant {
			case "caller changes while resolving owner":
				rawResolver := reader.foldSiblingResolver
				changed := false
				reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "ForeignSlotCaller$1" && !changed {
						changed = true
						child.ConstantPoolManager.AddUtf8Info("changed-original-caller")
					}
					return rawResolver(name)
				}
			case "stale parent definition":
				p.children["ForeignSlotRoot$Entry"].object.ConstantPoolManager.AddUtf8Info("stale-parent-definition")
			case "copied method":
				copy := *method
				method = &copy
			case "duplicate method":
				child.Methods = append(child.Methods, method)
			case "missing lexical owner":
				delete(objects, "ForeignSlotCaller")
			case "wrong lexical owner":
				objects["ForeignSlotCaller"] = objects["ForeignSlotRoot"]
			case "missing enclosing method":
				for _, m := range factory.Methods {
					n, _ := sourceBridgeUTF8(factory, m.NameIndex)
					if n == "wrap" {
						m.NameIndex = factory.ThisClass
					}
				}
			case "wrong reciprocal row", "duplicate reciprocal row":
				for _, a := range factory.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							name, _ := sourceBridgeClassName(factory, row.InnerClassInfoIndex)
							if name == owner.GetClassName() {
								if variant == "wrong reciprocal row" {
									row.InnerClassAccessFlags |= 8
								} else {
									table.Classes = append(table.Classes, row)
								}
								break
							}
						}
					}
				}
			case "static context contradiction":
				for _, m := range factory.Methods {
					n, _ := sourceBridgeUTF8(factory, m.NameIndex)
					if n == "wrap" {
						m.AccessFlags |= 8
					}
				}
			case "wrong own enclosing slot":
				code.Code[1] = byte(core.OP_ALOAD_2)
			case "wrong capture flag", "wrong capture name":
				for _, f := range child.Fields {
					if f.AccessFlags&0x1000 != 0 {
						if variant == "wrong capture flag" {
							f.AccessFlags &^= 0x10
						} else {
							f.NameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("this$0"))
						}
					}
				}
			case "wrong call PC":
				pc++
			case "wrong call descriptor":
				desc = "(LForeignSlotRoot;J)V"
			case "parent absent":
				delete(p.children, "ForeignSlotRoot$Entry")
			case "parent private":
				for _, m := range p.children["ForeignSlotRoot$Entry"].object.Methods {
					n, _ := sourceBridgeUTF8(p.children["ForeignSlotRoot$Entry"].object, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := reader.nativeMemberForeignOriginalSuper(p, method, "ForeignSlotRoot$Entry", desc, pc); got != (variant == "original") {
				t.Fatalf("physical context=%v", got)
			}
		})
	}
}

func nativeForeignNestedAnonymousBinaryFixture(t *testing.T) string {
	old := `class Decorated extends Entry {
   Decorated(int n){super(n);}
   Object scope(){return ForeignSlotCaller.this;}
  }
  Entry make(int n){return new Decorated(n);}`
	replacement := `Entry make(int n){return new Entry(n){Object scope(){return ForeignSlotCaller.this;}};}`
	source := strings.Replace(nativeForeignNamedInAnonymousFixture, old, replacement, 1)
	if strings.Contains(source, "class Decorated") {
		t.Fatal("authored nested-anonymous fixture did not replace its named child")
	}
	source = strings.Replace(source, "class ForeignSlotCaller extends ForeignSlotRoot {", "class ForeignSlotCaller extends ForeignSlotRoot {static class Marker{static Object cache;}", 1)
	source = strings.Replace(source, "entry.getClass().getDeclaringClass()!=wrapped.getClass()", "entry.getClass().getEnclosingClass()!=wrapped.getClass()||!entry.getClass().isAnonymousClass()", 1)
	source = strings.Replace(source, "int rows=0;", "int rows=0;if(Class.forName(\"ForeignSlotCaller$Marker\").getDeclaredField(\"cache\").getType()!=Object.class)throw new AssertionError(\"unchanged foreign marker declaration\");", 1)
	return source
}

func TestAdversarialForeignNestedAnonymousSuperOnlyBorrowsPhysicalBinding(t *testing.T) {
	source := nativeForeignNestedAnonymousBinaryFixture(t)
	for _, variant := range []string{"ordinary foreign source", "unrepresentable unrelated foreign field", "unrelated foreign field and independent NEW", "unrelated foreign field and independent NEW null owner", "unrelated foreign field and independent NEW after SUPER"} {
		t.Run(variant, func(t *testing.T) {
			mutate := func(t *testing.T, files map[string][]byte) {
				if variant == "ordinary foreign source" {
					return
				}
				const name = "ForeignSlotCaller$Marker.class"
				object, err := Parse(files[name])
				if err != nil {
					t.Fatal(err)
				}
				changed := 0
				for _, field := range object.Fields {
					n, _ := sourceBridgeUTF8(object, field.NameIndex)
					if n == "cache" {
						field.AccessFlags |= 0x1000
						changed++
					}
				}
				if changed != 1 {
					t.Fatal("one original marker field required")
				}
				files[name] = object.Bytes()
				if strings.Contains(variant, "independent NEW") {
					const caller = "ForeignSlotCaller$1$1.class"
					object, err := Parse(files[caller])
					if err != nil {
						t.Fatal(err)
					}
					var options []string
					if strings.Contains(variant, "null owner") {
						options = append(options, "null owner")
					}
					if strings.Contains(variant, "after SUPER") {
						options = append(options, "after SUPER")
					}
					nativeForeignAnonymousInsertIndependentNew(t, object, options...)
					files[caller] = object.Bytes()
				}
			}
			// Foreign callers, their metadata and every foreign body are kept
			// as original class files. A binary SUPER consumer cannot grant the
			// parent family ownership of their otherwise unrepresentable source.
			testNativeIndependentMutatedFamilyFixture(t, source, []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", mutate, nativeLexicalExactSignatures)
		})
	}
}

type nativeForeignAnonymousPhysicalTestState struct {
	reader     *ClassObjectDumper
	family     *nativeMemberFamily
	objects    map[string]*ClassObject
	method     *MemberInfo
	code       *CodeAttribute
	pc         int
	descriptor string
}

func nativeForeignAnonymousPhysicalSetup(t *testing.T, files map[string][]byte) *nativeForeignAnonymousPhysicalTestState {
	t.Helper()
	objects := map[string]*ClassObject{}
	for _, raw := range files {
		o, err := Parse(bytes.Clone(raw))
		if err != nil {
			t.Fatal(err)
		}
		objects[o.GetClassName()] = o
	}
	z := nativeArchive(t, files)
	t.Cleanup(func() { z.Close() })
	p := z.nativeMemberReader(objects["ForeignSlotRoot"]).planNativeMemberFamily()
	if p == nil {
		t.Fatal("parent declaration")
	}
	child := objects["ForeignSlotCaller$1$1"]
	d := z.nativeMemberReader(child)
	d.foldSiblingResolver = func(name string) ([]byte, bool) {
		o := objects[name]
		if o == nil {
			return nil, false
		}
		return o.Bytes(), true
	}
	s := &nativeForeignAnonymousPhysicalTestState{reader: d, family: p, objects: objects, pc: -1}
	for _, m := range child.Methods {
		n, _ := sourceBridgeUTF8(child, m.NameIndex)
		if n == "<init>" {
			s.method = m
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					s.code = code
				}
			}
		}
	}
	if s.method == nil || s.code == nil {
		t.Fatal("constructor input")
	}
	decoder := core.NewDecompiler(s.code.Code, nil)
	if decoder.ParseOpcode() != nil {
		t.Fatal("constructor bytes")
	}
	for _, op := range decoder.Opcodes() {
		call := constructorMotionMember(child, op, core.OP_INVOKESPECIAL)
		if call != nil && call.Name == child.GetSupperClassName() {
			s.pc = int(op.CurrentOffset)
			s.descriptor = call.Description
		}
	}
	if s.pc < 0 {
		t.Fatal("original SUPER")
	}
	return s
}

func nativeForeignAnonymousInsertIndependentNew(t *testing.T, object *ClassObject, options ...string) (int, int) {
	t.Helper()
	var code *CodeAttribute
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "<init>" {
			for _, a := range method.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
		}
	}
	if code == nil || len(code.ExceptionTable) != 0 {
		t.Fatal("straight-line authored constructor required")
	}
	decoder := core.NewDecompiler(code.Code, nil)
	if decoder.ParseOpcode() != nil {
		t.Fatal("original constructor")
	}
	ops := constructorMotionOps(decoder)
	if len(ops) < 4 {
		t.Fatal("capture prefix")
	}
	insert := int(ops[3].CurrentOffset)
	callPC := -1
	var reference []byte
	for _, op := range ops {
		call := constructorMotionMember(object, op, core.OP_INVOKESPECIAL)
		if call != nil && call.Name == object.GetSupperClassName() {
			if callPC >= 0 {
				t.Fatal("one initial parent call")
			}
			callPC = int(op.CurrentOffset)
			reference = bytes.Clone(op.Data)
		}
	}
	if callPC < 0 || len(reference) != 2 {
		t.Fatal("parent Methodref")
	}
	packet := []byte{byte(core.OP_NEW), byte(object.SuperClass >> 8), byte(object.SuperClass), byte(core.OP_DUP), byte(core.OP_ALOAD_1), byte(core.OP_ICONST_0), byte(core.OP_INVOKESPECIAL), reference[0], reference[1], byte(core.OP_POP)}
	after := false
	for _, option := range options {
		switch option {
		case "null owner":
			packet[4] = byte(core.OP_ACONST_NULL)
		case "after SUPER":
			after = true
		default:
			t.Fatal("unknown authored insertion", option)
		}
	}
	if after {
		insert = callPC + 3
	}
	nativeForeignAnonymousShiftDebugRanges(t, code, insert, len(packet))
	code.Code = append(append(append([]byte(nil), code.Code[:insert]...), packet...), code.Code[insert:]...)
	code.AttrLen += uint32(len(packet))
	if code.MaxStack < 4 {
		code.MaxStack = 4
	}
	if _, err := Parse(object.Bytes()); err != nil {
		t.Fatal("patched classfile", err)
	}
	if callPC >= insert {
		callPC += len(packet)
	}
	return callPC, insert + 6
}

// Insertion changes instruction addresses, not the original parameter/debug
// identities. Preserve both boundaries of every live range; dropping debug
// attributes would conceal the source variant the regression must exercise.
func nativeForeignAnonymousShiftDebugRanges(t *testing.T, code *CodeAttribute, at, delta int) {
	t.Helper()
	for _, attribute := range code.Attributes {
		switch a := attribute.(type) {
		case *LineNumberTableAttribute:
			for _, row := range a.LineNumberTable {
				if int(row.StartPc) >= at {
					row.StartPc += uint16(delta)
				}
			}
		case *UnparsedAttribute:
			if a.Name != "LocalVariableTable" && a.Name != "LocalVariableTypeTable" {
				t.Fatalf("unhandled authored instruction-bearing debug attribute %s", a.Name)
			}
			if len(a.Info) < 2 {
				t.Fatal("truncated debug table")
			}
			count := int(binary.BigEndian.Uint16(a.Info[:2]))
			if len(a.Info) != 2+10*count {
				t.Fatal("debug table shape")
			}
			for i := 0; i < count; i++ {
				row := a.Info[2+10*i : 2+10*(i+1)]
				start := int(binary.BigEndian.Uint16(row[:2]))
				end := start + int(binary.BigEndian.Uint16(row[2:4]))
				if start >= at {
					start += delta
					end += delta
				} else if end > at {
					end += delta
				}
				binary.BigEndian.PutUint16(row[:2], uint16(start))
				binary.BigEndian.PutUint16(row[2:4], uint16(end-start))
			}
		default:
			t.Fatalf("unhandled authored constructor attribute %T", attribute)
		}
	}
}

func TestNativeForeignAnonymousPhysicalSuperKeepsOriginalIdentityAndPermissions(t *testing.T) {
	files := nativeCompileClasses(t, nativeForeignNestedAnonymousBinaryFixture(t))
	for _, variant := range []string{"original", "failed family", "owned root", "owned named", "owned local", "owned anonymous", "owned enum", "parent absent", "parent static", "parent private", "parent bridge", "stale parent", "copied method", "duplicate method", "duplicate body", "missing body", "wrong PC", "wrong descriptor", "wrong superclass", "wrong capture receiver", "wrong capture value", "wrong capture owner", "wrong capture flags", "wrong capture name", "wrong capture descriptor", "duplicate capture", "missing context", "foreign context", "missing ancestor", "cyclic ancestor", "wrong anonymous role", "duplicate enclosing method", "caller changes during resolution", "parent changes during resolution", "independent NEW before real SUPER", "NEW is not SUPER", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			s := nativeForeignAnonymousPhysicalSetup(t, files)
			child := s.reader.obj
			parent := s.family.children["ForeignSlotRoot$Entry"]
			switch variant {
			case "failed family":
				s.family.failed = true
			case "owned root":
				s.family.owner = child.GetClassName()
			case "owned named":
				s.family.children[child.GetClassName()] = &nativeMemberClass{object: child}
			case "owned local":
				s.family.methodLocals = map[string]*nativeMethodLocalClass{child.GetClassName(): {object: child}}
			case "owned anonymous":
				s.family.anonymousUnits = map[string]*nativeAnonymousFamily{child.GetClassName(): {}}
			case "owned enum":
				s.family.enumConstants = map[string]*nativeEnumConstantBody{child.GetClassName(): {}}
			case "parent absent":
				delete(s.family.children, "ForeignSlotRoot$Entry")
			case "parent static":
				parent.static = true
			case "parent private":
				for _, m := range parent.object.Methods {
					n, _ := sourceBridgeUTF8(parent.object, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "parent bridge":
				parent.accessBridges = map[string]*nativeConstructorAccessBridge{s.descriptor: {}}
			case "stale parent":
				parent.object.ConstantPoolManager.AddUtf8Info("changed-parent")
			case "copied method":
				copy := *s.method
				s.method = &copy
			case "duplicate method":
				child.Methods = append(child.Methods, s.method)
			case "duplicate body":
				s.method.Attributes = append(s.method.Attributes, s.code)
			case "missing body":
				var attrs []AttributeInfo
				for _, a := range s.method.Attributes {
					if _, ok := a.(*CodeAttribute); !ok {
						attrs = append(attrs, a)
					}
				}
				s.method.Attributes = attrs
			case "wrong PC":
				s.pc++
			case "wrong descriptor":
				s.descriptor = "(LForeignSlotRoot;J)V"
			case "wrong superclass":
				child.SuperClass = child.ThisClass
			case "wrong capture receiver":
				s.code.Code[0] = byte(core.OP_ALOAD_1)
			case "wrong capture value":
				s.code.Code[1] = byte(core.OP_ALOAD_2)
			case "wrong capture owner":
				index := core.Convert2bytesToInt(s.code.Code[3:5])
				child.ConstantPool[index-1].(*ConstantFieldrefInfo).ClassIndex = child.SuperClass
			case "wrong capture flags", "wrong capture name", "wrong capture descriptor", "duplicate capture":
				for _, f := range child.Fields {
					if f.AccessFlags&0x1000 != 0 {
						switch variant {
						case "wrong capture flags":
							f.AccessFlags &^= 0x10
						case "wrong capture name":
							f.NameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("this$0"))
						case "wrong capture descriptor":
							f.DescriptorIndex = uint16(child.ConstantPoolManager.AddUtf8Info("Ljava/lang/Object;"))
						case "duplicate capture":
							child.Fields = append(child.Fields, f)
						}
						break
					}
				}
			case "missing context":
				delete(s.objects, "ForeignSlotCaller$1")
			case "foreign context":
				s.objects["ForeignSlotCaller$1"] = s.objects["ForeignSlotRoot"]
			case "missing ancestor":
				delete(s.objects, "ForeignSlotCaller")
			case "cyclic ancestor":
				o := s.objects["ForeignSlotCaller$1"]
				o.SuperClass = o.ThisClass
			case "wrong anonymous role":
				for _, a := range child.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(child, row.InnerClassInfoIndex)
							if n == child.GetClassName() {
								row.InnerNameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("Named"))
							}
						}
					}
				}
			case "duplicate enclosing method":
				for _, a := range child.Attributes {
					if e, ok := a.(*UnparsedAttribute); ok && e.Name == "EnclosingMethod" {
						child.Attributes = append(child.Attributes, e)
						break
					}
				}
			case "caller changes during resolution", "parent changes during resolution":
				original := s.reader.foldSiblingResolver
				changed := false
				s.reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "ForeignSlotCaller$1" && !changed {
						changed = true
						if variant == "caller changes during resolution" {
							child.ConstantPoolManager.AddUtf8Info("changed-caller")
						} else {
							parent.object.ConstantPoolManager.AddUtf8Info("changed-parent-after-read")
						}
					}
					return original(name)
				}
			case "independent NEW before real SUPER", "NEW is not SUPER":
				actual, newPC := nativeForeignAnonymousInsertIndependentNew(t, child)
				s.pc = actual
				if variant == "NEW is not SUPER" {
					s.pc = newPC
				}
			case "budget":
				s.reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				s.reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				s.reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := *parent.constructors["(LForeignSlotRoot;I)V"]
			children, dependencies, getters := len(s.family.children), len(s.family.allocationDependencies), len(s.family.getters)
			got := s.reader.nativeMemberForeignAnonymousPhysicalSuper(s.family, s.method, "ForeignSlotRoot$Entry", s.descriptor, s.pc)
			want := variant == "original" || variant == "independent NEW before real SUPER"
			if got != want {
				t.Fatalf("physical SUPER=%v want=%v", got, want)
			}
			if *parent.constructors["(LForeignSlotRoot;I)V"] != before || len(s.family.children) != children || len(s.family.allocationDependencies) != dependencies || len(s.family.getters) != getters {
				t.Fatal("physical identity acquired source or constructor permissions")
			}
		})
	}
}

func TestNativeForeignAnonymousPhysicalSuperPreservesNarrowWordDomain(t *testing.T) {
	for _, domain := range []struct {
		name, source string
		low, high    int32
	}{{"boolean", "n>0", 0, 1}, {"byte", "(byte)n", -128, 127}, {"char", "(char)n", 0, 65535}, {"short", "(short)n", -32768, 32767}} {
		t.Run(domain.name, func(t *testing.T) {
			source := strings.NewReplacer("Entry(int value)", "Entry(long wide,"+domain.name+" narrow,int value)", "new Entry(n)", "new Entry(7L,"+domain.source+",n)").Replace(nativeForeignNestedAnonymousBinaryFixture(t))
			files := nativeCompileClasses(t, source)
			for _, value := range []int32{-2147483648, -32769, -32768, -129, -128, -1, 0, 1, 2, 127, 128, 32767, 32768, 65535, 65536, 2147483647} {
				t.Run(fmt.Sprint(value), func(t *testing.T) {
					s := nativeForeignAnonymousPhysicalSetup(t, files)
					object := s.reader.obj
					decoder := core.NewDecompiler(s.code.Code, nil)
					if decoder.ParseOpcode() != nil {
						t.Fatal("original")
					}
					ops := constructorMotionOps(decoder)
					loadPC, oldWidth := -1, 0
					for i, op := range ops {
						if int(op.CurrentOffset) < s.pc && core.GetRetrieveIdx(op) == 4 && op.Instr.OpCode == core.OP_ILOAD {
							loadPC = int(op.CurrentOffset)
							oldWidth = int(ops[i+1].CurrentOffset) - loadPC
						}
					}
					if loadPC < 0 {
						t.Fatal("original narrow argument after category-2 port")
					}
					object.ConstantPool = append(object.ConstantPool, &ConstantIntegerInfo{Value: value})
					index := len(object.ConstantPool)
					literal := []byte{byte(core.OP_LDC_W), byte(index >> 8), byte(index)}
					s.code.Code = append(append(append([]byte(nil), s.code.Code[:loadPC]...), literal...), s.code.Code[loadPC+oldWidth:]...)
					delta := len(literal) - oldWidth
					s.code.AttrLen += uint32(delta)
					s.pc += delta
					got := s.reader.nativeMemberForeignAnonymousPhysicalSuper(s.family, s.method, "ForeignSlotRoot$Entry", s.descriptor, s.pc)
					want := value >= domain.low && value <= domain.high
					if got != want {
						t.Fatalf("%s input %d proof=%v want=%v", domain.name, value, got, want)
					}
				})
			}
		})
	}
}

func nativeForeignAnonymousOpaqueMutation(t *testing.T, files map[string][]byte, prefix, variant string) {
	t.Helper()
	marker := prefix + "ForeignSlotCaller$Marker.class"
	object, err := Parse(files[marker])
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, f := range object.Fields {
		name, _ := sourceBridgeUTF8(object, f.NameIndex)
		if name == "cache" {
			f.AccessFlags |= 0x1000
			changed++
		}
	}
	if changed != 1 {
		t.Fatal("one foreign marker field")
	}
	files[marker] = object.Bytes()
	if variant != "plain" {
		name := prefix + "ForeignSlotCaller$1$1.class"
		object, err := Parse(files[name])
		if err != nil {
			t.Fatal(err)
		}
		var options []string
		if variant == "null owner" {
			options = append(options, "null owner")
		}
		nativeForeignAnonymousInsertIndependentNew(t, object, options...)
		files[name] = object.Bytes()
	}
}

func TestAdversarialForeignPhysicalConstructorsRetainPackagedInputs(t *testing.T) {
	source := "package ports.scope;\n" + nativeForeignNestedAnonymousBinaryFixture(t)
	source = strings.Replace(source, `Class.forName("ForeignSlotCaller$Marker")`, `Class.forName("ports.scope.ForeignSlotCaller$Marker")`, 1)
	testNativeIndependentMutatedFamilyFixture(t, source, []string{"ports/scope/ForeignSlotRoot"}, "ports.scope.ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", func(t *testing.T, files map[string][]byte) {
		nativeForeignAnonymousOpaqueMutation(t, files, "ports/scope/", "null owner")
	}, nativeLexicalExactSignatures)
}

func TestAdversarialForeignPhysicalConstructorsRetainRenamedInputs(t *testing.T) {
	source := strings.NewReplacer("ForeignSlotRoot", "DifferentValueRoot", "ForeignSlotCaller", "DifferentBinaryCaller").Replace(nativeForeignNestedAnonymousBinaryFixture(t))
	testNativeIndependentMutatedFamilyFixture(t, source, []string{"DifferentValueRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", func(t *testing.T, files map[string][]byte) {
		marker, err := Parse(files["DifferentBinaryCaller$Marker.class"])
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, f := range marker.Fields {
			name, _ := sourceBridgeUTF8(marker, f.NameIndex)
			if name == "cache" {
				f.AccessFlags |= 0x1000
				changed++
			}
		}
		if changed != 1 {
			t.Fatal("marker")
		}
		files["DifferentBinaryCaller$Marker.class"] = marker.Bytes()
		object, err := Parse(files["DifferentBinaryCaller$1$1.class"])
		if err != nil {
			t.Fatal(err)
		}
		nativeForeignAnonymousInsertIndependentNew(t, object)
		files["DifferentBinaryCaller$1$1.class"] = object.Bytes()
	}, nativeLexicalExactSignatures)
}

func TestAdversarialForeignPhysicalConstructorsOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	source := nativeForeignNestedAnonymousBinaryFixture(t)
	for _, variant := range []string{"plain", "independent NEW", "null owner"} {
		t.Run(variant, func(t *testing.T) {
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, source, "ForeignSlotRoot", debug)
			}, NativeJavac8, javac, []string{"ForeignSlotRoot"}, "ForeignSlotDriver", "10:foreign:named:anonymous:capture:callback\n", func(t *testing.T, files map[string][]byte) {
				nativeForeignAnonymousOpaqueMutation(t, files, "", variant)
			}, nativeLexicalExactSignatures)
		})
	}
}
