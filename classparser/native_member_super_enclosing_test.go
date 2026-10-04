package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberSuperEnclosingPathRequiresOriginalForest(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeMemberAncestorSuperFixture, "none")
	for _, variant := range []string{"original", "foreign family child", "foreign family parent", "missing ancestor", "static ancestor", "wrong capture name", "wrong capture descriptor", "nonfinal capture", "wrong load slot", "foreign field owner", "wrong field name", "wrong field descriptor", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["AncestorMemberOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original complete forest")
			}
			child, parent := p.children["AncestorMemberOwner$Layer$Child"], p.children["AncestorMemberOwner$Parent"]
			obj, err := Parse(append([]byte(nil), files["AncestorMemberOwner$Layer$Child.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			child.object = obj
			var code *CodeAttribute
			var ctor *nativeMemberConstructor
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if name == "<init>" {
					ctor = child.constructors[desc]
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(d)
			start := -1
			for i, op := range ops {
				if int(op.CurrentOffset) == ctor.capturePC {
					start = i + 1
				}
			}
			if start < 0 {
				t.Fatal("original capture")
			}
			field := constructorMotionMember(obj, ops[start+2], core.OP_GETFIELD)
			if field == nil {
				t.Fatal("original lexical field")
			}
			ref := obj.ConstantPool[core.Convert2bytesToInt(ops[start+2].Data)-1].(*ConstantFieldrefInfo)
			nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			var work *workbudget.Budget
			switch variant {
			case "foreign family child":
				delete(p.children, obj.GetClassName())
			case "foreign family parent":
				delete(p.children, parent.object.GetClassName())
			case "missing ancestor":
				delete(p.children, child.owner)
			case "static ancestor":
				p.children[child.owner].static = true
			case "wrong capture name":
				p.children[child.owner].field = "different"
			case "wrong capture descriptor", "nonfinal capture":
				ancestor := p.children[child.owner]
				for _, f := range ancestor.object.Fields {
					name, _ := sourceBridgeUTF8(ancestor.object, f.NameIndex)
					if name == ancestor.field {
						if variant == "nonfinal capture" {
							f.AccessFlags &^= 0x10
						} else {
							f.DescriptorIndex = sourceBridgePoolString(t, ancestor.object, "Ljava/lang/Object;")
						}
					}
				}
			case "wrong load slot":
				ops[start+1].Instr = core.InstrInfos[core.OP_ALOAD_2]
			case "foreign field owner":
				ref.ClassIndex = obj.ThisClass
			case "wrong field name":
				nt.NameIndex = sourceBridgePoolString(t, obj, "different")
			case "wrong field descriptor":
				nt.DescriptorIndex = sourceBridgePoolString(t, obj, "Ljava/lang/Object;")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			path, known := nativeMemberSuperEnclosingPath(child, parent, p, ops, start, work)
			if known != (variant == "original") {
				t.Fatalf("path=%#v proved=%v", path, known)
			}
			if variant == "original" {
				reader := z.nativeMemberReader(root)
				params, _, err := callbinding.Descriptor(ctor.descriptor)
				if err != nil {
					t.Fatal(err)
				}
				for _, mode := range []string{"original", "field PC", "cast before field", "cast after field", "different slot", "missing path", "cyclic path"} {
					t.Run(mode, func(t *testing.T) {
						packet := make([]*core.OpCode, len(ops))
						for i, op := range ops {
							copy := *op
							packet[i] = &copy
						}
						proof := *path
						wanted := &proof
						switch mode {
						case "field PC":
							proof.pc++
						case "cast before field", "cast after field":
							name := child.owner
							if mode == "cast after field" {
								name = parent.owner
							}
							idx := sourceBridgePoolString(t, obj, name)
							obj.ConstantPool = append(obj.ConstantPool, &ConstantClassInfo{NameIndex: idx})
							index := len(obj.ConstantPool)
							cast := &core.OpCode{Instr: core.InstrInfos[core.OP_CHECKCAST], Data: []byte{byte(index >> 8), byte(index)}, CurrentOffset: 60000}
							at := start + 2
							if mode == "cast after field" {
								at++
							}
							packet = append(append(append([]*core.OpCode{}, packet[:at]...), cast), packet[at:]...)
						case "different slot":
							packet[start+1].Instr = core.InstrInfos[core.OP_ALOAD_2]
						case "missing path":
							wanted = nil
						case "cyclic path":
							proof.prior = &proof
						}
						next, call := constructorMotionDelegationEnclosing(obj, packet, start, params, constructorParameterSlots(params), reader.buildInvocationMetadata(), wanted, 1)
						if got := next > 0 && call != nil; got != (mode == "original") {
							t.Fatalf("origin-preserving enclosing proof=%v next=%d", got, next)
						}
					})
				}
			}

		})
	}
}

func TestNativeMemberSuperEnclosingIRRequiresOriginalPathIdentity(t *testing.T) {
	for _, variant := range []string{"original", "wrong field PC", "missing PC", "foreign member", "wrong type", "foreign receiver", "not parameter", "wrong lexical alias", "opaque base", "cycle", "wrong method", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			owner := "IndependentRoot$Layer"
			ctx := &class_context.ClassContext{FunctionName: "<init>"}
			ctx.LocalNames = map[*utils.VariableId]string{}
			ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(owner))
			ref.IsParam = true
			ctx.LocalNames[ref.Id] = ctx.ShortTypeName(owner) + ".this"
			read := &nativeMemberLexicalRead{owner: owner, field: "this$0", descriptor: "LIndependentRoot;", pc: 19, parameterOwner: owner}
			field := values.NewRefMember(ref, read.field, types.NewJavaClass("IndependentRoot"))
			field.HasOriginPC = true
			field.OriginPC = 19
			var work *workbudget.Budget
			switch variant {
			case "wrong field PC":
				field.OriginPC++
			case "missing PC":
				field.HasOriginPC = false
			case "foreign member":
				field.Member = "other"
			case "wrong type":
				field = values.NewRefMember(ref, read.field, types.NewJavaClass("java.lang.String"))
				field.HasOriginPC = true
				field.OriginPC = 19
			case "foreign receiver":
				ref.ResetVarType(types.NewJavaClass("Foreign"))
			case "not parameter":
				ref.IsParam = false
			case "wrong lexical alias":
				ctx.LocalNames[ref.Id] = "other"
			case "opaque base":
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must never render") }, func() types.JavaType { return ref.Type() })
			case "cycle":
				field.Object = field
			case "wrong method":
				ctx.FunctionName = "ordinary"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				context, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(context, workbudget.Limits{})
			}
			if got := nativeMemberLexicalReadOperand(field, read, work, ctx); got != (variant == "original") {
				t.Fatalf("source path=%v", got)
			}
		})
	}
}
