package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberEnclosingParameterAllocationRequiresOriginalConstructorIdentity(t *testing.T) {
	files := nativeCompileClasses(t, constructorEnclosingParameterFixture)
	for _, variant := range []string{"original", "foreign object", "copied declaration", "missing method", "duplicate method", "copied method", "static method", "static current", "static target", "foreign enclosing owner", "missing capture", "missing constructor", "wrong descriptor", "before delegation", "foreign code", "duplicate code", "missing NEW", "missing DUP", "foreign load", "parameter write", "receiver write", "wide overlap", "nil opcode", "too many opcodes", "too many attributes", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["ConstructorScope.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original member forest")
			}
			current, target := p.children["ConstructorScope$Actor"], p.children["ConstructorScope$Target"]
			obj := current.object
			const descriptor = "(LConstructorScope;Ljava/lang/Object;J)V"
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if n == "<init>" && d == descriptor {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original constructor")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			allocation := -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_NEW {
					allocation = i
					break
				}
			}
			if allocation < 0 || allocation+2 >= len(ops) {
				t.Fatal("original allocation and enclosing load")
			}
			change := func(i, opcode int) {
				op := *ops[i]
				instr := *op.Instr
				instr.OpCode = opcode
				op.Instr = &instr
				ops[i] = &op
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign object":
				obj = root
			case "copied declaration":
				copy := *obj
				obj = &copy
			case "missing method":
				obj.Methods = nil
			case "duplicate method":
				obj.Methods = append(obj.Methods, method)
			case "copied method":
				copy := *method
				method = &copy
			case "static method":
				method.AccessFlags |= 8
			case "static current":
				current.static = true
			case "static target":
				target.static = true
			case "foreign enclosing owner":
				target.owner = "ForeignScope"
			case "missing capture":
				current.field = ""
			case "missing constructor":
				current.constructors = nil
			case "wrong descriptor":
				current.constructors[descriptor].descriptor = "(LConstructorScope;J)V"
			case "before delegation":
				current.constructors[descriptor].delegatePC = int(ops[allocation].CurrentOffset)
			case "foreign code":
				copy := *code
				code = &copy
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "missing NEW":
				change(allocation, core.OP_NOP)
			case "missing DUP":
				change(allocation+1, core.OP_NOP)
			case "foreign load":
				change(allocation+2, core.OP_ALOAD_2)
			case "parameter write":
				change(len(ops)-1, core.OP_ASTORE_1)
			case "receiver write":
				change(len(ops)-1, core.OP_ASTORE_0)
			case "wide overlap":
				change(len(ops)-1, core.OP_LSTORE_0)
			case "nil opcode":
				ops[len(ops)-1] = nil
			case "too many opcodes":
				ops = append(ops, make([]*core.OpCode, 65536-len(ops))...)
			case "too many attributes":
				method.Attributes = append(method.Attributes, make([]AttributeInfo, 4097-len(method.Attributes))...)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := append([]byte(nil), code.Code...)
			proof := nativeMemberConstructorEnclosingParameter(obj, method, code, ops, allocation, current, target, work)
			if (proof != nil) != (variant == "original") {
				t.Fatalf("original constructor parameter accepted=%v", proof != nil)
			}
			if proof != nil && (proof.member != current || proof.method != method || proof.constructor != current.constructors[descriptor] || proof.loadPC != int(ops[allocation+2].CurrentOffset)) {
				t.Fatal("certificate borrowed an unrelated source identity")
			}
			if !bytes.Equal(before, code.Code) {
				t.Fatal("proof changed original bytecode")
			}
		})
	}
}

func TestNativeMemberEnclosingParameterSourceRequiresExactRetainedParameter(t *testing.T) {
	for _, variant := range []string{"original nullable parameter", "foreign ref", "same ID copied ref", "ordinary local", "THIS", "literal null", "opaque", "stack alias", "wrong type", "wrong source name", "missing original slot", "wrong original slot", "mutated parameter seed", "foreign constructor", "foreign member", "missing constructor", "ordinary method", "static context", "wrong descriptor", "foreign source class", "nil context", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := &ClassObject{}
			cp := NewConstantPoolWithConstant(&obj.ConstantPool)
			obj.ThisClass = uint16(cp.AddNewClassInfo("SourceScope$Actor"))
			method := &MemberInfo{}
			ctor := &nativeMemberConstructor{}
			const desc = "(LSourceScope;Ljava/lang/Object;)V"
			member := &nativeMemberClass{object: obj, owner: "SourceScope", constructors: map[string]*nativeMemberConstructor{desc: ctor}}
			proof := &nativeMemberEnclosingParameter{member: member, method: method, constructor: ctor, descriptor: desc}
			id := &utils.VariableId{}
			ref := values.NewJavaRef(id, values.JavaNull, types.NewJavaClass("SourceScope"))
			ref.IsParam = true
			if variant != "missing original slot" {
				slot := 1
				if variant == "wrong original slot" {
					slot = 2
				}
				ref.MarkOriginalParameter(slot)
			}
			ctx := &class_context.ClassContext{ClassName: "SourceScope$Actor", FunctionName: "<init>", CurrentMethodDesc: desc, LocalNames: map[*utils.VariableId]string{id: "SourceScope.this"}}
			d := &ClassObjectDumper{obj: obj, CurrentMethod: method, nativeMemberCurrent: member, nativeConstructorEnclosing: ref}
			var value any = ref
			switch variant {
			case "foreign ref", "same ID copied ref":
				copy := *ref
				if variant == "foreign ref" {
					copy.Id = &utils.VariableId{}
				}
				value = &copy
			case "ordinary local":
				ref.IsParam = false
			case "THIS":
				ref.IsThis = true
			case "literal null":
				value = values.JavaNull
			case "opaque":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { panic("must not query mutable type") })
			case "stack alias":
				ref.StackVar = ref
			case "wrong type":
				ref = values.NewJavaRef(id, values.JavaNull, types.NewJavaClass("java.lang.Object"))
				ref.IsParam = true
				ref.MarkOriginalParameter(1)
				d.nativeConstructorEnclosing = ref
				value = ref
			case "wrong source name":
				ctx.LocalNames[id] = "OtherScope.this"
			case "mutated parameter seed":
				ref.Val = values.NewJavaLiteral("null", types.NewJavaClass("SourceScope"))
			case "foreign constructor":
				d.CurrentMethod = &MemberInfo{}
			case "foreign member":
				copy := *member
				d.nativeMemberCurrent = &copy
			case "missing constructor":
				member.constructors = nil
			case "ordinary method":
				ctx.FunctionName = "ordinary"
			case "static context":
				ctx.IsStatic = true
			case "wrong descriptor":
				ctx.CurrentMethodDesc = "(LSourceScope;)V"
			case "foreign source class":
				ctx.ClassName = "OtherScope$Actor"
			case "nil context":
				ctx = nil
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				parent, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(parent, workbudget.Limits{})
			}
			if accepted := d.nativeMemberConstructorEnclosingOperand(value, proof, ctx); accepted != (variant == "original nullable parameter") {
				t.Fatalf("source parameter accepted=%v", accepted)
			}
		})
	}
}

func TestNativeMemberEnclosingParameterAllocationNameRequiresOriginalNamespace(t *testing.T) {
	files := nativeCompileClasses(t, constructorEnclosingParameterFixture)
	for _, variant := range []string{"original", "nil object", "missing source identity", "failed family", "shadowed leading type", "shadowed leading formal", "missing certificate", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["ConstructorScope.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original source names")
			}
			child := p.children["ConstructorScope$Target"]
			plan := &nativeMemberAllocation{child: child, enclosingParameter: &nativeMemberEnclosingParameter{}}
			ctx := &class_context.ClassContext{}
			switch variant {
			case "nil object":
				child.object = nil
			case "missing source identity":
				delete(p.children, "ConstructorScope$Target")
			case "failed family":
				p.failed = true
			case "shadowed leading type":
				ctx.LexicalTypeNames = map[string]bool{"ConstructorScope": true}
			case "shadowed leading formal":
				ctx.TypeParams = []string{"ConstructorScope"}
			case "missing certificate":
				plan.enclosingParameter = nil
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				parent, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(parent, workbudget.Limits{})
			}
			name, known := d.nativeMemberEnclosingParameterAllocationName(plan, p, ctx)
			if known != (variant == "original") || known && name != "ConstructorScope.Target" {
				t.Fatalf("source identity %q known=%v", name, known)
			}
		})
	}
}
