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

func TestNativeMethodLocalLexicalReadsRequireWholeOriginalReceiverPath(t *testing.T) {
	files := nativeCompileClasses(t, nativeMethodLocalLexicalCaptureFixture)
	for _, variant := range []string{"original", "foreign local receiver", "slot zero redefined", "capture store outside constructor", "branch into field", "catch starts inside chain", "catch ends inside chain", "handler inside chain", "foreign descriptor", "static named capture", "static local capture", "local capture store outside constructor", "foreign local capture descriptor", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["LocalCaptureOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			obj := p.methodLocals["LocalCaptureOwner$Container$1Entry"].object
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "root" {
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
			if len(ops) != 5 {
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
				nt.DescriptorIndex = uint16(pool.AddUtf8Info("Ljava/lang/Object;"))
			case "static named capture":
				code.Code[second] = byte(core.OP_GETSTATIC)
			case "static local capture":
				code.Code[first] = byte(core.OP_GETSTATIC)
			case "local capture store outside constructor":
				code.Code = append(code.Code, byte(core.OP_ALOAD_0), byte(core.OP_ACONST_NULL), byte(core.OP_PUTFIELD), code.Code[first+1], code.Code[first+2], byte(core.OP_RETURN))
			case "foreign local capture descriptor":
				index := int(binary.BigEndian.Uint16(code.Code[first+1:]))
				ref := obj.ConstantPool[index-1].(*ConstantFieldrefInfo)
				nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				pool := NewConstantPoolWithConstant(&obj.ConstantPool)
				nt.DescriptorIndex = uint16(pool.AddUtf8Info("Ljava/lang/Object;"))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, known := nativeMemberLexicalReads(obj, p, work)
			if known != (variant == "original") {
				t.Fatalf("original path closure %v", known)
			}
		})
	}
}

func TestNativeMethodLocalLexicalScopeRequiresOriginalDeclarationAndPacket(t *testing.T) {
	files := nativeCompileClasses(t, nativeMethodLocalLexicalCaptureFixture)
	for _, variant := range []string{"original", "missing local", "foreign map key", "foreign object", "missing lexical object", "foreign lexical object", "foreign enclosing object", "missing enclosing member", "foreign named owner", "wrong named flags", "duplicate declaration role", "missing root", "wrong method", "wrong descriptor", "wrong ordinal", "wrong declaration", "wrong capture field", "wrong capture parameter", "wrong capture PC", "missing capture PC", "wrong delegate PC", "wrong delegate owner", "failed family", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["LocalCaptureOwner.class"])
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			const name = "LocalCaptureOwner$Container$1Entry"
			local := p.methodLocals[name]
			if local == nil {
				t.Fatal("original local")
			}
			obj := local.object
			var work *workbudget.Budget
			switch variant {
			case "missing local":
				delete(p.methodLocals, name)
			case "foreign map key":
				delete(p.methodLocals, name)
				p.methodLocals["Foreign"] = local
			case "foreign object":
				local.object, _ = Parse(files[name+".class"])
			case "missing lexical object":
				delete(p.lexicalObjects, name)
			case "foreign lexical object":
				p.lexicalObjects[name], _ = Parse(files[name+".class"])
			case "foreign enclosing object":
				p.lexicalObjects[local.owner.owner], _ = Parse(files[local.owner.owner+".class"])
			case "missing enclosing member":
				delete(p.children, local.owner.owner)
			case "foreign named owner":
				p.children[local.owner.owner].owner = "Foreign"
			case "wrong named flags":
				p.children[local.owner.owner].flags ^= 8
			case "duplicate declaration role":
				p.children[name] = &nativeMemberClass{object: obj}
			case "missing root":
				delete(p.lexicalObjects, p.owner)
			case "wrong method":
				local.owner.method = "unknown"
			case "wrong descriptor":
				local.owner.descriptor = "()V"
			case "wrong ordinal":
				local.owner.ordinal++
			case "wrong declaration":
				local.owner.declaration = root.Methods[0]
			case "wrong capture field":
				local.constructor.enclosingField = "val$argument"
			case "wrong capture parameter":
				local.constructor.captures[local.constructor.enclosingField]++
			case "wrong capture PC":
				local.constructor.capturePCs[local.constructor.enclosingField]++
			case "missing capture PC":
				delete(local.constructor.capturePCs, local.constructor.enclosingField)
			case "wrong delegate PC":
				local.constructor.delegatePC++
			case "wrong delegate owner":
				local.constructor.delegateOwner = "Foreign"
			case "failed family":
				p.failed = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, known := nativeMemberJointMethodLocalOwner(p, obj, work)
			if known != (variant == "original") {
				t.Fatalf("local scope admitted=%v", known)
			}
		})
	}
}
func TestNativeMethodLocalLexicalCaptureChainRequiresExactOriginalIRPath(t *testing.T) {
	files := nativeCompileClasses(t, nativeMethodLocalLexicalCaptureFixture)
	for _, variant := range []string{"original", "missing final PC", "wrong final PC", "missing inner PC", "wrong field", "wrong field type", "foreign THIS type", "foreign receiver", "mutable alias", "opaque", "cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["LocalCaptureOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original complete family")
			}
			leaf := p.methodLocals["LocalCaptureOwner$Container$1Entry"]
			reads, known := nativeMemberLexicalReads(leaf.object, p, nil)
			if !known {
				t.Fatal("original capture reads")
			}
			var read *nativeMemberLexicalRead
			for _, r := range reads["root()Ljava/lang/Object;"] {
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
			if got := nativeMemberLexicalReadOperand(value, read, work); got != (variant == "original") {
				t.Fatalf("IR path %v", got)
			}
		})
	}
}
