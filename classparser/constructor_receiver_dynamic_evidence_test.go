package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

const receiverDynamicEvidenceFixture = `class DynamicEvidenceInput{int result;DynamicEvidenceInput(Object token){java.util.function.Supplier<Object> s=()->token;result=s.get()==token?1:2;}}
class DynamicEvidenceReceiver{int result;DynamicEvidenceReceiver(Object token){java.util.function.Supplier<Object> s=()->this;result=s.get()==token?1:2;}}
class ReceiverDynamicBootstrap extends java.lang.invoke.ConstantCallSite{public ReceiverDynamicBootstrap(java.lang.invoke.MethodHandles.Lookup lookup,String name,java.lang.invoke.MethodType type,Object...args){super(java.lang.invoke.MethodHandles.dropArguments(java.lang.invoke.MethodHandles.constant(java.util.function.Supplier.class,null),0,type.parameterList()));}}`

func receiverDynamicEvidence(t *testing.T, files map[string][]byte, name string) (*ClassObject, *CodeAttribute, []*core.OpCode, int) {
	t.Helper()
	// Parsed Code/operand slices may retain the supplied bytes. Every mutation
	// control needs an owned input, or a changed-byte case corrupts later cases.
	obj, e := Parse(append([]byte(nil), files[name+".class"]...))
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "<init>" {
			continue
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if e = decoder.ParseOpcode(); e != nil {
					t.Fatal(e)
				}
				ops := constructorMotionOps(decoder)
				for i, op := range ops {
					if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
						return obj, code, ops, i
					}
				}
			}
		}
	}
	t.Fatal("original constructor dynamic site")
	return nil, nil, nil, 0
}

func TestNativeConstructorReceiverDynamicSiteRequiresOriginalTypedPacket(t *testing.T) {
	files := nativeCompileClasses(t, receiverDynamicEvidenceFixture)
	for _, variant := range []string{"original", "proof budget", "nil proof budget", "opaque bootstrap name", "constructor bootstrap", "constructor bootstrap with ordinary name", "static bootstrap with constructor name", "nil dumper", "nil class", "nil code", "nil opcode", "old class version", "wrong opcode", "wide opcode", "short packet", "reserved byte two", "reserved byte three", "offset", "changed bytes", "zero CP", "wrong CP kind", "missing name and type", "wrong name and type kind", "bad name CP", "empty name", "constructor name", "qualified name", "bad descriptor CP", "bad descriptor", "too many argument words", "missing bootstrap table", "duplicate bootstrap table", "bootstrap count mismatch", "bootstrap index", "nil bootstrap", "argument count mismatch", "missing handle", "wrong handle kind", "field handle", "nil handle", "missing method", "bad method descriptor", "zero argument CP", "nil argument CP", "unloadable argument", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, code, ops, index := receiverDynamicEvidence(t, files, "DynamicEvidenceInput")
			op := ops[index]
			site := obj.ConstantPool[int(core.Convert2bytesToInt(op.Data[:2]))-1].(*ConstantInvokeDynamicInfo)
			nt := obj.ConstantPool[site.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			var table *BootstrapMethodsAttribute
			for _, a := range obj.Attributes {
				if b, ok := a.(*BootstrapMethodsAttribute); ok {
					table = b
				}
			}
			if table == nil {
				t.Fatal("original bootstrap table")
			}
			boot := table.BootstrapMethods[site.BootstrapMethodAttrIndex]
			handle := obj.ConstantPool[boot.BootstrapMethodRef-1].(*ConstantMethodHandleInfo)
			member := obj.ConstantPool[handle.ReferenceIndex-1].(*ConstantMethodrefInfo)
			methodType := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			d := NewClassObjectDumper(obj)
			remaining := 512
			proofBudget := &remaining
			switch variant {
			case "proof budget":
				remaining = 1
			case "nil proof budget":
				proofBudget = nil
			case "opaque bootstrap name":
				methodType.NameIndex = sourceBridgePoolString(t, obj, "arbitraryBootstrap")
			case "constructor bootstrap":
				// REF_newInvokeSpecial supplies an opaque CallSite allocation;
				// no LambdaMetafactory owner/name participates in this proof.
				className := sourceBridgePoolString(t, obj, "ReceiverDynamicBootstrap")
				obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: className})
				classIndex := uint16(len(obj.ConstantPool))
				constructorName := sourceBridgePoolString(t, obj, "<init>")
				constructorDescriptor := sourceBridgePoolString(t, obj, "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)V")
				obj.ConstantPool = append(obj.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: constructorName, DescriptorIndex: constructorDescriptor})
				typeIndex := uint16(len(obj.ConstantPool))
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: classIndex, NameAndTypeIndex: typeIndex}})
				handle.ReferenceKind, handle.ReferenceIndex = 8, uint16(len(obj.ConstantPool))
			case "constructor bootstrap with ordinary name":
				handle.ReferenceKind = 8
			case "static bootstrap with constructor name":
				methodType.NameIndex = sourceBridgePoolString(t, obj, "<init>")
			case "nil dumper":
				d = nil
			case "nil class":
				obj = nil
			case "nil code":
				code = nil
			case "nil opcode":
				op = nil
			case "old class version":
				obj.MajorVersion = 50
			case "wrong opcode":
				copy := *op.Instr
				copy.OpCode = core.OP_INVOKESTATIC
				op.Instr = &copy
			case "wide opcode":
				op.IsWide = true
			case "short packet":
				op.Data = op.Data[:2]
			case "reserved byte two":
				op.Data[2] = 1
			case "reserved byte three":
				op.Data[3] = 1
			case "offset":
				op.CurrentOffset++
			case "changed bytes":
				code.Code[int(op.CurrentOffset)] = core.OP_NOP
			case "zero CP":
				op.Data[0], op.Data[1] = 0, 0
			case "wrong CP kind":
				obj.ConstantPool[int(core.Convert2bytesToInt(op.Data[:2]))-1] = &ConstantUtf8Info{Value: "not a dynamic call site"}
			case "missing name and type":
				site.NameAndTypeIndex = 0
			case "wrong name and type kind":
				obj.ConstantPool[site.NameAndTypeIndex-1] = &ConstantUtf8Info{Value: "wrong"}
			case "bad name CP":
				nt.NameIndex = 0
			case "empty name":
				nt.NameIndex = sourceBridgePoolString(t, obj, "")
			case "constructor name":
				nt.NameIndex = sourceBridgePoolString(t, obj, "<init>")
			case "qualified name":
				nt.NameIndex = sourceBridgePoolString(t, obj, "other/name")
			case "bad descriptor CP":
				nt.DescriptorIndex = 0
			case "bad descriptor":
				nt.DescriptorIndex = sourceBridgePoolString(t, obj, "(V)V")
			case "too many argument words":
				nt.DescriptorIndex = sourceBridgePoolString(t, obj, "("+strings.Repeat("J", 128)+")V")
			case "missing bootstrap table":
				for i, a := range obj.Attributes {
					if a == table {
						obj.Attributes = append(obj.Attributes[:i], obj.Attributes[i+1:]...)
						break
					}
				}
			case "duplicate bootstrap table":
				obj.Attributes = append(obj.Attributes, table)
			case "bootstrap count mismatch":
				table.NumBootstrapMethods++
			case "bootstrap index":
				site.BootstrapMethodAttrIndex = uint16(len(table.BootstrapMethods))
			case "nil bootstrap":
				table.BootstrapMethods[site.BootstrapMethodAttrIndex] = nil
			case "argument count mismatch":
				boot.NumBootstrapArguments++
			case "missing handle":
				boot.BootstrapMethodRef = 0
			case "wrong handle kind":
				obj.ConstantPool[boot.BootstrapMethodRef-1] = &ConstantUtf8Info{Value: "wrong"}
			case "field handle":
				handle.ReferenceKind = 1
			case "nil handle":
				obj.ConstantPool[boot.BootstrapMethodRef-1] = (*ConstantMethodHandleInfo)(nil)
			case "missing method":
				handle.ReferenceIndex = 0
			case "bad method descriptor":
				methodType.DescriptorIndex = sourceBridgePoolString(t, obj, "not a descriptor")
			case "zero argument CP":
				boot.BootstrapArguments[0] = 0
			case "nil argument CP":
				obj.ConstantPool[boot.BootstrapArguments[0]-1] = (*ConstantMethodTypeInfo)(nil)
			case "unloadable argument":
				obj.ConstantPool[boot.BootstrapArguments[0]-1] = &ConstantUtf8Info{Value: "not loadable"}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "opaque bootstrap name" || variant == "constructor bootstrap"
			if _, got := d.constructorReceiverDynamicDescriptor(obj, code, op, proofBudget); got != want {
				t.Fatalf("original dynamic site=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeConstructorReceiverDynamicDoesNotBorrowReceiverOrOriginalBody(t *testing.T) {
	files := nativeCompileClasses(t, receiverDynamicEvidenceFixture)
	for _, variant := range []string{"original", "receiver capture", "forged receiver load", "changed opcode view", "missing instruction", "stack", "open finalizer", "chain budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			name := "DynamicEvidenceInput"
			if variant == "receiver capture" || variant == "forged receiver load" {
				name = "DynamicEvidenceReceiver"
			}
			obj, code, ops, index := receiverDynamicEvidence(t, files, name)
			d := NewClassObjectDumper(obj)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			d.constructorReceiverFinalizerSilent = true
			remaining := 512
			switch variant {
			case "forged receiver load":
				copy := *ops[index-1].Instr
				if copy.OpCode != core.OP_ALOAD_0 {
					t.Fatal("original receiver factory operand")
				}
				copy.OpCode = core.OP_ALOAD_1
				ops[index-1].Instr = &copy
			case "changed opcode view":
				copy := *ops[index-1].Instr
				copy.OpCode = core.OP_ACONST_NULL
				ops[index-1].Instr = &copy
			case "missing instruction":
				ops = append(ops[:index-1], ops[index:]...)
			case "stack":
				code.MaxStack = 0
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
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
			writes := map[string]bool{name + "\x00unseenCapture\x00Ljava/lang/Object;": true}
			if got := d.constructorReceiverEffects(obj, code, ops, "(Ljava/lang/Object;)V", writes, map[string]bool{}, &remaining, 0); got != (variant == "original") {
				t.Fatalf("dynamic receiver observation=%v; variant=%s", got, variant)
			}
		})
	}
}
