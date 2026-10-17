package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

const localMemberOriginGuardSource = `class PacketOwner {class Item{Item(Object token){}}class Layer{Object make(final Object token){class Local{Object get(){return new Item(token);}}return new Local();}}}`

func TestNativeMethodLocalMemberEnclosingRequiresFreshOriginalPacket(t *testing.T) {
	files := nativeCompileClasses(t, localMemberOriginGuardSource)
	for _, mode := range []string{"original", "unique local", "unsealed receiver", "unsealed first read", "unsealed last read", "foreign receiver", "foreign receiver type", "wrong result type", "wrong read origin", "wrong field", "wrong original field", "missing local", "duplicate local role", "foreign local object", "foreign lexical object", "changed declaration", "changed packet", "changed owner bytes", "changed caller bytes", "missing input seal", "wrong method", "wrong invoke", "wrong new", "wrong descriptor", "wrong null protocol", "wrong implicit protocol", "foreign parameter role", "foreign anonymous role", "wrong path", "foreign child name", "foreign child field", "failed family", "nil local receipt", "nil child receipt", "no local declaration", "late local declaration", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["PacketOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			const current = "PacketOwner$Layer$1Local"
			local := p.methodLocals[current]
			if local == nil {
				t.Fatal("original local")
			}
			reader := z.nativeMemberReader(local.object)
			plans, known := reader.nativeMemberAllocations(p)
			if !known {
				t.Fatal("original allocation")
			}
			var plan *nativeMemberAllocation
			for _, a := range plans["get()Ljava/lang/Object;"] {
				plan = a
			}
			if plan == nil || plan.methodLocalEnclosing == nil || plan.methodLocalEnclosing.read.prior == nil {
				t.Fatal("original two-edge source-role receipt")
			}
			proof := plan.methodLocalEnclosing
			receiver := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(current))
			receiver.IsParam = true
			receiver.MarkOriginalParameter(0)
			receiver.IsThis = true
			if mode != "unsealed receiver" {
				receiver.MarkOriginalReceiver()
			}
			path := []*nativeMemberLexicalRead{}
			for r := proof.read; r != nil; r = r.prior {
				path = append(path, r)
			}
			var value values.JavaValue = receiver
			var first, last *values.RefMember
			for i := len(path) - 1; i >= 0; i-- {
				r := path[i]
				field := values.NewRefMember(value, r.field, types.NewJavaClass(r.descriptor[1:len(r.descriptor)-1]))
				field.HasOriginPC = true
				field.OriginPC = r.pc
				if !(mode == "unsealed first read" && i == len(path)-1) && !(mode == "unsealed last read" && i == 0) {
					original := r.field
					if mode == "wrong original field" && i == 0 {
						original = "foreign"
					}
					field.MarkOriginalFieldRead(&values.JavaClassMember{Name: r.owner, Member: original, Description: r.descriptor}, r.pc)
				}
				if first == nil {
					first = field
				}
				last = field
				value = field
			}
			call := &values.FunctionCallExpression{ClassName: plan.child.object.GetClassName(), FunctionName: "<init>", Descriptor: plan.descriptor, HasOriginPC: true, OriginPC: plan.invokePC, Arguments: []values.JavaValue{value}}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass(call.ClassName), ConstructorCall: call, HasOriginPC: true, OriginPC: plan.newPC}
			body := []statements.Statement{&statements.ReturnStatement{JavaValue: allocation}}
			var operand any = value
			var work *workbudget.Budget
			switch mode {
			case "unique local", "no local declaration", "late local declaration":
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, value.Type())
				ref.Id.SetName("retained")
				operand = ref
				call.Arguments[0] = ref
				declaration := statements.NewAssignStatement(ref, value, true)
				if mode == "unique local" {
					body = append([]statements.Statement{declaration}, body...)
				} else if mode == "late local declaration" {
					body = append(body, declaration)
				}
			case "foreign receiver":
				receiver.IsThis = false
			case "foreign receiver type":
				receiver.ResetVarType(types.NewJavaClass("PacketOwner"))
			case "wrong result type":
				last.JavaType = types.NewJavaClass("java/lang/Object")
			case "wrong read origin":
				last.OriginPC++
			case "wrong field":
				first.Member = "other"
			case "missing local":
				delete(p.methodLocals, current)
			case "duplicate local role":
				p.children[current] = &nativeMemberClass{object: local.object}
			case "foreign local object":
				local.object, _ = Parse(files[current+".class"])
			case "foreign lexical object":
				p.lexicalObjects[current], _ = Parse(files[current+".class"])
			case "changed declaration":
				local.owner.method = "other"
			case "changed packet":
				local.constructor.capturePCs[local.constructor.enclosingField]++
			case "changed owner bytes":
				p.lexicalObjects[p.owner].AccessFlags ^= 0x10
			case "changed caller bytes":
				proof.caller.AccessFlags ^= 0x10
			case "missing input seal":
				delete(proof.inputs, p.lexicalObjects[p.owner])
			case "wrong method":
				proof.methodKey = "other()Ljava/lang/Object;"
			case "wrong invoke":
				plan.invokePC++
			case "wrong new":
				plan.newPC++
			case "wrong descriptor":
				plan.descriptor = "()V"
			case "wrong null protocol":
				plan.checkPC++
			case "wrong implicit protocol":
				plan.implicitEnclosing = !plan.implicitEnclosing
			case "foreign parameter role":
				plan.enclosingParameter = &nativeMemberEnclosingParameter{}
			case "foreign anonymous role":
				plan.anonymousEnclosingRead = &nativeMemberLexicalRead{}
			case "wrong path":
				proof.read.prior = nil
			case "foreign child name":
				plan.child.name = "Other"
			case "foreign child field":
				plan.child.field = "other"
			case "failed family":
				p.failed = true
			case "nil local receipt":
				proof.local = nil
			case "nil child receipt":
				proof.child = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := mode == "original" || mode == "unique local"
			if got := nativeMemberLexicalEnclosingOperand(operand, plan, p, current, body, work); got != want {
				t.Fatalf("source permission=%v want=%v", got, want)
			}
		})
	}
}

func TestNativeMethodLocalMemberAllocationKeepsOriginalReceiverBarriers(t *testing.T) {
	files := nativeCompileClasses(t, localMemberOriginGuardSource)
	for _, mode := range []string{"original", "foreign base", "slot zero write", "capture store", "static capture", "budget", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["PacketOwner.class"])
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("family")
			}
			obj := p.methodLocals["PacketOwner$Layer$1Local"].object
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "get" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original get")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("decode")
			}
			ops := constructorMotionOps(decoder)
			first := -1
			for _, op := range ops {
				if field := constructorMotionMember(obj, op, core.OP_GETFIELD); field != nil && field.Name == obj.GetClassName() && field.Member == p.methodLocals[obj.GetClassName()].constructor.enclosingField {
					first = int(op.CurrentOffset)
					break
				}
			}
			if first < 0 {
				t.Fatal("original hidden owner")
			}
			reader := z.nativeMemberReader(obj)
			switch mode {
			case "foreign base":
				code.Code[first-1] = byte(core.OP_ALOAD_1)
			case "slot zero write":
				code.Code = append(code.Code, byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_0), byte(core.OP_RETURN))
			case "capture store":
				code.Code = append(code.Code, byte(core.OP_ALOAD_0), byte(core.OP_ACONST_NULL), byte(core.OP_PUTFIELD), code.Code[first+1], code.Code[first+2], byte(core.OP_RETURN))
			case "static capture":
				code.Code[first] = byte(core.OP_GETSTATIC)
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, known := reader.nativeMemberAllocations(p)
			if known != (mode == "original") {
				t.Fatalf("allocation closure=%v", known)
			}
		})
	}
}

func TestNativeMethodLocalMemberSourceNameKeepsOriginalMethodAndTypeBinding(t *testing.T) {
	files := nativeCompileClasses(t, localMemberOriginGuardSource)
	for _, mode := range []string{"simple", "qualified", "shadowed root", "no denotable anchor", "wrong source method", "wrong source descriptor", "copied method", "foreign method", "missing current local", "failed family", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["PacketOwner.class"])
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			local := p.methodLocals["PacketOwner$Layer$1Local"]
			if local == nil {
				t.Fatal("local role")
			}
			c := z.nativeMemberReader(local.object)
			c.nativeMemberRoot = p
			c.nativeMethodLocalCurrent = local
			plans, known := c.nativeMemberAllocations(p)
			if !known {
				t.Fatal("original allocation")
			}
			var plan *nativeMemberAllocation
			for _, candidate := range plans["get()Ljava/lang/Object;"] {
				plan = candidate
			}
			if plan == nil {
				t.Fatal("plan")
			}
			for _, method := range local.object.Methods {
				name, _ := sourceBridgeUTF8(local.object, method.NameIndex)
				if name == "get" {
					c.CurrentMethod = method
				}
			}
			ctx := &class_context.ClassContext{ClassName: local.object.GetClassName(), FunctionName: "get", CurrentMethodDesc: "()Ljava/lang/Object;"}
			wantName, want := "Item", true
			switch mode {
			case "qualified":
				ctx.TypeParams = []string{"Item"}
				wantName = "PacketOwner.Item"
			case "shadowed root":
				ctx.TypeParams = []string{"PacketOwner"}
			case "no denotable anchor":
				ctx.TypeParams = []string{"Item", "PacketOwner"}
				want = false
			case "wrong source method":
				ctx.FunctionName = "other"
				want = false
			case "wrong source descriptor":
				ctx.CurrentMethodDesc = "()V"
				want = false
			case "copied method":
				copy := *c.CurrentMethod
				c.CurrentMethod = &copy
				want = false
			case "foreign method":
				c.CurrentMethod = root.Methods[0]
				want = false
			case "missing current local":
				c.nativeMethodLocalCurrent = nil
				want = false
			case "failed family":
				p.failed = true
				want = false
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				want = false
			case "memory":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				want = false
			case "canceled":
				request, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(request, workbudget.Limits{})
				want = false
			}
			name, qualifier, known := c.nativeMethodLocalMemberAllocationNames(plan, p, ctx)
			if known != want || known && (name != wantName || qualifier != "") {
				t.Fatalf("name=%q qualifier=%q known=%v want=%q/%v", name, qualifier, known, wantName, want)
			}
		})
	}
}
