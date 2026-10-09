package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMethodLocalDelegationCompilerMetadataCannotBorrowProfile(t *testing.T) {
	files := nativeCompileClasses(t, localCapturedDelegationFixture)
	for _, kind := range []string{"native absent table", "modern output", "wrong target", "owner version", "child version", "owner minor", "child minor", "cached owner", "restored table", "missing signature", "duplicate signature", "wrong signature", "foreign attribute", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["DelegationOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(bytes.Clone(files["DelegationOwner$1Entry.class"]))
			if err != nil {
				t.Fatal(err)
			}
			owner, known := originalMethodLocalOwner(child, root, nil)
			if !known {
				t.Fatal("original owner")
			}
			var method *MemberInfo
			var table *UnparsedAttribute
			var signature *SignatureAttribute
			for _, m := range child.Methods {
				if name, _ := sourceBridgeUTF8(child, m.NameIndex); name == "<init>" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original constructor")
			}
			keep := []AttributeInfo{}
			for _, a := range method.Attributes {
				if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
					table = p
					continue
				}
				if p, ok := a.(*SignatureAttribute); ok {
					signature = p
				}
				keep = append(keep, a)
			}
			if table == nil || signature == nil {
				t.Fatal("independent modern compiler protocol")
			}
			method.Attributes = keep
			desc, _ := sourceBridgeUTF8(child, method.DescriptorIndex)
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.options.SourceCompiler, d.options.TargetSourceVersion = NativeJavac8, 8
			switch kind {
			case "modern output":
				d.options.SourceCompiler = ModernJavac
			case "wrong target":
				d.options.TargetSourceVersion = 11
			case "owner version":
				root.MajorVersion = 51
			case "child version":
				child.MajorVersion = 51
			case "owner minor":
				root.MinorVersion = 1
			case "child minor":
				child.MinorVersion = 1
			case "cached owner":
				owner.declaration = nil
			case "restored table":
				method.Attributes = append(method.Attributes, table)
			case "missing signature":
				withoutSignature := []AttributeInfo{}
				for _, a := range method.Attributes {
					if a != signature {
						withoutSignature = append(withoutSignature, a)
					}
				}
				method.Attributes = withoutSignature
			case "duplicate signature":
				method.Attributes = append(method.Attributes, signature)
			case "wrong signature":
				signature.SignatureIndex = uint16(child.ConstantPoolManager.AddUtf8Info("(I)V"))
			case "foreign attribute":
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "Opaque"})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.nativeMethodLocalConstructorSourceMetadata(child, method, params, owner); got != (kind == "native absent table") {
				t.Fatal("compiler metadata eligibility", kind, got)
			}
		})
	}
}

func TestAdversarialMethodLocalDelegationRequiresOriginalHiddenWords(t *testing.T) {
	files := nativeCompileClasses(t, localCapturedDelegationFixture)
	for _, kind := range []string{"original", "padding", "wrong owner", "duplicate constructor", "duplicate code", "wrong category", "receiver word", "missing argument", "argument calculation", "foreign parent", "wrong invoke", "post-delegation throw", "extra field", "not final", "not synthetic", "signature source arguments", "handler", "small stack", "small locals", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			owner, err := Parse(bytes.Clone(files["DelegationOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(bytes.Clone(files["DelegationOwner$1Entry.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var ctor *MemberInfo
			var code *CodeAttribute
			for _, m := range child.Methods {
				name, _ := sourceBridgeUTF8(child, m.NameIndex)
				if name == "<init>" {
					ctor = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if ctor == nil || code == nil {
				t.Fatal("original local constructor")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			invokeIndex := -1
			for i, op := range ops {
				call := constructorMotionMember(child, op, core.OP_INVOKESPECIAL)
				if call != nil && call.Name == "DelegationBase" && call.Member == "<init>" && call.Description == "(IJLjava/lang/Object;)V" {
					if invokeIndex != -1 {
						t.Fatal("duplicate original parent invocation")
					}
					invokeIndex = i
				}
			}
			if invokeIndex < 3 || invokeIndex+2 != len(ops) || !constructorMotionLoad(ops[invokeIndex-3], "I") || !constructorMotionLoad(ops[invokeIndex-2], "J") || !constructorMotionLoad(ops[invokeIndex-1], "Ljava/lang/Object;") {
				t.Fatal("original typed SUPER words")
			}
			firstArg := int(ops[invokeIndex-3].CurrentOffset)
			invokePC := int(ops[invokeIndex].CurrentOffset)
			var work *workbudget.Budget
			switch kind {
			case "padding":
				code.Code = append(append(append([]byte{}, code.Code[:invokePC]...), byte(core.OP_NOP)), code.Code[invokePC:]...)
				invokePC++
			case "wrong owner":
				owner = child
			case "duplicate constructor":
				child.Methods = append(child.Methods, ctor)
			case "duplicate code":
				ctor.Attributes = append(ctor.Attributes, code)
			case "wrong category":
				code.Code[firstArg] = byte(core.OP_ALOAD_2)
			case "receiver word":
				code.Code[firstArg] = byte(core.OP_ILOAD_0)
			case "missing argument":
				code.Code[firstArg] = byte(core.OP_NOP)
			case "argument calculation":
				code.Code = append(append(append([]byte{}, code.Code[:invokePC]...), byte(core.OP_ICONST_1), byte(core.OP_IADD)), code.Code[invokePC:]...)
			case "foreign parent":
				ref := child.ConstantPool[core.Convert2bytesToInt(ops[len(ops)-2].Data)-1].(*ConstantMethodrefInfo)
				ref.ClassIndex = child.ThisClass
			case "wrong invoke":
				code.Code[invokePC] = byte(core.OP_INVOKEVIRTUAL)
			case "post-delegation throw":
				code.Code[len(code.Code)-1] = byte(core.OP_ACONST_NULL)
				code.Code = append(code.Code, byte(core.OP_ATHROW))
			case "extra field":
				child.Fields = append(child.Fields, child.Fields[0])
			case "not final":
				child.Fields[0].AccessFlags &^= 0x10
			case "not synthetic":
				child.Fields[0].AccessFlags &^= 0x1000
			case "signature source arguments":
				found := false
				for _, a := range ctor.Attributes {
					if signature, ok := a.(*SignatureAttribute); ok {
						signature.SignatureIndex = uint16(child.ConstantPoolManager.AddUtf8Info("(I)V"))
						found = true
					}
				}
				if !found {
					t.Fatal("original zero-source-argument Signature")
				}
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code)), HandlerPc: 0}}
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			proof, known := originalMethodLocalSourceConstructor(child, owner, work)
			if known != (kind == "original" || kind == "padding") {
				t.Fatalf("physical delegation known=%v proof=%+v", known, proof)
			}
			if known {
				if len(proof.delegateParams) != 3 || proof.delegateDescriptor != "(IJLjava/lang/Object;)V" || proof.delegatePC != invokePC {
					t.Fatal("lost original delegation packet", proof)
				}
				if _, defaultProof := originalMethodLocalDefaultConstructor(child, owner, nil); defaultProof {
					t.Fatal("explicit SUPER arguments borrowed default-constructor license")
				}
			}
		})
	}
}

func TestAdversarialMethodLocalDelegationSourceRequiresClosedBinding(t *testing.T) {
	files := nativeCompileClasses(t, localCapturedDelegationFixture)
	for _, kind := range []string{"original", "missing context", "missing transaction", "missing parent", "bad parent bytes", "duplicate parent constructor", "private constructor", "abstract parent", "interface parent", "generic parent", "generic constructor", "checked exception", "unknown exceptions", "shadowed captured name", "ancestry cycle", "missing ancestor", "missing binding", "different declaration", "missing original owner", "wrong physical descriptor", "budget", "output budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			parse := func(name string) *ClassObject {
				o, err := Parse(bytes.Clone(files[name+".class"]))
				if err != nil {
					t.Fatal(err)
				}
				return o
			}
			owner, child, parent := parse("DelegationOwner"), parse("DelegationOwner$1Entry"), parse("DelegationBase")
			originalOwner, known := originalMethodLocalOwner(child, owner, nil)
			if !known {
				t.Fatal("actual local ownership")
			}
			ctor, known := originalMethodLocalSourceConstructor(child, owner, nil)
			if !known {
				t.Fatal("actual hidden delegation words")
			}
			var method, super *MemberInfo
			for _, m := range child.Methods {
				if name, _ := sourceBridgeUTF8(child, m.NameIndex); name == "<init>" {
					method = m
				}
			}
			for _, m := range parent.Methods {
				if name, _ := sourceBridgeUTF8(parent, m.NameIndex); name == "<init>" {
					super = m
				}
			}
			if method == nil || super == nil {
				t.Fatal("actual delegation members")
			}
			local := &nativeMethodLocalClass{object: child, owner: originalOwner, constructor: ctor, bindings: map[string]string{}, captureIDs: map[string]*coreutils.VariableId{}, sourceRefs: map[string]*values.JavaRef{}}
			for field := range ctor.captures {
				binding := strings.TrimPrefix(field, "val$")
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
				local.bindings[field], local.captureIDs[field], local.sourceRefs[field] = binding, ref.Id, ref
			}
			d := NewClassObjectDumper(child)
			d.nativeMethodLocalCurrent = local
			d.nativeMemberRoot = &nativeMemberFamily{lexicalObjects: map[string]*ClassObject{owner.GetClassName(): owner}}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == parent.GetClassName() {
					if kind == "missing parent" {
						return nil, false
					}
					if kind == "bad parent bytes" {
						return []byte{1, 2}, true
					}
					return parent.Bytes(), true
				}
				b, known := files[name+".class"]
				return bytes.Clone(b), known
			}
			d.FuncCtx = &class_context.ClassContext{}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			switch kind {
			case "missing context":
				d.FuncCtx = nil
			case "missing transaction":
				d.nativeMemberRoot = nil
			case "duplicate parent constructor":
				parent.Methods = append(parent.Methods, super)
			case "private constructor":
				super.AccessFlags = 2
			case "abstract parent":
				parent.AccessFlags |= 0x0400
			case "interface parent":
				parent.AccessFlags |= 0x0200
			case "generic parent":
				parent.Attributes = append(parent.Attributes, &SignatureAttribute{SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;"))})
			case "generic constructor":
				super.Attributes = append(super.Attributes, &SignatureAttribute{SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>(IJLjava/lang/Object;)V"))})
			case "checked exception":
				super.Attributes = append(super.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{uint16(parent.ConstantPoolManager.AddNewClassInfo("java/io/IOException"))}})
			case "unknown exceptions":
				d.FuncCtx.InvocationMetadata = nil
			case "shadowed captured name":
				parent.Fields[0].NameIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("n"))
			case "ancestry cycle":
				parent.SuperClass = parent.ThisClass
			case "missing ancestor":
				parent.SuperClass = uint16(parent.ConstantPoolManager.AddNewClassInfo("UnresolvedAncestor"))
			case "missing binding":
				delete(local.bindings, "val$n")
			case "different declaration":
				local.captureIDs["val$n"] = coreutils.NewRootVariableId()
			case "missing original owner":
				delete(d.nativeMemberRoot.lexicalObjects, owner.GetClassName())
			case "wrong physical descriptor":
				method.DescriptorIndex = uint16(child.ConstantPoolManager.AddUtf8Info("()V"))
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			source, err := d.nativeMethodLocalDelegationSource(method)
			if kind == "original" || kind == "abstract parent" {
				if err != nil || source == nil || !strings.Contains(source.bodyCode, "super(") {
					t.Fatal("original source transaction", err, source)
				}
			} else if err == nil || source != nil {
				t.Fatal("unproved source permission", kind, err, source)
			}
		})
	}
}

func TestAdversarialMethodLocalDelegationCachedPacketCannotReplaceOriginal(t *testing.T) {
	files := nativeCompileClasses(t, localCapturedDelegationFixture)
	owner, err := Parse(files["DelegationOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	child, err := Parse(files["DelegationOwner$1Entry.class"])
	if err != nil {
		t.Fatal(err)
	}
	actual, known := originalMethodLocalSourceConstructor(child, owner, nil)
	if !known {
		t.Fatal("original packet")
	}
	for _, kind := range []string{"original", "owner", "descriptor", "pc", "omitted word", "swapped words", "capture slot", "capture pc", "extra capture pc", "budget"} {
		t.Run(kind, func(t *testing.T) {
			cached, _ := originalMethodLocalSourceConstructor(child, owner, nil)
			var work *workbudget.Budget
			switch kind {
			case "owner":
				cached.delegateOwner = owner.GetClassName()
			case "descriptor":
				cached.delegateDescriptor = "()V"
			case "pc":
				cached.delegatePC++
			case "omitted word":
				cached.delegateParams = cached.delegateParams[:2]
			case "swapped words":
				cached.delegateParams[0], cached.delegateParams[1] = cached.delegateParams[1], cached.delegateParams[0]
			case "capture slot":
				cached.captures["val$n"]++
			case "capture pc":
				cached.capturePCs["val$n"]++
			case "extra capture pc":
				cached.capturePCs["unproved"] = 0
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if got := sameOriginalMethodLocalConstructor(actual, cached, work); got != (kind == "original") {
				t.Fatal("cached packet corroboration", got, cached)
			}
		})
	}
}
