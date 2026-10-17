package javaclassparser

import (
	"bytes"
	"context"
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
