package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousLexicalCaptureChainRequiresExactOriginalIRPath(t *testing.T) {
	testNativeAnonymousLexicalCaptureChain(t, nativeAnonymousNestedFixture, "NestedOwner$1$1", false)
}

func TestNativeAnonymousNamedLexicalThisRequiresExactOriginalIRPath(t *testing.T) {
	testNativeAnonymousLexicalCaptureChain(t, nativeAnonymousNestedContextFixture("member-context-depth"), "NestedOwner$Layer$Middle$1$1", true)
}

func testNativeAnonymousLexicalCaptureChain(t *testing.T, fixture, leafName string, named bool) {
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "missing binding", "wrong binding", "missing named anchor", "missing final PC", "wrong final PC", "missing inner PC", "wrong field", "wrong field type", "foreign THIS type", "foreign receiver", "mutable alias", "opaque", "cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			d := z.nativeMemberReader(root)
			var forest *nativeAnonymousForest
			if named {
				members := d.planNativeMemberFamily()
				if members != nil && d.planNativeMemberAnonymousScopes(members) {
					forest = members.anonymousForest
				}
			} else {
				p := d.planNativeAnonymousForest()
				if p != nil {
					forest = p.forest
				}
			}
			if forest == nil {
				t.Fatal("original complete family")
			}
			leaf := forest.units[leafName]
			reads := forest.reads[leaf.object.GetClassName()]
			var read *nativeMemberLexicalRead
			for _, r := range reads["origin()Ljava/lang/Object;"] {
				if r.prior != nil {
					read = r
				}
			}
			if read == nil {
				t.Fatal("original two-hop path")
			}
			ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(leaf.object.GetClassName()))
			ref.IsThis = true
			var path []*nativeMemberLexicalRead
			for r := read; r != nil; r = r.prior {
				path = append(path, r)
			}
			var value values.JavaValue = ref
			for i := len(path) - 1; i >= 0; i-- {
				r := path[i]
				field := values.NewRefMember(value, r.field, types.NewJavaClass(r.descriptor[1:len(r.descriptor)-1]))
				field.OriginPC = r.pc
				field.HasOriginPC = true
				value = field
			}
			field := value.(*values.RefMember)
			inner := field.Object.(*values.RefMember)
			var work *workbudget.Budget
			switch variant {
			case "missing final PC":
				field.HasOriginPC = false
			case "wrong final PC":
				field.OriginPC++
			case "missing inner PC":
				inner.HasOriginPC = false
			case "wrong field":
				inner.Member = "different"
			case "wrong field type":
				value = values.NewRefMember(field.Object, field.Member, types.NewJavaClass("java.lang.String"))
				value.(*values.RefMember).HasOriginPC = true
				value.(*values.RefMember).OriginPC = field.OriginPC
			case "foreign THIS type":
				ref.ResetVarType(types.NewJavaClass("Foreign"))
			case "foreign receiver":
				ref.IsThis = false
			case "mutable alias":
				alias := values.NewJavaRef(&utils.VariableId{}, inner, inner.Type())
				field.Object = alias
			case "opaque":
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must never render") }, func() types.JavaType { return inner.Type() })
			case "cycle":
				inner.Object = field
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			irValid := variant == "original" || variant == "missing binding" || variant == "wrong binding" || variant == "missing named anchor"
			if got := nativeMemberLexicalReadOperand(value, read, work); got != irValid {
				t.Fatalf("IR path %v", got)
			}
			leafD := NewClassObjectDumper(leaf.object)
			leafD.Work = work
			leafD.nativeAnonymousForest = forest
			ctx := &class_context.ClassContext{ClassName: leaf.object.GetClassName()}
			want := "token"
			if named {
				want = ctx.ShortTypeName(read.descriptor[1:len(read.descriptor)-1]) + ".this"
			}
			leafD.nativeAnonymousBindings = map[string]string{nativeMemberCaptureIndexKey(read.owner, read.field): want}
			switch variant {
			case "missing binding":
				delete(leafD.nativeAnonymousBindings, nativeMemberCaptureIndexKey(read.owner, read.field))
			case "wrong binding":
				leafD.nativeAnonymousBindings[nativeMemberCaptureIndexKey(read.owner, read.field)] = "Foreign.this"
			case "missing named anchor":
				if named {
					delete(forest.members.children, read.descriptor[1:len(read.descriptor)-1])
				} else {
					delete(leafD.nativeAnonymousBindings, nativeMemberCaptureIndexKey(read.owner, read.field))
				}
			}
			leafD.FuncCtx = ctx
			ctx.FunctionName = "origin"
			ctx.CurrentMethodDesc = "()Ljava/lang/Object;"
			leafD.wireNativeAnonymousForestCaptures(ctx)
			final := value.(*values.RefMember)
			pc := -1
			if final.HasOriginPC {
				pc = final.OriginPC
			}
			source, known := ctx.SourceLexicalCapturedField(value, pc, final.Member)
			if known != (variant == "original") || leafD.nativeCaptureFailed != (variant != "original") || known && source != want {
				t.Fatalf("source binding %q known=%v failed=%v", source, known, leafD.nativeCaptureFailed)
			}

		})
	}
}

func TestNativeAnonymousLexicalReadsRequireWholeOriginalReceiverPath(t *testing.T) {
	testNativeAnonymousLexicalReceiverPath(t, nativeAnonymousNestedFixture, "NestedOwner$1$1", false)
}

func TestNativeAnonymousNamedLexicalThisRequiresWholeOriginalReceiverPath(t *testing.T) {
	testNativeAnonymousLexicalReceiverPath(t, nativeAnonymousNestedContextFixture("member-context-depth"), "NestedOwner$Layer$Middle$1$1", true)
}

func testNativeAnonymousLexicalReceiverPath(t *testing.T, fixture, leafName string, named bool) {
	files := nativeCompileClasses(t, fixture)
	for _, variant := range []string{"original", "foreign local receiver", "slot zero redefined", "capture store outside constructor", "branch into field", "catch starts inside chain", "catch ends inside chain", "handler inside chain", "foreign descriptor", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["NestedOwner.class"])
			d := z.nativeMemberReader(root)
			var forest *nativeAnonymousForest
			if named {
				p := d.planNativeMemberFamily()
				if p != nil && d.planNativeMemberAnonymousScopes(p) {
					forest = p.anonymousForest
				}
			} else {
				p := d.planNativeAnonymousForest()
				if p != nil {
					forest = p.forest
				}
			}
			if forest == nil {
				t.Fatal("original family")
			}
			obj := forest.units[leafName].object
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "origin" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original root method")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("original decode")
			}
			ops := constructorMotionOps(decoder)
			wantOps := 4
			if named {
				wantOps = 5
			}
			if len(ops) != wantOps {
				t.Fatal("original two-hop path")
			}
			first, second := int(ops[1].CurrentOffset), int(ops[2].CurrentOffset)
			var work *workbudget.Budget
			switch variant {
			case "foreign local receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "slot zero redefined":
				code.Code = append(code.Code, byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_0), byte(core.OP_RETURN))
			case "capture store outside constructor":
				code.Code = append(code.Code, byte(core.OP_ALOAD_0), byte(core.OP_ACONST_NULL), byte(core.OP_PUTFIELD), code.Code[first+1], code.Code[first+2], byte(core.OP_RETURN))
			case "branch into field":
				pc := len(code.Code) + 4
				code.Code = append(code.Code, byte(core.OP_ALOAD_0), byte(core.OP_GETFIELD), code.Code[first+1], code.Code[first+2], byte(core.OP_GOTO), 0, 0)
				binary.BigEndian.PutUint16(code.Code[pc+1:], uint16(int16(second-pc)))
			case "catch starts inside chain":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: uint16(first), EndPc: uint16(second + 3), HandlerPc: uint16(len(code.Code) - 1)}}
			case "catch ends inside chain":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(second), HandlerPc: uint16(len(code.Code) - 1)}}
			case "handler inside chain":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code) - 1), HandlerPc: uint16(second)}}
			case "foreign descriptor":
				index := int(binary.BigEndian.Uint16(code.Code[second+1:]))
				ref := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
				nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				pool := NewConstantPoolWithConstant(&obj.ConstantPool)
				nt.DescriptorIndex = uint16(pool.AddUtf8Info("Ljava/lang/String;"))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			known := nativeAnonymousForestCaptureReads(forest, work)
			if known != (variant == "original") {
				t.Fatalf("original path closure %v", known)
			}
		})
	}
}
