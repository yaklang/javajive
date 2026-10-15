package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousPrivateCallDeclarationRequiresOriginalNonvirtualPacket(t *testing.T) {
	source := strings.NewReplacer("BODY", "return seed+first;", "OVERLOAD", "", "EFFECTS", "false", "EXPECTED", "seed+owner.firstValue()", "SEEDS", "0,1", "SUPPLIER", "java.util.function.IntSupplier", "METHOD", "getAsInt", "TYPE", "int", "FIRST", "7", "LAST", "13", "CHANGED", "17").Replace(anonymousPrivateCallFixture)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"PrivateCallOwner.java": source}, "none", "8")
	for _, variant := range []string{"original", "nil forest", "copied owner", "copied method", "removed physical method", "duplicated physical method", "copied physical method", "foreign unit", "missing packet", "wrong owner", "wrong target", "wrong descriptor", "wrong ordinal", "changed code", "wrong CP kind", "wrong receiver descriptor", "extra argument", "method formal", "static call", "inherited call", "generic result", "invented lexical owner", "public target", "static target", "duplicate target", "target signature", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["PrivateCallOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete anonymous call forest")
			}
			f := p.anonymousForest
			obj := f.objects["PrivateCallOwner$1"]
			var g *nativeMemberPrivateGetter
			for _, packet := range p.getters {
				if packet.owner == obj.GetClassName() && packet.call != nil {
					if g != nil {
						t.Fatal("ambiguous original private call")
					}
					g = packet
				}
			}
			if g == nil {
				t.Fatal("missing original private call packet")
			}
			method := g.method
			_, target := nativeMemberCallTarget(obj, g.field, g.fieldDescriptor, false, f.resolve, nil)
			if target == nil {
				t.Fatal("missing original nonvirtual target")
			}
			var code *CodeAttribute
			for _, attr := range method.Attributes {
				if c, ok := attr.(*CodeAttribute); ok {
					code = c
				}
			}
			if code == nil || len(code.Code) != 5 || code.Code[1] != core.OP_INVOKESPECIAL {
				t.Fatal("expected original receiver/private-call/return packet")
			}
			var work *workbudget.Budget
			switch variant {
			case "nil forest":
				f = nil
			case "removed physical method":
				for i, v := range obj.Methods {
					if v == method {
						obj.Methods = append(obj.Methods[:i], obj.Methods[i+1:]...)
						break
					}
				}
			case "duplicated physical method":
				obj.Methods = append(obj.Methods, method)
			case "copied physical method":
				for i, v := range obj.Methods {
					if v == method {
						x := *v
						obj.Methods[i] = &x
					}
				}
			case "copied owner":
				copy := *obj
				obj = &copy
			case "copied method":
				copy := *method
				method = &copy
			case "foreign unit":
				copy := *obj
				f.units[obj.GetClassName()].object = &copy
			case "missing packet":
				delete(p.getters, nativeMemberGetterKey(g.owner, g.name, g.descriptor))
			case "wrong owner":
				g.owner = "UnrelatedOwner"
			case "wrong target":
				g.field = "lastValue"
			case "wrong descriptor":
				g.fieldDescriptor = "()J"
			case "wrong ordinal":
				g.ordinal += 100
			case "changed code":
				code.Code[1] = core.OP_INVOKEVIRTUAL
			case "wrong CP kind":
				index := int(code.Code[2])<<8 | int(code.Code[3])
				original := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
				obj.ConstantPool[index-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: original.ConstantMemberrefInfo}
			case "wrong receiver descriptor":
				method.DescriptorIndex = sourceBridgePoolString(t, obj, "(LPrivateCallOwner;)I")
			case "extra argument":
				g.call.argumentCount++
			case "method formal":
				g.call.methodFormalCount++
			case "static call":
				g.call.static = true
			case "inherited call":
				g.call.inherited = true
			case "generic result":
				g.call.rawGeneric = true
			case "invented lexical owner":
				g.call.rawLexicalOwner = []string{"UnrelatedOwner"}
			case "public target":
				target.AccessFlags = 1
			case "static target":
				target.AccessFlags |= 8
			case "duplicate target":
				copy := *target
				obj.Methods = append(obj.Methods, &copy)
			case "target signature":
				target.Attributes = append(target.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "<T:Ljava/lang/Object;>()I")})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousPrivateCallAccessorDeclaration(f, obj, method, work); got != (variant == "original") {
				t.Fatalf("original nonvirtual declaration=%v", got)
			}
		})
	}
}

func TestNativeAnonymousPrivateCallSourceReopensPhysicalReceiverAndLexicalLookup(t *testing.T) {
	source := strings.NewReplacer("BODY", "return seed+first;", "OVERLOAD", "", "EFFECTS", "false", "EXPECTED", "seed+owner.firstValue()", "SEEDS", "0,1", "SUPPLIER", "java.util.function.IntSupplier", "METHOD", "getAsInt", "TYPE", "int", "FIRST", "7", "LAST", "13", "CHANGED", "17").Replace(anonymousPrivateCallFixture)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"PrivateCallOwner.java": source}, "none", "8")
	for _, variant := range []string{"original", "missing consumer", "missing packet", "wrong PC", "wrong method", "wrong descriptor", "wrong field PC", "wrong field name", "wrong operand PC", "missing original receiver", "ordinary parameter zero", "wrong receiver", "opaque receiver", "extra argument", "changed opcode", "public target", "shadow method", "unknown superclass", "foreign object", "foreign forest", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["PrivateCallOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete forest")
			}
			f := p.anonymousForest
			caller := f.objects["PrivateCallOwner$1$1"]
			var consumer *nativeAnonymousLexicalConsumer
			for _, c := range f.consumers[caller.GetClassName()]["getAsInt()I"] {
				if c.getter != nil && c.getter.call != nil {
					consumer = c
				}
			}
			if consumer == nil || consumer.read == nil {
				t.Fatal("original THIS-chain private consumer")
			}
			g := consumer.getter
			reader := z.nativeMemberReader(caller)
			reader.nativeAnonymousForest, reader.nativeMemberRoot = f, p
			ctx := &class_context.ClassContext{ClassName: caller.GetClassName(), FunctionName: "getAsInt", CurrentMethodDesc: "()I"}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(caller.GetClassName()))
			ref.IsThis, ref.IsParam = true, true
			if variant != "missing original receiver" {
				ref.MarkOriginalParameter(0)
			}
			if variant != "missing original receiver" && variant != "ordinary parameter zero" {
				ref.MarkOriginalReceiver()
			}
			var path []*nativeMemberLexicalRead
			for r := consumer.read; r != nil; r = r.prior {
				path = append(path, r)
			}
			var operand values.JavaValue = ref
			for i := len(path) - 1; i >= 0; i-- {
				r := path[i]
				field := values.NewRefMember(operand, r.field, types.NewJavaClass(r.descriptor[1:len(r.descriptor)-1]))
				field.HasOriginPC, field.OriginPC = true, r.pc
				operand = field
			}
			args := []any{operand}
			pc := consumer.pc
			reader.wireNativeMemberPrivateGetters(p, ctx)
			if ctx.SourcePrivateGetter == nil || p.failed {
				t.Fatal("original private source wiring")
			}
			switch variant {
			case "missing consumer":
				delete(f.consumers[caller.GetClassName()]["getAsInt()I"], pc)
			case "missing packet":
				delete(p.getters, nativeMemberGetterKey(g.owner, g.name, g.descriptor))
			case "wrong PC":
				pc++
			case "wrong method":
				ctx.FunctionName = "different"
			case "wrong descriptor":
				ctx.CurrentMethodDesc = "()J"
			case "wrong field PC":
				consumer.read.pc++
			case "wrong field name":
				consumer.read.field = "different"
			case "wrong operand PC":
				operand.(*values.RefMember).OriginPC++
			case "wrong receiver":
				ref.IsThis = false
			case "opaque receiver":
				original := operand
				args[0] = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return original.Type() })
			case "extra argument":
				args = append(args, values.JavaNull)
			case "changed opcode":
				for _, m := range caller.Methods {
					name, _ := sourceBridgeUTF8(caller, m.NameIndex)
					if name == "getAsInt" {
						for _, attr := range m.Attributes {
							if code, ok := attr.(*CodeAttribute); ok {
								code.Code[consumer.pc] = core.OP_INVOKEVIRTUAL
							}
						}
					}
				}
			case "public target":
				_, method := nativeMemberCallTarget(f.objects[g.owner], g.field, g.fieldDescriptor, false, f.resolve, nil)
				method.AccessFlags = 1
			case "shadow method":
				caller.Methods = append(caller.Methods, &MemberInfo{AccessFlags: 1, NameIndex: sourceBridgePoolString(t, caller, g.field), DescriptorIndex: sourceBridgePoolString(t, caller, "()I")})
			case "unknown superclass":
				caller.SuperClass = uint16(NewConstantPoolWithConstant(&caller.ConstantPool).AddNewClassInfo("UnknownPrivateCallParent"))
			case "foreign object":
				copy := *caller
				reader.obj = &copy
			case "foreign forest":
				p.anonymousForest = &nativeAnonymousForest{}
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			text, known := ctx.SourcePrivateGetter(g.owner, g.name, g.descriptor, pc, args, false)
			if known != (variant == "original") || known && !strings.HasPrefix(text, g.field+"()") {
				t.Fatalf("original physical lexical call=%v source=%q", known, text)
			}
		})
	}
}
