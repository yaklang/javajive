package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorRuntimeClassRequiresFreshReceiverAndExactBootstrap(t *testing.T) {
	files := nativeCompileClasses(t, `class QueryEvidenceBase{} final class QueryEvidenceChild extends QueryEvidenceBase{}`)
	bootstrap, ok := jdkConstructorClassBytes("java/lang/Object", 8)
	if !ok {
		t.Fatal("missing original bootstrap evidence")
	}
	for _, variant := range []string{
		"original", "inherited symbolic owner", "parent symbolic owner", "self storage", "runtime-specific evidence",
		"not fresh", "nil storage", "class mirror receiver", "class mirror ancestor", "final ancestor", "open finalizer", "special", "interface", "static",
		"wrong member", "wrong descriptor", "unrelated owner", "unrelated caller", "missing parent", "cycle", "interface ancestor",
		"root shadow", "parent shadow", "provider changes root methods", "provider changes root name", "missing native", "not final", "private", "static native", "synchronized native", "native with code", "duplicate method", "bootstrap parent",
		"chain budget", "work", "memory", "canceled",
	} {
		t.Run(variant, func(t *testing.T) {
			parse := func(raw []byte) *ClassObject {
				o, err := Parse(bytes.Clone(raw))
				if err != nil {
					t.Fatal(err)
				}
				return o
			}
			root := parse(files["QueryEvidenceChild.class"])
			parent := parse(files["QueryEvidenceBase.class"])
			object := parse(bootstrap)
			var target *MemberInfo
			for _, method := range object.Methods {
				n, _ := object.getUtf8(method.NameIndex)
				d, _ := object.getUtf8(method.DescriptorIndex)
				if n == "getClass" && d == "()Ljava/lang/Class;" {
					target = method
				}
			}
			if target == nil {
				t.Fatal("missing physical bootstrap method")
			}
			shadow := func(o *ClassObject) {
				o.Methods = append(o.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, o, "getClass"), DescriptorIndex: sourceBridgePoolString(t, o, "()Ljava/lang/Class;"), AccessFlags: 0x0001})
			}
			inputs := map[string]*ClassObject{parent.GetClassName(): parent, "java/lang/Object": object}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				if n == "java/lang/Object" {
					switch variant {
					case "provider changes root methods":
						shadow(root)
					case "provider changes root name":
						root.ConstantPool[root.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, root, "ChangedRoot")
					}
				}
				o := inputs[n]
				if o == nil {
					return nil, false
				}
				return o.Bytes(), true
			}
			member := &values.JavaClassMember{Name: "java/lang/Object", Member: "getClass", Description: "()Ljava/lang/Class;"}
			caller, opcode, remaining := parent, core.OP_INVOKEVIRTUAL, 512
			aliases := &constructorSelfStorageProof{freshReceiver: true}
			switch variant {
			case "inherited symbolic owner":
				member.Name = root.GetClassName()
			case "parent symbolic owner":
				member.Name = parent.GetClassName()
			case "self storage":
				aliases.selfStored = true
			case "runtime-specific evidence":
				d.constructorProfileEvidence = &constructorProfileEvidence{eligible: true}
			case "not fresh":
				aliases.freshReceiver = false
			case "nil storage":
				aliases = nil
			case "class mirror receiver":
				root.ConstantPool[root.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, root, "java/lang/Class")
			case "class mirror ancestor":
				parent.ConstantPool[parent.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, parent, "java/lang/Class")
				root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("java/lang/Class"))
				inputs["java/lang/Class"] = parent
			case "final ancestor":
				parent.AccessFlags |= 0x0010
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "special":
				opcode = core.OP_INVOKESPECIAL
			case "interface":
				opcode = core.OP_INVOKEINTERFACE
			case "static":
				opcode = core.OP_INVOKESTATIC
			case "wrong member":
				member.Member = "clone"
			case "wrong descriptor":
				member.Description = "()Ljava/lang/Object;"
			case "unrelated owner":
				member.Name = "OtherClass"
			case "unrelated caller":
				caller = parse(files["QueryEvidenceChild.class"])
				caller.ConstantPool[caller.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, caller, "OtherClass")
			case "missing parent":
				root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("MissingQueryParent"))
			case "cycle":
				parent.SuperClass = parent.ThisClass
			case "interface ancestor":
				parent.AccessFlags |= 0x0200
			case "root shadow":
				shadow(root)
			case "parent shadow":
				shadow(parent)
			case "missing native":
				target.AccessFlags &^= 0x0100
			case "not final":
				target.AccessFlags &^= 0x0010
			case "private":
				target.AccessFlags = 0x0112
			case "static native":
				target.AccessFlags |= 0x0008
			case "synchronized native":
				target.AccessFlags |= 0x0020
			case "native with code":
				target.Attributes = append(target.Attributes, &CodeAttribute{Type: "Code", MaxStack: 1, MaxLocals: 1, Code: []byte{core.OP_ALOAD_0, core.OP_ARETURN}})
			case "duplicate method":
				object.Methods = append(object.Methods, target)
			case "bootstrap parent":
				object.SuperClass = object.ThisClass
			case "chain budget":
				remaining = 1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.constructorReceiverRuntimeClass(caller, member, opcode, &remaining, aliases)
			want := variant == "original" || variant == "inherited symbolic owner" || variant == "parent symbolic owner" || variant == "self storage" || variant == "runtime-specific evidence"
			if got != want {
				t.Fatalf("bootstrap query=%v want=%v", got, want)
			}
			if aliases != nil && aliases.referenceRead {
				t.Fatal("type query acquired reference-field read evidence")
			}
			if variant == "runtime-specific evidence" && (!d.constructorProfileEvidence.eligible || !d.constructorProfileEvidence.runtimeSpecific) {
				t.Fatal("VM primitive must preserve owned evidence and forbid runtime certificate reuse")
			}
		})
	}
}

func TestNativeConstructorRuntimeClassEvidenceCannotShortcutAnotherRuntime(t *testing.T) {
	files := nativeCompileClasses(t, `final class RuntimeEvidenceRoot{}`)
	for _, specific := range []bool{false, true} {
		obj, err := Parse(bytes.Clone(files["RuntimeEvidenceRoot.class"]))
		if err != nil {
			t.Fatal(err)
		}
		d := NewClassObjectDumper(obj)
		d.options.TargetSourceVersion = 8
		d.FuncCtx = &class_context.ClassContext{}
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		e := &constructorProfileEvidence{eligible: true, rootName: obj.GetClassName(), rootSuper: obj.GetSupperClassName(), rootFlags: obj.AccessFlags}
		previous := *e
		previous.bootstrapUsed, previous.runtimeSpecific = true, specific
		remaining := 512
		if got := e.revalidates(d, &previous, &remaining); got == specific {
			t.Fatalf("runtime-specific=%v certificate reused=%v", specific, got)
		}
	}
}

const constructorStaticBorrowGuardFixture = `
class BorrowStaticBase {BorrowStaticBase(){BorrowStaticProbe.kind(this);}}
final class BorrowStaticChild extends BorrowStaticBase {Object capture;}
class BorrowStaticProbe {
 static Object sink;
 static Class<?> kind(Object value){return value.getClass();}
 static Class<?> transit(Object value){return kind(value);}
 static Class<?> typedBase(BorrowStaticBase value){return value.getClass();}
 static Class<?> unrelated(String value){return value.getClass();}
 static Class<?> impossibleArray(Object[] value){return value.getClass();}
 static Class<?> wide(long first,Object value,double second,int third,Object duplicate){return value.getClass();}
 static Object alias(Object value){return value;}
 static Class<?> escape(Object value){sink=value;return value.getClass();}
 static Class<?> array(Object value,Object[] port){port[0]=value;return value.getClass();}
 static Class<?> field(Object value){Object copy=((BorrowStaticChild)value).capture;return value.getClass();}
 static Class<?> write(Object value){((BorrowStaticChild)value).capture=null;return value.getClass();}
 static Class<?> opaque(Object value){System.identityHashCode(value);return value.getClass();}
 static Class<?> branch(Object value,int flag){if(flag<0)sink=value;return value.getClass();}
 static Class<?> cycle(Object value){return cycle(value);}
 static Class<?> retained(Object value){new BorrowStaticHolder(value);return value.getClass();}
 static synchronized Class<?> locked(Object value){return value.getClass();}
 static native Class<?> nativeKind(Object value);
 static Class<?> typedHandler(Object value){try{return value.getClass();}catch(RuntimeException failure){throw failure;}}
}
class BorrowStaticHolder {final Object value;BorrowStaticHolder(Object input){value=input;}}
class BorrowStaticInherited extends BorrowStaticProbe {}
interface BorrowStaticInterface {static Class<?> kind(Object value){return value.getClass();}}
`

func TestNativeConstructorStaticBorrowKeepsOriginalPortsAndConfinement(t *testing.T) {
	files := nativeCompileClasses(t, constructorStaticBorrowGuardFixture)
	for _, row := range []struct {
		name, method, descriptor string
		want                     bool
	}{
		{"class mirror", "kind", "(Ljava/lang/Object;)Ljava/lang/Class;", true},
		{"transitive", "transit", "(Ljava/lang/Object;)Ljava/lang/Class;", true},
		{"original superclass port", "typedBase", "(LBorrowStaticBase;)Ljava/lang/Class;", true},
		{"unrelated class port", "unrelated", "(Ljava/lang/String;)Ljava/lang/Class;", false},
		{"array port", "impossibleArray", "([Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"wide ports", "wide", "(JLjava/lang/Object;DILjava/lang/Object;)Ljava/lang/Class;", true},
		{"return receiver", "alias", "(Ljava/lang/Object;)Ljava/lang/Object;", false},
		{"static publication", "escape", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"array publication", "array", "(Ljava/lang/Object;[Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"borrowed field read", "field", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"borrowed field write", "write", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"opaque receiver input", "opaque", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"reachable publication branch", "branch", "(Ljava/lang/Object;I)Ljava/lang/Class;", false},
		{"unreachable original branch", "branch", "(Ljava/lang/Object;I)Ljava/lang/Class;", true},
		{"recursive call", "cycle", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"confined holder", "retained", "(Ljava/lang/Object;)Ljava/lang/Class;", true},
		{"monitor entry", "locked", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"opaque native", "nativeKind", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
		{"typed handler", "typedHandler", "(Ljava/lang/Object;)Ljava/lang/Class;", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["BorrowStaticChild.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }
			input := constructorEffectValue{kind: 'L', receiver: true}
			arguments := []constructorEffectValue{input}
			switch row.name {
			case "wide ports":
				arguments = []constructorEffectValue{{kind: 'J'}, input, {kind: 'D'}, {kind: 'I'}, input}
			case "array publication":
				arguments = append(arguments, constructorEffectValue{kind: 'L'})
			case "reachable publication branch":
				arguments = append(arguments, constructorEffectValue{kind: 'I'})
			case "unreachable original branch":
				arguments = append(arguments, constructorEffectValue{kind: 'I', knownInt: true, intWord: 1})
			}
			remaining := 4096
			aliases := &constructorSelfStorageProof{freshReceiver: true}
			value, known := d.constructorReceiverBorrowedStaticMethod(&values.JavaClassMember{Name: "BorrowStaticProbe", Member: row.method, Description: row.descriptor}, core.OP_INVOKESTATIC, map[string]bool{"BorrowStaticChild\x00capture\x00Ljava/lang/Object;": true}, map[string]bool{}, &remaining, 0, aliases, arguments...)
			if known != row.want || known && (value.kind != 'L' || value.reachesReceiver() || value.knownNull || value.knownInt || !aliases.borrowedReceiver) {
				t.Fatalf("borrowed static proof=%v want=%v result=%+v", known, row.want, value)
			}
		})
	}
}

func TestNativeConstructorStaticBorrowRequiresExactOriginalDeclaration(t *testing.T) {
	originals := nativeCompileClasses(t, constructorStaticBorrowGuardFixture)
	variants := []string{"original", "missing input", "wrong width", "uninitialized input", "retained input", "null receiver contradiction", "integer receiver contradiction", "no borrowed receiver", "not fresh", "open finalizer", "unclosed heap alias", "interface owner", "inherited symbolic owner", "missing class", "foreign bytes", "wrong name", "wrong descriptor", "wrong opcode", "special opcode", "interface opcode", "constructor name", "initializer name", "changed static flag", "synchronized flag", "native flag", "abstract flag", "duplicate declaration", "invalid name encoding", "missing code", "duplicate code", "short locals", "short stack", "empty code", "active cycle", "depth", "fuel", "work", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for name, raw := range originals {
				files[name] = bytes.Clone(raw)
			}
			root, err := Parse(files["BorrowStaticChild.class"])
			if err != nil {
				t.Fatal(err)
			}
			helper, err := Parse(files["BorrowStaticProbe.class"])
			if err != nil {
				t.Fatal(err)
			}
			var target *MemberInfo
			var body *CodeAttribute
			for _, m := range helper.Methods {
				n, _ := helper.getUtf8(m.NameIndex)
				if n == "kind" {
					target = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							body = c
						}
					}
				}
			}
			if target == nil || body == nil {
				t.Fatal("original helper method")
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }
			member := &values.JavaClassMember{Name: "BorrowStaticProbe", Member: "kind", Description: "(Ljava/lang/Object;)Ljava/lang/Class;"}
			arguments := []constructorEffectValue{{kind: 'L', receiver: true}}
			aliases := &constructorSelfStorageProof{freshReceiver: true}
			opcode, remaining, depth := core.OP_INVOKESTATIC, 4096, 0
			active := map[string]bool{}
			switch variant {
			case "missing input":
				arguments = nil
			case "wrong width":
				arguments[0].kind = 'J'
			case "uninitialized input":
				arguments[0].allocation = 17
			case "retained input":
				arguments[0].receiver = false
				arguments[0].retainsReceiver = true
			case "null receiver contradiction":
				arguments[0].knownNull = true
			case "integer receiver contradiction":
				arguments[0].knownInt = true
			case "no borrowed receiver":
				arguments[0].receiver = false
			case "not fresh":
				aliases.freshReceiver = false
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "unclosed heap alias":
				aliases.selfStored = true
				aliases.referenceRead = true
			case "interface owner":
				member.Name = "BorrowStaticInterface"
			case "inherited symbolic owner":
				member.Name = "BorrowStaticInherited"
			case "missing class":
				delete(files, "BorrowStaticProbe.class")
			case "foreign bytes":
				files["BorrowStaticProbe.class"] = files["BorrowStaticHolder.class"]
			case "wrong name":
				member.Member = "missing"
			case "wrong descriptor":
				member.Description = "(Ljava/lang/Object;)I"
			case "wrong opcode":
				opcode = core.OP_INVOKEVIRTUAL
			case "special opcode":
				opcode = core.OP_INVOKESPECIAL
			case "interface opcode":
				opcode = core.OP_INVOKEINTERFACE
			case "constructor name":
				member.Member = "<init>"
			case "initializer name":
				member.Member = "<clinit>"
			case "changed static flag":
				target.AccessFlags &^= 8
			case "synchronized flag":
				target.AccessFlags |= 0x20
			case "native flag":
				target.AccessFlags |= 0x100
			case "abstract flag":
				target.AccessFlags |= 0x400
			case "duplicate declaration":
				helper.Methods = append(helper.Methods, target)
			case "invalid name encoding":
				target.NameIndex = helper.ThisClass
			case "missing code":
				target.Attributes = nil
			case "duplicate code":
				target.Attributes = append(target.Attributes, body)
			case "short locals":
				body.MaxLocals = 0
			case "short stack":
				body.MaxStack = 0
			case "empty code":
				body.Code = nil
			case "active cycle":
				active["static\x00"+member.Name+"\x00"+member.Member+"\x00"+member.Description] = true
			case "depth":
				depth = 17
			case "fuel":
				remaining = 1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "changed static flag" || variant == "synchronized flag" || variant == "native flag" || variant == "abstract flag" || variant == "duplicate declaration" || variant == "invalid name encoding" || variant == "missing code" || variant == "duplicate code" || variant == "short locals" || variant == "short stack" || variant == "empty code" {
				files["BorrowStaticProbe.class"] = helper.Bytes()
			}
			_, known := d.constructorReceiverBorrowedStaticMethod(member, opcode, map[string]bool{}, active, &remaining, depth, aliases, arguments...)
			if known != (variant == "original") {
				t.Fatalf("original static binding accepted=%v", known)
			}
		})
	}
}

func TestNativeConstructorStaticBorrowRechecksMutableRootAndOriginalInputs(t *testing.T) {
	originals := nativeCompileClasses(t, constructorStaticBorrowGuardFixture)
	for _, variant := range []string{"original", "changed root field", "changed root flags", "changed root object", "changed original helper"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(bytes.Clone(originals["BorrowStaticChild.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			reads, mutated := 0, false
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				raw, known := originals[name+".class"]
				if name == "BorrowStaticProbe" {
					reads++
					if !mutated {
						switch variant {
						case "changed root field":
							root.Fields[0].NameIndex = sourceBridgePoolString(t, root, "differentCapture")
							mutated = true
						case "changed root flags":
							root.AccessFlags |= 0x1000
							mutated = true
						case "changed root object":
							copy, err := Parse(bytes.Clone(root.Bytes()))
							if err != nil {
								t.Fatal(err)
							}
							d.obj = copy
							mutated = true
						}
					}
					if variant == "changed original helper" && reads > 1 {
						helper, err := Parse(bytes.Clone(raw))
						if err != nil {
							t.Fatal(err)
						}
						helper.AccessFlags |= 0x10
						raw = helper.Bytes()
						mutated = true
					}
				}
				return bytes.Clone(raw), known
			}
			accepted := d.constructorCaptureChainDoesNotObserve("BorrowStaticBase", "()V", map[string]bool{"BorrowStaticChild\x00capture\x00Ljava/lang/Object;": true})
			if accepted != (variant == "original") {
				t.Fatalf("mutable original admitted=%v reads=%d mutated=%v", accepted, reads, mutated)
			}
			if variant != "original" && !mutated {
				t.Fatal("mutation guard did not reach its original observation")
			}
		})
	}
}
