package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeConstructorBodyEnclosingParameterRequiresOriginalPacket(t *testing.T) {
	originals := nativeCompileClasses(t, constructorParameterChainFixture)
	for _, variant := range []string{"original", "ordinary method", "static method", "wrong load slot", "slot redefined", "missing constructor packet", "copied declaration", "missing lexical parent", "nonfinal child capture", "nonfinal ancestor capture", "branch into chain", "handler boundary inside chain", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			// Parse Code/opaque attributes borrow input: every mutant gets its own bytes.
			files := map[string][]byte{}
			for n, b := range originals {
				files[n] = append([]byte(nil), b...)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["ParameterChainOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("actual original family")
			}
			child := p.children["ParameterChainOwner$Layer$Leaf"]
			obj := child.object
			desc := "(LParameterChainOwner$Layer;J)V"
			ctor := child.constructors[desc]
			if ctor == nil {
				t.Fatal("actual physical constructor")
			}
			reads, known := nativeMemberLexicalReads(obj, p, nil)
			if !known {
				t.Fatal("actual original read proof")
			}
			var read *nativeMemberLexicalRead
			for _, r := range reads["<init>"+desc] {
				if r.parameterDescriptor == desc {
					read = r
					break
				}
			}
			if read == nil || read.pc <= ctor.delegatePC || read.parameterOwner != child.owner {
				t.Fatal("actual post-delegation slot-1 path")
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			var work *workbudget.Budget
			clearFinal := func(owner *nativeMemberClass) {
				for _, f := range owner.object.Fields {
					n, _ := sourceBridgeUTF8(owner.object, f.NameIndex)
					if n == owner.field {
						f.AccessFlags &^= 0x10
					}
				}
			}
			switch variant {
			case "ordinary method":
				method.NameIndex = sourceBridgePoolString(t, obj, "ordinary")
			case "static method":
				method.AccessFlags |= 8
			case "wrong load slot":
				code.Code[read.basePC] = core.OP_ALOAD_2
			case "slot redefined":
				code.Code[read.basePC] = core.OP_ASTORE_1
			case "missing constructor packet":
				delete(child.constructors, desc)
			case "copied declaration":
				child.object, e = Parse(append([]byte(nil), files["ParameterChainOwner$Layer$Leaf.class"]...))
				if e != nil {
					t.Fatal(e)
				}
			case "missing lexical parent":
				delete(p.children, child.owner)
			case "nonfinal child capture":
				clearFinal(child)
			case "nonfinal ancestor capture":
				clearFinal(p.children[child.owner])
			case "branch into chain":
				pc := len(code.Code)
				offset := read.pc - pc
				code.Code = append(code.Code, core.OP_GOTO, byte(offset>>8), byte(offset))
			case "handler boundary inside chain":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: uint16(read.pc), EndPc: uint16(len(code.Code)), HandlerPc: uint16(read.basePC)})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			result, ok := nativeMemberLexicalReads(obj, p, work)
			got := ok && result["<init>"+desc][read.pc] != nil && result["<init>"+desc][read.pc].parameterDescriptor == desc
			if got != (variant == "original") {
				t.Fatalf("post-delegation original parameter proof=%v", got)
			}
		})
	}
}

func TestNativeConstructorBodyEnclosingParameterIRCannotBorrowEqualSourceNames(t *testing.T) {
	for _, variant := range []string{"original", "same name different slot", "missing slot origin", "changed parameter value", "missing field PC", "wrong field PC", "wrong method", "different descriptor", "same spelling local", "foreign receiver type", "opaque receiver", "cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			owner := "IndependentRoot$Layer"
			desc := "(LIndependentRoot$Layer;LIndependentRoot$Layer;)V"
			ctx := &class_context.ClassContext{FunctionName: "<init>", CurrentMethodDesc: desc, LocalNames: map[*utils.VariableId]string{}}
			ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(owner))
			ref.IsParam = true
			if variant != "missing slot origin" {
				slot := 1
				if variant == "same name different slot" {
					slot = 2
				}
				ref.MarkOriginalParameter(slot)
			}
			ctx.LocalNames[ref.Id] = ctx.ShortTypeName(owner) + ".this"
			read := &nativeMemberLexicalRead{owner: owner, field: "this$0", descriptor: "LIndependentRoot;", pc: 19, parameterOwner: owner, parameterDescriptor: desc}
			field := values.NewRefMember(ref, read.field, types.NewJavaClass("IndependentRoot"))
			field.HasOriginPC = true
			field.OriginPC = 19
			var work *workbudget.Budget
			switch variant {
			case "changed parameter value":
				ref.Val = values.NewJavaLiteral(nil, types.NewJavaClass(owner))
			case "missing field PC":
				field.HasOriginPC = false
			case "wrong field PC":
				field.OriginPC++
			case "wrong method":
				ctx.FunctionName = "ordinary"
			case "different descriptor":
				ctx.CurrentMethodDesc = "(LIndependentRoot$Layer;)V"
			case "same spelling local":
				ref.IsParam = false
			case "foreign receiver type":
				ref.ResetVarType(types.NewJavaClass("Foreign"))
			case "opaque receiver":
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return ref.Type() })
			case "cycle":
				field.Object = field
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberLexicalReadOperand(field, read, work, ctx); got != (variant == "original") {
				t.Fatalf("original parameter operand=%v", got)
			}
		})
	}
}
