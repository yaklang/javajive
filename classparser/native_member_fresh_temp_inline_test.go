package javaclassparser

import (
	"context"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Original bytecode supplies the NEW/check/invokespecial packet. The source
// graph is built independently, so a pretty string or mutable Ref.Val cannot
// manufacture permission to move or delete an expression.
func TestNativeFreshTemporaryRestorationRequiresOriginalPacketAndUniqueUse(t *testing.T) {
	if os.Getenv("JAVA8_JAVAC") == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	source := `class FreshTemporaryPacket {class Branch {Branch(int n){}class Leaf {Leaf(int n){}}} Object make(int n){return new Branch(n).new Leaf(n);}}`
	files := nativePrivateEnumCompile(t, source, "FreshTemporaryPacket", "none")
	variants := []string{"original", "slot wrappers", "receiver call", "local result", "bad fresh NEW", "no fresh NEW", "bad fresh init", "bad fresh descriptor", "fresh alias", "fresh factory", "fresh type", "fresh call receiver", "no declaration", "parameter declaration", "THIS declaration", "opaque declaration", "bad check PC", "no check PC", "bad check owner", "bad check descriptor", "check arguments", "check static", "check special", "wrong check ref", "bad target NEW", "no target NEW", "bad target init", "no target init", "bad target owner", "bad target descriptor", "target not special", "target different receiver", "wrong enclosing ref", "effectful prefix", "conditional prefix", "intervening effect", "extra use", "reassignment", "duplicate target site", "opaque value", "cycle", "wrapper depth", "oversize AST", "missing declaration owner", "duplicate constructor", "synthetic constructor", "synthetic constructor attribute", "constructor argument count", "qualifier field argument", "qualifier call argument", "qualifier lambda argument", "work", "memory", "canceled", "preview", "failed family", "wrong method", "foreign context", "copied caller", "detached target", "target argument count"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["FreshTemporaryPacket.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			d.nativeMemberRoot = p
			d.FuncCtx = &class_context.ClassContext{ClassName: root.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(I)Ljava/lang/Object;"}
			d.wireNativeMemberSource()
			var plan *nativeMemberAllocation
			for _, a := range d.nativeMemberCalls["make(I)Ljava/lang/Object;"] {
				if a.freshEnclosing != nil {
					if plan != nil {
						t.Fatal("ambiguous packet")
					}
					plan = a
				}
			}
			if plan == nil || plan.checkPC < 0 {
				t.Fatal("original fresh enclosing packet")
			}
			proof := plan.freshEnclosing
			self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(root.GetClassName()))
			self.IsThis = true
			arg := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer("int"))
			arg.IsParam = true
			fresh := &values.NewExpression{JavaType: types.NewJavaClass(proof.owner), HasOriginPC: true, OriginPC: proof.newPC}
			freshCall := &values.FunctionCallExpression{Object: fresh, ClassName: proof.owner, FunctionName: "<init>", Descriptor: proof.descriptor, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: proof.invokePC, Arguments: []values.JavaValue{self, arg}}
			fresh.ConstructorCall = freshCall
			ref := values.NewJavaRef(utils.NewRootVariableId(), fresh, fresh.Type())
			decl := &statements.AssignStatement{LeftValue: ref, JavaValue: fresh, IsFirst: true}
			check := &values.FunctionCallExpression{Object: ref, ClassName: "java/lang/Object", FunctionName: "getClass", Descriptor: "()Ljava/lang/Class;", Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: plan.checkPC}
			target := &values.NewExpression{JavaType: types.NewJavaClass(plan.child.object.GetClassName()), HasOriginPC: true, OriginPC: plan.newPC}
			ctor := &values.FunctionCallExpression{Object: target, ClassName: plan.child.object.GetClassName(), FunctionName: "<init>", Descriptor: plan.descriptor, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: plan.invokePC, Arguments: []values.JavaValue{ref, arg}}
			target.ConstructorCall = ctor
			body := []statements.Statement{decl, &statements.ExpressionStatement{Expression: check}, &statements.ReturnStatement{JavaValue: target}}
			opaque := func() *values.CustomValue {
				return values.NewCustomValue(func(*class_context.ClassContext) string { t.Fatal("must not format for proof"); return "" }, func() types.JavaType { return fresh.Type() })
			}
			switch variant {
			case "slot wrappers":
				decl.LeftValue = values.NewSlotValue(ref, ref.Type())
				check.Object = values.NewSlotValue(ref, ref.Type())
				ctor.Arguments[0] = values.NewSlotValue(ref, ref.Type())
			case "receiver call":
				body[2] = &statements.ReturnStatement{JavaValue: &values.FunctionCallExpression{Object: target, ClassName: ctor.ClassName, FunctionName: "read", Kind: values.InvokeVirtual}}
			case "local result":
				body[2] = &statements.AssignStatement{LeftValue: values.NewJavaRef(utils.NewRootVariableId(), nil, target.Type()), JavaValue: target, IsFirst: true}
			case "bad fresh NEW":
				fresh.OriginPC++
			case "no fresh NEW":
				fresh.HasOriginPC = false
			case "bad fresh init":
				freshCall.OriginPC++
			case "bad fresh descriptor":
				freshCall.Descriptor = "()V"
			case "fresh alias":
				decl.JavaValue = values.NewJavaRef(utils.NewRootVariableId(), fresh, fresh.Type())
			case "fresh factory":
				decl.JavaValue = &values.FunctionCallExpression{ClassName: proof.owner, FunctionName: "make", Arguments: []values.JavaValue{fresh}, Kind: values.InvokeStatic, IsStatic: true}
			case "fresh type":
				fresh.JavaType = types.NewJavaClass("Foreign")
			case "fresh call receiver":
				freshCall.Object = ref
			case "no declaration":
				decl.IsFirst = false
			case "parameter declaration":
				ref.IsParam = true
			case "THIS declaration":
				ref.IsThis = true
			case "opaque declaration":
				ref.CustomValue = opaque()
			case "bad check PC":
				check.OriginPC++
			case "no check PC":
				check.HasOriginPC = false
			case "bad check owner":
				check.ClassName = proof.owner
			case "bad check descriptor":
				check.Descriptor = "()Ljava/lang/Object;"
			case "check arguments":
				check.Arguments = []values.JavaValue{arg}
			case "check static":
				check.IsStatic = true
			case "check special":
				check.Kind = values.InvokeSpecial
			case "wrong check ref":
				check.Object = values.NewJavaRef(utils.NewRootVariableId(), fresh, fresh.Type())
			case "bad target NEW":
				target.OriginPC++
			case "no target NEW":
				target.HasOriginPC = false
			case "bad target init":
				ctor.OriginPC++
			case "no target init":
				ctor.HasOriginPC = false
			case "bad target owner":
				ctor.ClassName = "Foreign"
			case "bad target descriptor":
				ctor.Descriptor = "()V"
			case "target not special":
				ctor.IsSpecialInvoke = false
			case "target different receiver":
				ctor.Object = fresh
			case "wrong enclosing ref":
				ctor.Arguments[0] = values.NewJavaRef(utils.NewRootVariableId(), fresh, fresh.Type())
			case "effectful prefix":
				body[2] = &statements.ReturnStatement{JavaValue: &values.FunctionCallExpression{ClassName: "Foreign", FunctionName: "apply", IsStatic: true, Kind: values.InvokeStatic, Arguments: []values.JavaValue{arg, target}}}
			case "conditional prefix":
				body[2] = &statements.ReturnStatement{JavaValue: &values.TernaryExpression{Condition: arg, TrueValue: target, FalseValue: values.JavaNull}}
			case "intervening effect":
				body = append(body[:2], &statements.ExpressionStatement{Expression: &values.FunctionCallExpression{ClassName: "Foreign", FunctionName: "mark", IsStatic: true}}, body[2])
			case "extra use":
				body = append(body, &statements.ExpressionStatement{Expression: ref})
			case "reassignment":
				body = append(body, &statements.AssignStatement{LeftValue: ref, JavaValue: values.JavaNull})
			case "duplicate target site":
				copy := *target
				body = append(body, &statements.ExpressionStatement{Expression: &copy})
			case "opaque value":
				body = append(body, &statements.ExpressionStatement{Expression: opaque()})
			case "cycle":
				v := &values.CastExpression{}
				v.Value = v
				body = append(body, &statements.ExpressionStatement{Expression: v})
			case "wrapper depth":
				var v values.JavaValue = ref
				for i := 0; i < 40; i++ {
					v = values.NewSlotValue(v, ref.Type())
				}
				ctor.Arguments[0] = v
			case "oversize AST":
				for i := 0; i < 8200; i++ {
					body = append(body, &statements.ExpressionStatement{Expression: arg})
				}
			case "missing declaration owner":
				delete(p.lexicalObjects, proof.owner)
			case "duplicate constructor", "synthetic constructor", "synthetic constructor attribute":
				object := p.lexicalObjects[proof.owner]
				for _, m := range object.Methods {
					n, _ := sourceBridgeUTF8(object, m.NameIndex)
					desc, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
					if n == "<init>" && desc == proof.descriptor {
						switch variant {
						case "duplicate constructor":
							object.Methods = append(object.Methods, m)
						case "synthetic constructor":
							m.AccessFlags |= 0x1000
						case "synthetic constructor attribute":
							m.Attributes = append(m.Attributes, &SyntheticAttribute{})
						}
						break
					}
				}
			case "constructor argument count":
				freshCall.Arguments = freshCall.Arguments[:1]
			case "qualifier field argument":
				freshCall.Arguments[1] = values.NewRefMember(self, "n", arg.Type())
			case "qualifier call argument":
				freshCall.Arguments[1] = &values.FunctionCallExpression{ClassName: "Foreign", FunctionName: "mark", IsStatic: true}
			case "qualifier lambda argument":
				freshCall.Arguments[1] = opaque()
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "preview":
				// Trace/preview rendering may not reject a complete family before the
				// source graph has finished binding names and restoring original packets.
				_, known := d.FuncCtx.SourceMemberAllocation("Foreign", "()V", -1, -1, nil)
				if known || p.failed {
					t.Fatal("preview committed source failure")
				}
				d.nativeSourceNamesReady = true
				_, known = d.FuncCtx.SourceMemberAllocation("Foreign", "()V", -1, -1, nil)
				if known || !p.failed {
					t.Fatal("committing renderer accepted missing original packet")
				}
				return
			case "failed family":
				p.failed = true
			case "foreign context":
				d.FuncCtx.ClassName = "Foreign"
			case "copied caller":
				d.obj, _ = Parse(append([]byte(nil), files["FreshTemporaryPacket.class"]...))
			case "detached target":
				delete(p.lexicalObjects, plan.child.object.GetClassName())
			case "target argument count":
				ctor.Arguments = ctor.Arguments[:1]
			case "wrong method":
				d.FuncCtx.FunctionName = "other"
			}
			before := append([]values.JavaValue(nil), ctor.Arguments...)
			got := d.recoverNativeFreshEnclosingTemps(body)
			want := variant == "original" || variant == "slot wrappers" || variant == "receiver call" || variant == "local result"
			if want {
				if len(got) != 1 || got[0] != body[2] || ctor.Arguments[0] != decl.JavaValue {
					t.Fatalf("original packet not restored: length=%d", len(got))
				}
				if ctor.Arguments[1] != arg || freshCall.Arguments[1] != arg {
					t.Fatal("argument identity changed")
				}
			} else {
				if len(got) != len(body) {
					t.Fatal("refused proof removed statements")
				}
				for i := range got {
					if got[i] != body[i] {
						t.Fatal("refused proof replaced statements")
					}
				}
				for i, v := range before {
					if ctor.Arguments[i] != v {
						t.Fatal("refused proof mutated operand")
					}
				}
			}
		})
	}
}
