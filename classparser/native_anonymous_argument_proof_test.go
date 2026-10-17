package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousPrivateArgumentSourceRequiresOriginalProducerTree(t *testing.T) {
	source := strings.NewReplacer("ARG", "ArgumentEffects.pick(value)", "EFFECTS", "true").Replace(nativePrivateArgumentFixture)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ArgumentOwner.java": source}, "none", "8")
	for _, variant := range []string{"original", "other identical receiver read", "receiver field", "receiver descriptor", "receiver CP kind", "consumer PC", "source call PC", "source call name", "source call owner", "source call descriptor", "source call kind", "source call static", "source call special", "missing call origin", "missing argument", "opaque argument", "missing qualifier", "wrong qualifier", "effectful qualifier", "evaluated class qualifier", "source field PC", "source field name", "missing field witness", "wrong THIS seed", "ordinary parameter zero", "source THIS mutation", "argument phi", "missing SSA producer", "original call opcode", "original CP kind", "source method", "stale argument certificate", "graph budget", "allocation budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["ArgumentOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original forest")
			}
			f := p.anonymousForest
			caller := f.objects["ArgumentOwner$1$1"]
			var consumer *nativeAnonymousLexicalConsumer
			for _, c := range f.consumers[caller.GetClassName()]["getAsInt()I"] {
				if c.getter != nil && c.getter.call != nil {
					consumer = c
				}
			}
			if consumer == nil || len(consumer.argumentOrigins) != 1 {
				t.Fatal("original argument certificate")
			}
			reader := z.nativeMemberReader(caller)
			reader.nativeAnonymousForest, reader.nativeMemberRoot = f, p
			ctx := &class_context.ClassContext{ClassName: caller.GetClassName(), FunctionName: "getAsInt", CurrentMethodDesc: "()I", InvocationMetadata: reader.buildInvocationMetadata()}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range caller.Methods {
				name, _ := sourceBridgeUTF8(caller, m.NameIndex)
				if name == "getAsInt" {
					method = m
					for _, a := range m.Attributes {
						if ca, ok := a.(*CodeAttribute); ok {
							code = ca
						}
					}
				}
			}
			ir, fn, known := reader.nativeOriginalMethodSnapshot(method, code)
			if !known {
				t.Fatal("original frames")
			}
			records, known := nativeAnonymousArgumentRecords(fn, nil)
			if !known {
				t.Fatal("original records")
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(caller.GetClassName()))
			ref.IsThis, ref.IsParam = true, true
			if variant != "wrong THIS seed" {
				ref.MarkOriginalParameter(0)
				if variant != "ordinary parameter zero" {
					ref.MarkOriginalReceiver()
				} else {
					ref.IsThis = false
				}
			}
			// Construct a decoded tree directly from the independently compiled
			// method. The mutations below alter one witness at a time.
			var build func(ssabuild.Origin) values.JavaValue
			build = func(origin ssabuild.Origin) values.JavaValue {
				if origin.Kind == ssabuild.OriginParam && origin.Slot == 0 {
					return ref
				}
				ins, ok := ir.InstrByID(methodir.InstrID(origin.PC))
				if !ok {
					t.Fatal("missing fixture instruction")
				}
				record := records[int(origin.PC)]
				switch ins.Opcode {
				case core.OP_GETFIELD:
					typ, err := types.ParseDescriptor(ins.Desc)
					if err != nil {
						t.Fatal(err)
					}
					v := values.NewRefMember(build(record.Uses[0]), ins.Member, typ)
					v.HasOriginPC, v.OriginPC = true, int(ins.PC)
					v.MarkOriginalFieldRead(values.NewJavaClassMember(ins.Class, ins.Member, ins.Desc, typ), int(ins.PC))
					return v
				case core.OP_INVOKESTATIC:
					mt, err := types.ParseMethodDescriptor(ins.Desc)
					if err != nil {
						t.Fatal(err)
					}
					v := &values.FunctionCallExpression{ClassName: ins.Class, FunctionName: ins.Member, Descriptor: ins.Desc, Kind: values.InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: int(ins.PC), FuncType: mt.FunctionType(), Object: values.NewJavaClassValue(types.NewJavaClass(ins.Class))}
					for _, use := range record.Uses {
						v.Arguments = append(v.Arguments, build(use))
					}
					return v
				default:
					t.Fatalf("unexpected fixture producer %s", ins.Name)
					return nil
				}
			}
			receiver := build(records[consumer.pc].Uses[0])
			argument := build(consumer.argumentOrigins[0]).(*values.FunctionCallExpression)
			field := argument.Arguments[0].(*values.RefMember)
			args := []any{receiver, argument}
			switch variant {
			case "other identical receiver read":
				consumer.read.pc = int(field.Object.(*values.RefMember).OriginPC)
			case "receiver field":
				consumer.read.field = "other"
			case "receiver descriptor":
				consumer.read.descriptor = "Ljava/lang/Object;"
			case "receiver CP kind":
				index := int(code.Code[consumer.read.pc+1])<<8 | int(code.Code[consumer.read.pc+2])
				original := caller.ConstantPool[index-1].(*ConstantFieldrefInfo)
				caller.ConstantPool[index-1] = &ConstantMethodrefInfo{ConstantMemberrefInfo: original.ConstantMemberrefInfo}
			case "consumer PC":
				consumer.pc++
			case "source call PC":
				argument.OriginPC++
			case "source call name":
				argument.FunctionName = "other"
			case "source call owner":
				argument.ClassName = "OtherEffects"
			case "source call descriptor":
				argument.Descriptor = "(Ljava/lang/Object;)LArgumentBox;"
			case "source call kind":
				argument.Kind = values.InvokeVirtual
			case "source call static":
				argument.IsStatic = false
			case "source call special":
				argument.IsSpecialInvoke = true
			case "missing qualifier":
				argument.Object = nil
			case "wrong qualifier":
				argument.Object = values.NewJavaClassValue(types.NewJavaClass("OtherEffects"))
			case "effectful qualifier":
				argument.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaClass("ArgumentEffects") })
			case "evaluated class qualifier":
				argument.Object.(*values.JavaClassValue).HasOriginPC = true
			case "missing call origin":
				argument.HasOriginPC = false
			case "missing argument":
				argument.Arguments = nil
			case "opaque argument":
				args[1] = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return argument.Type() })
			case "source field PC":
				field.OriginPC++
			case "source field name":
				field.Member = "other"
			case "missing field witness":
				unproved := values.NewRefMember(field.Object, field.Member, field.Type())
				unproved.HasOriginPC, unproved.OriginPC = true, field.OriginPC
				argument.Arguments[0] = unproved
			case "source THIS mutation":
				ref.IsThis = false
			case "argument phi":
				consumer.argumentOrigins[0].Kind = ssabuild.OriginPhi
			case "missing SSA producer":
				consumer.argumentOrigins[0].PC++
			case "original call opcode":
				code.Code[argument.OriginPC] = core.OP_INVOKEVIRTUAL
			case "original CP kind":
				index := int(code.Code[argument.OriginPC+1])<<8 | int(code.Code[argument.OriginPC+2])
				original := caller.ConstantPool[index-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[index-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: original.ConstantMemberrefInfo}
			case "source method":
				ctx.FunctionName = "other"
			case "stale argument certificate":
				consumer.argumentOrigins[0] = records[consumer.pc].Uses[0]
			case "graph budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			closed := nativeAnonymousLexicalConsumerSourceClosed(reader, consumer, ctx, reader.Work)
			_, rendered := nativeAnonymousPrivateArgumentsSource(reader, consumer, args, ctx)
			if variant == "receiver CP kind" && closed {
				t.Fatal("receiver-only CP category may not borrow argument rejection")
			}
			if got := closed && rendered; got != (variant == "original") {
				t.Fatalf("original receiver and argument DAG=%v (closed=%v,source=%v)", got, closed, rendered)
			}
		})
	}
}
