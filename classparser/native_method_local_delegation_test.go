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
	for _, kind := range []string{"original", "missing context", "missing transaction", "missing parent", "bad parent bytes", "duplicate parent constructor", "private constructor", "abstract parent", "interface parent", "generic parent", "malformed generic parent", "free parent bound", "cyclic parent bound", "wrong generic parent hierarchy", "wrong generic parent interface", "duplicate parent signature", "invalid parent signature index", "generic constructor", "empty exceptions", "duplicate empty exceptions", "checked exception", "unknown exceptions", "shadowed captured name", "ancestry cycle", "missing ancestor", "missing binding", "different declaration", "missing original owner", "wrong physical descriptor", "budget", "output budget", "canceled"} {
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
			parent.ConstantPoolManager.AddUtf8Info("Signature")
			parent.ConstantPoolManager.AddUtf8Info("Exceptions")
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
			if kind == "checked exception" {
				// First retain the readable no-throws parent declaration, then
				// change resolver bytes below. This is a genuinely inconsistent
				// cached provider, not simply a newly populated exception cache.
				exceptions, known := exactInvocationExceptions(d.FuncCtx.InvocationMetadata, parent.GetClassName(), "<init>", ctor.delegateDescriptor)
				if !known || len(exceptions) != 0 {
					t.Fatal("original no-throws provider must be cached", exceptions, known)
				}
			}
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
				parent.Attributes = append(parent.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;"))})
			case "malformed generic parent", "free parent bound", "cyclic parent bound", "wrong generic parent hierarchy", "wrong generic parent interface", "duplicate parent signature", "invalid parent signature index":
				sig := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				switch kind {
				case "malformed generic parent":
					sig += "x"
				case "free parent bound":
					sig = "<T:TU;>Ljava/lang/Object;"
				case "cyclic parent bound":
					sig = "<T:TT;>Ljava/lang/Object;"
				case "wrong generic parent hierarchy":
					sig = "<T:Ljava/lang/Object;>Ljava/lang/Number;"
				case "wrong generic parent interface":
					sig += "Ljava/io/Serializable;"
				}
				a := &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info(sig))}
				if kind == "invalid parent signature index" {
					a.SignatureIndex = 65535
				}
				parent.Attributes = append(parent.Attributes, a)
				if kind == "duplicate parent signature" {
					parent.Attributes = append(parent.Attributes, a)
				}
			case "generic constructor":
				super.Attributes = append(super.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>(IJLjava/lang/Object;)V"))})
			case "empty exceptions", "duplicate empty exceptions":
				a := &ExceptionsAttribute{AttrLen: 2}
				super.Attributes = append(super.Attributes, a)
				if kind == "duplicate empty exceptions" {
					super.Attributes = append(super.Attributes, a)
				}
			case "checked exception":
				super.Attributes = append(super.Attributes, &ExceptionsAttribute{AttrLen: 4, ExceptionIndexTable: []uint16{uint16(parent.ConstantPoolManager.AddNewClassInfo("java/io/IOException"))}})
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
			// These variants must reach the intended proof, not accidentally pass
			// because a hand-written attribute cannot be serialized and parsed.
			expectedProof := map[string]string{
				"malformed generic parent":       "closed original generic scope",
				"free parent bound":              "closed original generic scope",
				"cyclic parent bound":            "closed original generic scope",
				"wrong generic parent hierarchy": "changes physical hierarchy",
				"wrong generic parent interface": "changes physical hierarchy",
				"generic constructor":            "needs instantiated generic binding",
				"checked exception":              "separate checked-exception source proof",
			}
			if expectedProof[kind] != "" || kind == "generic parent" || kind == "empty exceptions" {
				if _, parseErr := Parse(bytes.Clone(parent.Bytes())); parseErr != nil {
					t.Fatal("control must have readable original parent metadata", kind, parseErr)
				}
			}
			source, err := d.nativeMethodLocalDelegationSource(method)
			if want := expectedProof[kind]; want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
				t.Fatal("wrong refusal proof", kind, want, err)
			}
			if kind == "original" || kind == "abstract parent" || kind == "generic parent" || kind == "empty exceptions" {
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
	for _, kind := range []string{"original", "owner", "descriptor", "pc", "omitted word", "swapped words", "cast count", "both cast counts", "cast identity", "capture slot", "capture pc", "extra capture pc", "budget"} {
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
			case "cast count":
				cached.delegateCasts = nil
			case "cast identity":
				cached.delegateCasts[0] = "LWrong;"
			case "capture slot":
				cached.captures["val$n"]++
			case "capture pc":
				cached.capturePCs["val$n"]++
			case "extra capture pc":
				cached.capturePCs["unproved"] = 0
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			corroboration := actual
			if kind == "both cast counts" {
				copy := *actual
				copy.delegateCasts = nil
				corroboration = &copy
				cached.delegateCasts = nil
			}
			if got := sameOriginalMethodLocalConstructor(corroboration, cached, work); got != (kind == "original") {
				t.Fatal("cached packet corroboration", got, cached)
			}
		})
	}
}

func TestAdversarialMethodLocalDelegationWideningUsesFreshOriginalHierarchy(t *testing.T) {
	files := nativeCompileClasses(t, `interface HierarchyMarker{}class HierarchyRoot implements HierarchyMarker{}class HierarchyMiddle extends HierarchyRoot{}class HierarchyLeaf extends HierarchyMiddle{}class HierarchyOwner{}`)
	for _, kind := range []string{"original", "identity", "object", "interface", "reference covariance", "primitive matrix covariance", "array marker", "narrowing", "primitive", "array narrowing", "primitive array narrowing", "missing actual", "missing endpoint", "wrong identity", "cycle", "stale provider cannot invent edge", "stale provider cannot remove edge", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["HierarchyOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			data := map[string][]byte{}
			for n, b := range files {
				data[n] = bytes.Clone(b)
			}
			d := NewClassObjectDumper(root)
			d.FuncCtx = &class_context.ClassContext{}
			actual, formal := "LHierarchyLeaf;", "LHierarchyRoot;"
			want := true
			switch kind {
			case "identity":
				formal = actual
			case "object":
				formal = "Ljava/lang/Object;"
			case "interface":
				formal = "LHierarchyMarker;"
			case "reference covariance":
				actual, formal = "[[LHierarchyLeaf;", "[[LHierarchyRoot;"
			case "primitive matrix covariance":
				actual, formal = "[[I", "[Ljava/lang/Object;"
			case "array marker":
				actual, formal = "[I", "Ljava/lang/Cloneable;"
			case "narrowing":
				actual, formal, want = "LHierarchyRoot;", "LHierarchyLeaf;", false
			case "primitive":
				actual, formal, want = "I", "J", false
			case "array narrowing":
				actual, formal, want = "[Ljava/lang/Object;", "[Ljava/lang/String;", false
			case "primitive array narrowing":
				actual, formal, want = "[I", "[J", false
			case "missing actual":
				delete(data, "HierarchyLeaf.class")
				want = false
			case "missing endpoint":
				delete(data, "HierarchyRoot.class")
				want = false
			case "wrong identity":
				data["HierarchyLeaf.class"] = bytes.Clone(data["HierarchyRoot.class"])
				want = false
			case "cycle":
				leaf, err := Parse(bytes.Clone(data["HierarchyLeaf.class"]))
				if err != nil {
					t.Fatal(err)
				}
				leaf.SuperClass = leaf.ThisClass
				data["HierarchyLeaf.class"] = leaf.Bytes()
				want = false
			case "stale provider cannot invent edge":
				actual, formal, want = "LHierarchyRoot;", "LHierarchyLeaf;", false
				d.FuncCtx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					return callbinding.Class{Name: n, Parents: []string{"HierarchyLeaf"}, ParentsComplete: true}, true
				}
			case "stale provider cannot remove edge":
				d.FuncCtx.InvocationMetadata = func(string) (callbinding.Class, bool) { return callbinding.Class{}, false }
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				want = false
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				want = false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
				want = false
			}
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := data[n+".class"]; return bytes.Clone(b), ok }
			if got := d.nativeMethodLocalDelegationArgumentAssignable(d.nativeMethodLocalDelegationWideningQuery(), actual, formal); got != want {
				t.Fatal("original reference conversion", kind, got, want)
			}
		})
	}
}

func TestAdversarialMethodLocalDelegationCastPacketRetainsOriginalTarget(t *testing.T) {
	files := nativeCompileClasses(t, `class CastParent{CastParent(Object[]x){}}class CastOwner{Object make(final String[][]x){class Entry extends CastParent{Entry(){super((Object[])x);}Object capture(){return x;}}return new Entry();}}`)
	for _, kind := range []string{"original", "different cast", "duplicate cast", "cast calculation", "default certificate"} {
		t.Run(kind, func(t *testing.T) {
			child, err := Parse(bytes.Clone(files["CastOwner$1Entry.class"]))
			if err != nil {
				t.Fatal(err)
			}
			owner, err := Parse(bytes.Clone(files["CastOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			for _, m := range child.Methods {
				if n, _ := sourceBridgeUTF8(child, m.NameIndex); n == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original constructor")
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.ConstantPool, i) })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			pc := -1
			for _, op := range constructorMotionOps(d) {
				if op.Instr.OpCode == core.OP_CHECKCAST {
					if pc >= 0 {
						t.Fatal("ambiguous cast")
					}
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("original array widening cast")
			}
			switch kind {
			case "different cast":
				index := child.ConstantPoolManager.AddNewClassInfo("[[Ljava/lang/String;")
				code.Code[pc+1], code.Code[pc+2] = byte(index>>8), byte(index)
			case "duplicate cast":
				code.Code = append(append(append([]byte{}, code.Code[:pc]...), code.Code[pc:pc+3]...), code.Code[pc:]...)
			case "cast calculation":
				code.Code[pc] = byte(core.OP_INSTANCEOF)
			}
			if kind == "default certificate" {
				if _, known := originalMethodLocalDefaultConstructor(child, owner, nil); known {
					t.Fatal("explicit cast packet cannot become default constructor")
				}
				return
			}
			packet, known := originalMethodLocalSourceConstructor(child, owner, nil)
			if kind == "original" {
				if !known || len(packet.delegateCasts) != 1 || packet.delegateCasts[0] != "[Ljava/lang/Object;" {
					t.Fatal("original complete cast packet", packet, known)
				}
			} else if known {
				t.Fatal("altered cast packet admitted", kind, packet)
			}
		})
	}
}

const localGenericParentSource = `
class ParentEffects {static boolean fail;static final RuntimeException failure=new IllegalStateException("same");static GenericParent<?,?> published;}
class GenericParent<A,B> {final java.util.Map<A,B> map;final Object seenAlias,seenOwner;GenericParent(java.util.Map<A,B> map){this.map=map;seenAlias=alias();seenOwner=owner();ParentEffects.published=this;if(ParentEffects.fail)throw ParentEffects.failure;}Object alias(){return null;}Object owner(){return null;}}
class GenericOwner<K,V> extends java.util.LinkedHashMap<K,java.util.List<V>> {
 Object make(final java.util.Map<K,java.util.List<V>> input){class Local extends GenericParent<K,java.util.List<V>>{Local(){super(input);}Object alias(){return input;}Object owner(){return GenericOwner.this;}}return new Local();}
}
class GenericDriver {public static void main(String[]args)throws Exception {int rows=0;
 for(boolean nil:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}){
 GenericOwner<String,Integer> owner=new GenericOwner<String,Integer>();java.util.List<Integer> token=java.util.Arrays.asList(Integer.MIN_VALUE,0,Integer.MAX_VALUE);owner.put("root",token);java.util.Map<String,java.util.List<Integer>> input=nil?null:new java.util.LinkedHashMap<String,java.util.List<Integer>>();if(input!=null)input.put("alias",token);ParentEffects.fail=fail;ParentEffects.published=null;Object made=null;
 try{made=owner.make(input);if(fail)throw new AssertionError("lost parent failure");}catch(RuntimeException actual){if(!fail||actual!=ParentEffects.failure)throw new AssertionError("parent exception identity",actual);}
 GenericParent<?,?> parent=ParentEffects.published;if(parent==null||parent.map!=input||parent.seenAlias!=input||parent.seenOwner!=owner||!fail&&made!=parent)throw new AssertionError("original capture before observing SUPER");
 Class<?> local=parent.getClass();if(!local.isLocalClass()||local.getEnclosingClass()!=GenericOwner.class||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("local source ownership");java.lang.reflect.Type parentType=local.getGenericSuperclass();if(!(parentType instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("raw parent view");java.lang.reflect.Type[] actuals=((java.lang.reflect.ParameterizedType)parentType).getActualTypeArguments();if(actuals.length!=2||!actuals[0].equals(GenericOwner.class.getTypeParameters()[0])||!(actuals[1] instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)actuals[1]).getRawType()!=java.util.List.class||!((java.lang.reflect.ParameterizedType)actuals[1]).getActualTypeArguments()[0].equals(GenericOwner.class.getTypeParameters()[1]))throw new AssertionError("original formal declarations");rows++;
 }System.out.println(rows+":generic-local-super:pre-super-capture:identity:failure:binding");}}
`

func TestAdversarialLocalGenericParentKeepsCapturedMapInstantiation(t *testing.T) {
	testResourceCompletionFamily(t, localGenericParentSource, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
}
func TestAdversarialLocalGenericParentKeepsLiveEnclosingMapInstantiation(t *testing.T) {
	source := strings.ReplaceAll(localGenericParentSource, "super(input);", "super(GenericOwner.this);")
	source = strings.ReplaceAll(source, "parent.map!=input", "parent.map!=owner")
	testResourceCompletionFamily(t, source, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
}

func localGenericDelegationTestState(t *testing.T, files map[string][]byte) (*ClassObjectDumper, *ClassObject, *MemberInfo, string) {
	t.Helper()
	parse := func(name string) *ClassObject {
		object, err := Parse(bytes.Clone(files[name+".class"]))
		if err != nil {
			t.Fatal(err)
		}
		return object
	}
	owner, child, parent := parse("GenericOwner"), parse("GenericOwner$1Local"), parse("GenericParent")
	localOwner, known := originalMethodLocalOwner(child, owner, nil)
	if !known {
		t.Fatal("original lexical owner")
	}
	packet, known := originalMethodLocalSourceConstructor(child, owner, nil)
	if !known {
		t.Fatal("original captured delegation")
	}
	var constructor *MemberInfo
	for _, method := range parent.Methods {
		name, _ := sourceBridgeUTF8(parent, method.NameIndex)
		desc, _ := sourceBridgeUTF8(parent, method.DescriptorIndex)
		if name == "<init>" && desc == packet.delegateDescriptor {
			constructor = method
		}
	}
	if constructor == nil {
		t.Fatal("original parent target")
	}
	parentSignature, known := nativeMethodLocalOriginalSignature(parent, parent.Attributes, nil)
	if !known {
		t.Fatal("original parent signature")
	}
	localSignature, known := nativeMethodLocalOriginalSignature(child, child.Attributes, nil)
	if !known {
		t.Fatal("original local signature")
	}
	ownerSignature, known := nativeMethodLocalOriginalSignature(owner, owner.Attributes, nil)
	if !known {
		t.Fatal("original owner signature")
	}
	methodSignature, known := nativeMethodLocalOriginalSignature(owner, localOwner.declaration.Attributes, nil)
	if !known {
		t.Fatal("original factory signature")
	}
	d := NewClassObjectDumper(child)
	d.nativeMemberRoot = &nativeMemberFamily{owner: owner.GetClassName(), lexicalObjects: map[string]*ClassObject{owner.GetClassName(): owner, child.GetClassName(): child}}
	d.nativeMethodLocalCurrent = &nativeMethodLocalClass{object: child, owner: localOwner, constructor: packet}
	d.FuncCtx = &class_context.ClassContext{ClassName: child.GetClassName(), ClassSig: localSignature, TypeParams: []string{"K", "V"}, LexicalTypeParamSignatures: []string{methodSignature, ownerSignature}}
	d.foldSiblingResolver = func(name string) ([]byte, bool) {
		if name == parent.GetClassName() {
			return parent.Bytes(), true
		}
		raw, known := files[name+".class"]
		return bytes.Clone(raw), known
	}
	return d, parent, constructor, parentSignature
}

func TestNativeMethodLocalGenericDelegationRequiresOriginalBindingPacket(t *testing.T) {
	files := nativeCompileClasses(t, localGenericParentSource)
	for _, kind := range []string{"original", "missing local", "missing family", "foreign local", "foreign parent method", "changed hidden word", "missing local signature", "duplicate local signature", "raw parent view", "wrong parent identity", "free local formal", "missing source formal", "stale local signature", "stale lexical scope", "swapped lexical scopes", "missing original owner", "parent signature erasure", "constructor method formal", "constructor throws signature", "duplicate constructor signature", "constructor checked throws", "duplicate empty throws", "foreign parent signature provider", "work", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			d, parent, method, signature := localGenericDelegationTestState(t, files)
			local := d.nativeMethodLocalCurrent
			setSignature := func(object *ClassObject, attributes *[]AttributeInfo, value string) {
				kept := []AttributeInfo{}
				for _, a := range *attributes {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				if value != "" {
					kept = append(kept, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(object.ConstantPoolManager.AddUtf8Info(value))})
				}
				*attributes = kept
			}
			switch kind {
			case "missing local":
				d.nativeMethodLocalCurrent = nil
			case "missing family":
				d.nativeMemberRoot = nil
			case "foreign local":
				copy := *local
				copy.object = parent
				d.nativeMethodLocalCurrent = &copy
			case "foreign parent method":
				copy := *method
				method = &copy
			case "changed hidden word":
				copy := *local.constructor
				copy.delegateParams = append([]int(nil), copy.delegateParams...)
				copy.delegateParams[0] = 0
				local.constructor = &copy
			case "missing local signature":
				setSignature(d.obj, &d.obj.Attributes, "")
			case "duplicate local signature":
				for _, a := range d.obj.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						d.obj.Attributes = append(d.obj.Attributes, a)
						break
					}
				}
			case "raw parent view":
				setSignature(d.obj, &d.obj.Attributes, "LGenericParent;")
				d.FuncCtx.ClassSig = "LGenericParent;"
			case "wrong parent identity":
				setSignature(d.obj, &d.obj.Attributes, "LOther<TK;TV;>;")
				d.FuncCtx.ClassSig = "LOther<TK;TV;>;"
			case "free local formal":
				setSignature(d.obj, &d.obj.Attributes, "LGenericParent<TUnknown;TV;>;")
				d.FuncCtx.ClassSig = "LGenericParent<TUnknown;TV;>;"
				d.FuncCtx.TypeParams = append(d.FuncCtx.TypeParams, "Unknown")
			case "missing source formal":
				d.FuncCtx.TypeParams = []string{"K"}
			case "stale local signature":
				d.FuncCtx.ClassSig = "LGenericParent<TV;TK;>;"
			case "stale lexical scope":
				d.FuncCtx.LexicalTypeParamSignatures[1] = "<K:Ljava/lang/Number;V:Ljava/lang/Object;>Ljava/lang/Object;"
			case "swapped lexical scopes":
				d.FuncCtx.LexicalTypeParamSignatures[0], d.FuncCtx.LexicalTypeParamSignatures[1] = d.FuncCtx.LexicalTypeParamSignatures[1], d.FuncCtx.LexicalTypeParamSignatures[0]
			case "missing original owner":
				delete(d.nativeMemberRoot.lexicalObjects, local.owner.owner)
			case "parent signature erasure":
				setSignature(parent, &method.Attributes, "(Ljava/util/List<TA;>;)V")
			case "constructor method formal":
				setSignature(parent, &method.Attributes, "<A:Ljava/lang/Object;>(Ljava/util/Map<TA;TB;>;)V")
			case "constructor throws signature":
				setSignature(parent, &method.Attributes, "(Ljava/util/Map<TA;TB;>;)V^Ljava/lang/RuntimeException;")
			case "duplicate constructor signature":
				for _, a := range method.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						method.Attributes = append(method.Attributes, a)
						break
					}
				}
			case "constructor checked throws":
				method.Attributes = append(method.Attributes, &ExceptionsAttribute{AttrLen: 4, ExceptionIndexTable: []uint16{uint16(parent.ConstantPoolManager.AddNewClassInfo("java/io/IOException"))}})
			case "duplicate empty throws":
				method.Attributes = append(method.Attributes, &ExceptionsAttribute{AttrLen: 2}, &ExceptionsAttribute{AttrLen: 2})
			case "foreign parent signature provider":
				old := d.foldSiblingResolver
				d.foldSiblingResolver = func(name string) ([]byte, bool) {
					raw, ok := old(name)
					if name == parent.GetClassName() {
						copy, err := Parse(bytes.Clone(raw))
						if err != nil {
							t.Fatal(err)
						}
						setSignature(copy, &copy.Attributes, "<A:Ljava/lang/Number;B:Ljava/lang/Object;>Ljava/lang/Object;")
						return copy.Bytes(), true
					}
					return raw, ok
				}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			params, known := d.nativeMethodLocalGenericDelegationParameters(parent, method, signature)
			if known != (kind == "original") {
				t.Fatalf("proof=%v params=%v", known, params)
			}
			if known && (len(params) != 1 || params[0].String(d.FuncCtx) != "Map<K, List<V>>") {
				t.Fatal("wrong instantiated constructor view", params)
			}
		})
	}
}

func TestNativeMethodLocalGenericDelegationRequiresClosedConstructorFamily(t *testing.T) {
	for _, row := range []struct {
		name, extra string
		want        bool
	}{
		{"unique", "", true},
		{"same arity public", "GenericParent(Object decoy){this((java.util.Map<A,B>)null);}", false},
		{"same arity method formal", "<X>GenericParent(X decoy){this((java.util.Map<A,B>)null);}", false},
		{"same arity variable arity", "GenericParent(Object... decoy){this((java.util.Map<A,B>)null);}", false},
		{"private rival", "private GenericParent(Object decoy){this((java.util.Map<A,B>)null);}", true},
		{"different fixed arity", "GenericParent(){this((java.util.Map<A,B>)null);}", true},
		{"later variable arity phase", "GenericParent(int tag,Object... rest){this((java.util.Map<A,B>)null);}", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			source := strings.Replace(localGenericParentSource, "Object alias(){return null;}", row.extra+"Object alias(){return null;}", 1)
			files := nativeCompileClasses(t, source)
			d, parent, method, signature := localGenericDelegationTestState(t, files)
			if _, known := d.nativeMethodLocalGenericDelegationParameters(parent, method, signature); known != row.want {
				t.Fatalf("family proof=%v want=%v", known, row.want)
			}
		})
	}
}

func TestNativeMethodLocalGenericParameterErasureKeepsDeclarationRole(t *testing.T) {
	for _, row := range []struct {
		signature, want string
		known           bool
	}{
		{"TT;", "Ljava/lang/Number;", true}, {"LT;", "LT;", true},
		{"[TT;", "[Ljava/lang/Number;", true}, {"[LT;", "[LT;", true},
		{"Ljava/util/Map<LT;TT;>;", "Ljava/util/Map;", true},
		{"TUnknown;", "", false}, {"J", "J", true}, {"V", "", false},
	} {
		t.Run(row.signature, func(t *testing.T) {
			erasure, known := nativeMethodLocalGenericParameterErasure(types.ParseSignature(row.signature), map[string]string{"T": "Ljava/lang/Number;"}, 0)
			if known != row.known || known && erasure != row.want {
				t.Fatal(erasure, known)
			}
		})
	}
}

func TestAdversarialLocalGenericParentKeepsListInstantiationAndCaptureOrder(t *testing.T) {
	source := strings.ReplaceAll(localGenericParentSource, "java.util.Map<A,B>", "java.util.List<A>")
	source = strings.ReplaceAll(source, "final java.util.Map<K,java.util.List<V>> input", "final java.util.List<K> input")
	source = strings.ReplaceAll(source, "java.util.Map<String,java.util.List<Integer>> input=nil?null:new java.util.LinkedHashMap<String,java.util.List<Integer>>();if(input!=null)input.put(\"alias\",token);", "java.util.List<String> input=nil?null:new java.util.ArrayList<String>();if(input!=null)input.add(\"alias\");")
	testResourceCompletionFamily(t, source, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
}
func TestAdversarialLocalGenericParentKeepsEnclosingMethodFormal(t *testing.T) {
	for _, variable := range []string{"M", "A"} {
		t.Run(variable, func(t *testing.T) {
			source := strings.Replace(localGenericParentSource, "Object make(final java.util.Map<K,java.util.List<V>> input)", "<"+variable+"> Object make(final java.util.Map<K,"+variable+"> input)", 1)
			source = strings.Replace(source, "Local extends GenericParent<K,java.util.List<V>>", "Local extends GenericParent<K,"+variable+">", 1)
			original := "!(actuals[1] instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)actuals[1]).getRawType()!=java.util.List.class||!((java.lang.reflect.ParameterizedType)actuals[1]).getActualTypeArguments()[0].equals(GenericOwner.class.getTypeParameters()[1])"
			replacement := "!actuals[1].equals(GenericOwner.class.getDeclaredMethod(\"make\",java.util.Map.class).getTypeParameters()[0])"
			if !strings.Contains(source, original) {
				t.Fatal("original generic declaration oracle")
			}
			source = strings.Replace(source, original, replacement, 1)
			testResourceCompletionFamily(t, source, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
		})
	}
}

func TestAdversarialLocalGenericParentKeepsBareFormalInput(t *testing.T) {
	source := strings.ReplaceAll(localGenericParentSource, "java.util.Map<A,B>", "A")
	source = strings.ReplaceAll(source, "final java.util.Map<K,java.util.List<V>> input", "final K input")
	source = strings.ReplaceAll(source, "java.util.Map<String,java.util.List<Integer>> input=nil?null:new java.util.LinkedHashMap<String,java.util.List<Integer>>();if(input!=null)input.put(\"alias\",token);", "String input=nil?null:new String(\"alias\");")
	testResourceCompletionFamily(t, source, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
}

func TestNativeMethodLocalGenericDelegationCannotNarrowExecutableErasure(t *testing.T) {
	const fixture = `class GenericParent<A,B>{GenericParent(A input){}} class GenericOwner<K,V>{Object make(final K input){class Local extends GenericParent<K,java.util.List<V>>{Local(){super(input);}Object read(){return input;}}return new Local();}}`
	for _, row := range []struct {
		name string
		want bool
	}{{"same erased formal", true}, {"narrower concrete argument", false}} {
		t.Run(row.name, func(t *testing.T) {
			source := fixture
			if !row.want {
				source = strings.Replace(source, "final K input", "final String input", 1)
				source = strings.Replace(source, "GenericParent<K,java.util.List<V>>", "GenericParent<String,java.util.List<V>>", 1)
			}
			d, parent, method, signature := localGenericDelegationTestState(t, nativeCompileClasses(t, source))
			if _, known := d.nativeMethodLocalGenericDelegationParameters(parent, method, signature); known != row.want {
				t.Fatal("executable erasure proof", known)
			}
		})
	}
}

func TestAdversarialLocalGenericParentRetainsNullHiddenEnclosingInstance(t *testing.T) {
	const reflectPort = `
 java.lang.reflect.Constructor<?> ctor=local.getDeclaredConstructors()[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();if(ports.length!=2||ports[0]!=GenericOwner.class||ports[1]!=java.util.Map.class)throw new AssertionError("original hidden ports");
 ParentEffects.published=null;Object reflected=null;try{reflected=ctor.newInstance(new Object[]{null,input});if(fail)throw new AssertionError("lost reflected parent failure");}catch(java.lang.reflect.InvocationTargetException wrapped){if(!fail||wrapped.getCause()!=ParentEffects.failure)throw new AssertionError("reflected exception identity",wrapped);}
 GenericParent<?,?> observed=ParentEffects.published;if(observed==null||observed.map!=input||observed.seenAlias!=input||observed.seenOwner!=null||!fail&&reflected!=observed)throw new AssertionError("nullable original enclosing word, pre-SUPER capture and argument order");
 `
	if strings.Count(localGenericParentSource, "rows++;") != 1 {
		t.Fatal("original driver continuation")
	}
	source := strings.Replace(localGenericParentSource, "rows++;", reflectPort+"rows++;", 1)
	testResourceCompletionFamily(t, source, "GenericOwner", "GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
}

func TestAdversarialInstanceLocalNestedAnonymousBinding(t *testing.T) {
	const source = `
class ChainRoot {
 class Layer {
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> make(final java.util.function.Supplier<Object> alias,final long seed){
  class Local {java.util.function.Supplier<java.util.function.Supplier<Object[]>> open(){return new java.util.function.Supplier<java.util.function.Supplier<Object[]>>(){public java.util.function.Supplier<Object[]> get(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed)};}};}};}}
  return new Local().open();
 }
 }
}
class ChainDriver {public static void main(String[]args){int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(final boolean failure:new boolean[]{false,true}){
 final Object token=new Object();final RuntimeException expected=new IllegalStateException("same");final int[] calls={0};
 java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(failure)throw expected;return token;}};
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> activation=new ChainRoot().new Layer().make(alias,seed);if(calls[0]!=0)throw new AssertionError("eager outer capture");
 java.util.function.Supplier<Object[]> first=activation.get(),second=activation.get();if(first==second||calls[0]!=0)throw new AssertionError("activation or eager leaf");
 for(java.util.function.Supplier<Object[]> leaf:new java.util.function.Supplier[]{first,second,first}){try{Object[] value=leaf.get();if(failure||value.length!=2||value[0]!=token||!(value[1]instanceof Long)||((Long)value[1]).longValue()!=seed)throw new AssertionError("binding,identity,wide word");}catch(RuntimeException actual){if(!failure||actual!=expected)throw new AssertionError("failure identity",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");
 Class<?> level1=activation.getClass(),level2=first.getClass();Class<?> local=level1.getEnclosingClass();if(!level1.isAnonymousClass()||!level2.isAnonymousClass()||local==null||!local.isLocalClass()||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!level1.getEnclosingMethod().getName().equals("open")||level2.getEnclosingClass()!=level1||!level2.getEnclosingMethod().getName().equals("get"))throw new AssertionError("actual lexical owner chain");rows++;
 }System.out.println(rows+":local:anonymous:anonymous:activation:identity:wide:deferred");}}
`
	testResourceCompletionFamily(t, source, "ChainRoot", "ChainDriver", "10:local:anonymous:anonymous:activation:identity:wide:deferred\n")
}

func TestAdversarialInstanceLocalNestedPreSuperPublication(t *testing.T) {
	const source = `
abstract class EarlyBase implements java.util.function.Supplier<Object[]> {
 static Object published,observed;static long observedWord;static int publications;
 static final RuntimeException failure=new IllegalArgumentException("same");
 EarlyBase(int marker){published=this;publications++;Object[] row=get();observed=row[0];observedWord=((Long)row[1]).longValue();if(marker!=0)throw failure;}
}
class EarlyRoot {
 class Layer {
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> make(final java.util.function.Supplier<Object> alias,final long seed,final int marker){
  class Local {java.util.function.Supplier<java.util.function.Supplier<Object[]>> open(){return new java.util.function.Supplier<java.util.function.Supplier<Object[]>>(){public java.util.function.Supplier<Object[]> get(){return new EarlyBase(marker){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed)};}};}};}}
  return new Local().open();
 }
 }
}
class EarlyDriver {public static void main(String[]args){int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int marker=0;marker<2;marker++){
 final Object token=new Object();final int[] calls={0};java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;return token;}};
 EarlyBase.published=null;EarlyBase.observed=null;EarlyBase.observedWord=0;EarlyBase.publications=0;
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> activation=new EarlyRoot().new Layer().make(alias,seed,marker);if(calls[0]!=0||EarlyBase.publications!=0)throw new AssertionError("eager activation");
 java.util.function.Supplier<Object[]> leaf=null;try{leaf=activation.get();if(marker!=0)throw new AssertionError("lost constructor failure");}catch(RuntimeException actual){if(marker==0||actual!=EarlyBase.failure)throw new AssertionError("constructor failure identity",actual);}
 if(EarlyBase.publications!=1||EarlyBase.published==null||EarlyBase.observed!=token||EarlyBase.observedWord!=seed||calls[0]!=1)throw new AssertionError("pre-super capture storage and callback order");
 if(marker==0){if(leaf!=EarlyBase.published)throw new AssertionError("published receiver identity");Object[] value=leaf.get();if(value[0]!=token||((Long)value[1]).longValue()!=seed||calls[0]!=2)throw new AssertionError("retained binding");}
 Class<?> c=EarlyBase.published.getClass();if(!c.isAnonymousClass()||c.getEnclosingClass()!=activation.getClass()||c.getEnclosingMethod()==null||!c.getEnclosingMethod().getName().equals("get"))throw new AssertionError("published lexical identity");rows++;
 }System.out.println(rows+":nested:local:pre-super:capture:publication:callback:abrupt");}}
`
	testResourceCompletionFamily(t, source, "EarlyRoot", "EarlyDriver", "10:nested:local:pre-super:capture:publication:callback:abrupt\n")
}

func TestNativeInstanceLocalAnonymousEnclosingWordRequiresOriginalScope(t *testing.T) {
	source := `class InstanceOwner {class Layer {Object make(final Object alias){class Local {Object open(){return new Object(){Object value(){return alias;}};}}return new Local().open();}}}`
	originals := nativeCompileSourceReleaseClasses(t, map[string]string{"InstanceOwner.java": source}, "none", "8")
	for _, mode := range []string{"original", "missing local role", "foreign member role", "different local object", "cached enclosing field", "cached enclosing port", "cached source super", "changed method owner", "missing declaring owner", "failed candidate", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, originals)
			defer z.Close()
			root, e := Parse(originals["InstanceOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original named/local plan")
			}
			local := p.methodLocals["InstanceOwner$Layer$1Local"]
			if local == nil {
				t.Fatal("original local role")
			}
			child, e := Parse(originals["InstanceOwner$Layer$1Local$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			reader := NewClassObjectDumper(local.object)
			reader.options = d.options
			reader.Work = d.Work
			reader.foldSiblingResolver = d.foldSiblingResolver
			reader.declarationResolver = d.declarationResolver
			switch mode {
			case "missing local role":
				delete(p.methodLocals, local.object.GetClassName())
			case "foreign member role":
				p.children[local.object.GetClassName()] = &nativeMemberClass{object: local.object, owner: local.owner.owner, name: "Local", field: "this$1"}
			case "different local object":
				copy, e := Parse(originals["InstanceOwner$Layer$1Local.class"])
				if e != nil {
					t.Fatal(e)
				}
				local.object = copy
			case "cached enclosing field":
				local.constructor.enclosingField = "this$63"
			case "cached enclosing port":
				local.constructor.captures[local.constructor.enclosingField] = 1
			case "cached source super":
				local.constructor.delegateDescriptor = "(Ljava/lang/Object;)V"
			case "changed method owner":
				local.owner.method = "other"
			case "missing declaring owner":
				delete(p.lexicalObjects, local.owner.owner)
			case "failed candidate":
				p.failed = true
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			owner, method, known := originalAnonymousOwner(child)
			if !known {
				t.Fatal("original anonymous owner")
			}
			packet := reader.nativeAnonymousConstructorForCompiler(child, owner, method, p.owner, p, nil, reader.buildInvocationMetadata(), reader.originalNativeConstructorAccessBridges())
			if (packet != nil) != (mode == "original") {
				t.Fatalf("scope/port proof accepted=%v", packet != nil)
			}
			if packet != nil && (packet.enclosingField != "this$2" || packet.fields[packet.enclosingField] != 0) {
				t.Fatal("original hidden receiver word changed")
			}
		})
	}
}

func TestAdversarialLocalEnclosingReadsPreserveLiveStateAndNullPorts(t *testing.T) {
	const source = `
class ViewRoot {volatile int root;class Layer {volatile int layer;
 java.util.function.Supplier<Object[]> make(final java.util.function.Supplier<Object> alias,final long seed){class Local {java.util.function.Supplier<Object[]> open(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed),Integer.valueOf(ViewRoot.this.root),Integer.valueOf(Layer.this.layer)};}};}}return new Local().open();}
}}
class ViewDriver {public static void main(String[]args)throws Exception{int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean nullOuter:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true}){
 ViewRoot root=new ViewRoot();ViewRoot.Layer layer=root.new Layer();final Object token=new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");
 java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object[]> ordinary=layer.make(alias,seed);if(calls[0]!=0)throw new AssertionError("eager capture");
 Class<?> local=Class.forName("ViewRoot$Layer$1Local");java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("constructor identity");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] values=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==ViewRoot.Layer.class)values[i]=nullOuter?null:layer;else if(ports[i]==java.util.function.Supplier.class)values[i]=alias;else if(ports[i]==long.class)values[i]=Long.valueOf(seed);else throw new AssertionError("unexpected hidden word");}
 Object instance=ctor.newInstance(values);java.lang.reflect.Method open=local.getDeclaredMethod("open");open.setAccessible(true);java.util.function.Supplier<Object[]> leaf=(java.util.function.Supplier<Object[]>)open.invoke(instance);if(calls[0]!=0||leaf.getClass()!=ordinary.getClass())throw new AssertionError("deferred shared class");
 for(int step=0;step<3;step++){root.root=101+step;layer.layer=-23-step;try{Object[] row=leaf.get();if(fail||nullOuter||row.length!=4||row[0]!=token||((Long)row[1]).longValue()!=seed||((Integer)row[2]).intValue()!=101+step||((Integer)row[3]).intValue()!=-23-step)throw new AssertionError("live enclosing field state");}catch(RuntimeException actual){if(fail){if(actual!=expected)throw new AssertionError("left-to-right exception identity",actual);}else if(!nullOuter||actual.getClass()!=NullPointerException.class)throw new AssertionError("null enclosing domain",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");if(!local.isLocalClass()||local.getEnclosingClass()!=ViewRoot.Layer.class||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!leaf.getClass().isAnonymousClass()||leaf.getClass().getEnclosingClass()!=local)throw new AssertionError("actual lexical declarations");rows++;
 }System.out.println(rows+":live-enclosing:volatile:null-hidden-word:deferred:identity");}}
`
	testResourceCompletionFamily(t, source, "ViewRoot", "ViewDriver", "20:live-enclosing:volatile:null-hidden-word:deferred:identity\n")
}

func TestNativeLocalEnclosingQualifierRequiresScopedReceiverAndNamespace(t *testing.T) {
	source := `class NameRoot {int value;class Layer {java.util.function.Supplier<Object> make(final Object token){class Local {java.util.function.Supplier<Object> open(){return new java.util.function.Supplier<Object>(){public Object get(){return new Object[]{token,Integer.valueOf(NameRoot.this.value)};}};}}return new Local().open();}}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"NameRoot.java": source}, "none", "8")
	for _, mode := range []string{"original", "missing source binder", "different source ID", "nonreceiver binder", "unsealed receiver", "unrelated qualifier", "wrong member suffix", "type parameter shadow", "cached local role", "different owner definition", "missing intermediate class", "failed candidate", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["NameRoot.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original lexical plan")
			}
			local := p.methodLocals["NameRoot$Layer$1Local"]
			group := p.anonymousUnits["NameRoot$Layer$1Local$1"]
			if local == nil || group == nil {
				t.Fatal("local and anonymous roles")
			}
			child := group.children["NameRoot$Layer$1Local$1"]
			binder := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(local.owner.owner))
			binder.IsThis = true
			binder.IsParam = true
			binder.MarkOriginalParameter(0)
			if mode != "unsealed receiver" {
				binder.MarkOriginalReceiver()
			}
			field := local.constructor.enclosingField
			local.sourceRefs = map[string]*values.JavaRef{field: binder}
			local.captureIDs = map[string]*coreutils.VariableId{field: binder.Id}
			local.bindings = map[string]string{field: "NameRoot.Layer.this"}
			c := NewClassObjectDumper(child.object)
			c.options = d.options
			c.Work = d.Work
			c.foldSiblingResolver = d.foldSiblingResolver
			c.declarationResolver = d.declarationResolver
			c.nativeMemberRoot = p
			c.nativeAnonymousForest = p.anonymousForest
			c.FuncCtx = &class_context.ClassContext{ClassName: child.object.GetClassName(), FunctionName: "get", CurrentMethodDesc: "()Ljava/lang/Object;"}
			switch mode {
			case "missing source binder":
				delete(local.sourceRefs, field)
			case "different source ID":
				local.sourceRefs[field] = values.NewJavaRef(coreutils.NewRootVariableId(), nil, binder.Type())
			case "nonreceiver binder":
				binder.IsThis = false
			case "unrelated qualifier":
				local.bindings[field] = "Other.Layer.this"
			case "wrong member suffix":
				local.bindings[field] = "NameRoot.Other.this"
			case "type parameter shadow":
				c.FuncCtx.TypeParams = []string{"NameRoot"}
			case "cached local role":
				local.constructor.enclosingField = "this$63"
			case "different owner definition":
				copy, e := Parse(files["NameRoot$Layer.class"])
				if e != nil {
					t.Fatal(e)
				}
				p.lexicalObjects[local.owner.owner] = copy
			case "missing intermediate class":
				delete(p.lexicalObjects, local.object.GetClassName())
			case "failed candidate":
				p.failed = true
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			text, known := c.nativeMethodLocalAnonymousEnclosingName(local, c.FuncCtx)
			if known != (mode == "original") {
				t.Fatalf("qualified receiver permission=%v text=%q", known, text)
			}
			if known && text != "NameRoot.Layer.this" {
				t.Fatal("source declaration identity changed")
			}
		})
	}
}

func TestAdversarialLocalEnclosingComparisonKeepsNullableHiddenWords(t *testing.T) {
	const source = `
class ViewRoot {volatile int root;class Layer {volatile int layer;
 java.util.function.Supplier<Object[]> make(final java.util.function.Supplier<Object> alias,final long seed){class Local {java.util.function.Supplier<Object[]> open(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){if(Layer.this==null)return new Object[]{alias.get(),Long.valueOf(seed),"null-layer",Integer.valueOf(-1)};return new Object[]{alias.get(),Long.valueOf(seed),Integer.valueOf(ViewRoot.this.root),Integer.valueOf(Layer.this.layer)};}};}}return new Local().open();}
}}
class ViewDriver {public static void main(String[]args)throws Exception{int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean nullOuter:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true}){
 ViewRoot root=new ViewRoot();ViewRoot.Layer layer=root.new Layer();final Object token=new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");
 java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object[]> ordinary=layer.make(alias,seed);if(calls[0]!=0)throw new AssertionError("eager capture");
 Class<?> local=Class.forName("ViewRoot$Layer$1Local");java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("constructor identity");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] values=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==ViewRoot.Layer.class)values[i]=nullOuter?null:layer;else if(ports[i]==java.util.function.Supplier.class)values[i]=alias;else if(ports[i]==long.class)values[i]=Long.valueOf(seed);else throw new AssertionError("unexpected hidden word");}
 Object instance=ctor.newInstance(values);java.lang.reflect.Method open=local.getDeclaredMethod("open");open.setAccessible(true);java.util.function.Supplier<Object[]> leaf=(java.util.function.Supplier<Object[]>)open.invoke(instance);if(calls[0]!=0||leaf.getClass()!=ordinary.getClass())throw new AssertionError("deferred shared class");
 for(int step=0;step<3;step++){root.root=101+step;layer.layer=-23-step;try{Object[] row=leaf.get();if(fail||row.length!=4||row[0]!=token||((Long)row[1]).longValue()!=seed)throw new AssertionError("captured state");if(nullOuter){if(!"null-layer".equals(row[2])||((Integer)row[3]).intValue()!=-1)throw new AssertionError("nullable enclosing word was assumed non-null");}else if(((Integer)row[2]).intValue()!=101+step||((Integer)row[3]).intValue()!=-23-step)throw new AssertionError("live enclosing field state");}catch(RuntimeException actual){if(fail){if(actual!=expected)throw new AssertionError("left-to-right exception identity",actual);}else throw new AssertionError("unexpected null-guard failure",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");if(!local.isLocalClass()||local.getEnclosingClass()!=ViewRoot.Layer.class||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!leaf.getClass().isAnonymousClass()||leaf.getClass().getEnclosingClass()!=local)throw new AssertionError("actual lexical declarations");rows++;
 }System.out.println(rows+":enclosing-null-comparison:volatile:deferred:identity");}}
`
	testResourceCompletionFamily(t, source, "ViewRoot", "ViewDriver", "20:enclosing-null-comparison:volatile:deferred:identity\n")
}

const packagedLocalEnclosingSource = `
class ViewRoot {volatile int root;class Layer {volatile int layer;
 java.util.function.Supplier<Object[]> make(final java.util.function.Supplier<Object> alias,final long seed){class Local {java.util.function.Supplier<Object[]> open(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed),Integer.valueOf(ViewRoot.this.root),Integer.valueOf(Layer.this.layer)};}};}}return new Local().open();}
}}
class ViewDriver {public static void main(String[]args)throws Exception{int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(boolean nullOuter:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true}){
 ViewRoot root=new ViewRoot();ViewRoot.Layer layer=root.new Layer();final Object token=new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");
 java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object[]> ordinary=layer.make(alias,seed);if(calls[0]!=0)throw new AssertionError("eager capture");
 Class<?> local=Class.forName("ViewRoot$Layer$1Local");java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("constructor identity");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] values=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==ViewRoot.Layer.class)values[i]=nullOuter?null:layer;else if(ports[i]==java.util.function.Supplier.class)values[i]=alias;else if(ports[i]==long.class)values[i]=Long.valueOf(seed);else throw new AssertionError("unexpected hidden word");}
 Object instance=ctor.newInstance(values);java.lang.reflect.Method open=local.getDeclaredMethod("open");open.setAccessible(true);java.util.function.Supplier<Object[]> leaf=(java.util.function.Supplier<Object[]>)open.invoke(instance);if(calls[0]!=0||leaf.getClass()!=ordinary.getClass())throw new AssertionError("deferred shared class");
 for(int step=0;step<3;step++){root.root=101+step;layer.layer=-23-step;try{Object[] row=leaf.get();if(fail||nullOuter||row.length!=4||row[0]!=token||((Long)row[1]).longValue()!=seed||((Integer)row[2]).intValue()!=101+step||((Integer)row[3]).intValue()!=-23-step)throw new AssertionError("live enclosing field state");}catch(RuntimeException actual){if(fail){if(actual!=expected)throw new AssertionError("left-to-right exception identity",actual);}else if(!nullOuter||actual.getClass()!=NullPointerException.class)throw new AssertionError("null enclosing domain",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");if(!local.isLocalClass()||local.getEnclosingClass()!=ViewRoot.Layer.class||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!leaf.getClass().isAnonymousClass()||leaf.getClass().getEnclosingClass()!=local)throw new AssertionError("actual lexical declarations");rows++;
 }System.out.println(rows+":live-enclosing:volatile:null-hidden-word:deferred:identity");}}
`

func TestAdversarialPackagedLocalEnclosingReadsKeepLiveStateAndNullPorts(t *testing.T) {
	for _, methodFormal := range []bool{false, true} {
		t.Run(map[bool]string{false: "packaged", true: "package-like method formal"}[methodFormal], func(t *testing.T) {
			source := "package probe.bindings;\n" + strings.Replace(packagedLocalEnclosingSource, "Class.forName(\"ViewRoot$Layer$1Local\")", "Class.forName(\"probe.bindings.ViewRoot$Layer$1Local\")", 1)
			if methodFormal {
				source = strings.Replace(source, "java.util.function.Supplier<Object[]> make(", "<probe> java.util.function.Supplier<Object[]> make(", 1)
			}
			testResourceCompletionFamily(t, source, "probe/bindings/ViewRoot", "probe.bindings.ViewDriver", "20:live-enclosing:volatile:null-hidden-word:deferred:identity\n")
		})
	}
}

func TestNativePackagedLocalEnclosingQualifierRequiresOriginalOwnerAndNamespace(t *testing.T) {
	source := `package proof.one;class NameRoot {int value;class Layer {java.util.function.Supplier<Object> make(final Object token){class Local {java.util.function.Supplier<Object> open(){return new java.util.function.Supplier<Object>(){public Object get(){return new Object[]{token,Integer.valueOf(NameRoot.this.value)};}};}}return new Local().open();}}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"NameRoot.java": source}, "none", "8")
	for _, mode := range []string{"original", "package-like formal", "same-spelled member formal", "lexical name shadow", "wrong package", "binary member spelling", "missing source binder", "different source ID", "nonreceiver binder", "unsealed receiver", "unrelated qualifier", "wrong member suffix", "type parameter shadow", "cached local role", "different owner definition", "missing intermediate class", "failed candidate", "budget", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["proof/one/NameRoot.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original lexical plan")
			}
			local := p.methodLocals["proof/one/NameRoot$Layer$1Local"]
			group := p.anonymousUnits["proof/one/NameRoot$Layer$1Local$1"]
			if local == nil || group == nil {
				t.Fatal("local and anonymous roles")
			}
			child := group.children["proof/one/NameRoot$Layer$1Local$1"]
			binder := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(local.owner.owner))
			binder.IsThis = true
			binder.IsParam = true
			binder.MarkOriginalParameter(0)
			if mode != "unsealed receiver" {
				binder.MarkOriginalReceiver()
			}
			field := local.constructor.enclosingField
			local.sourceRefs = map[string]*values.JavaRef{field: binder}
			local.captureIDs = map[string]*coreutils.VariableId{field: binder.Id}
			local.bindings = map[string]string{field: "proof.one.NameRoot.Layer.this"}
			c := NewClassObjectDumper(child.object)
			c.options = d.options
			c.Work = d.Work
			c.foldSiblingResolver = d.foldSiblingResolver
			c.declarationResolver = d.declarationResolver
			c.nativeMemberRoot = p
			c.nativeAnonymousForest = p.anonymousForest
			c.FuncCtx = &class_context.ClassContext{ClassName: child.object.GetClassName(), FunctionName: "get", CurrentMethodDesc: "()Ljava/lang/Object;"}
			switch mode {
			case "package-like formal":
				c.FuncCtx.TypeParams = []string{"proof"}
			case "same-spelled member formal":
				c.FuncCtx.TypeParams = []string{"Layer"}
			case "lexical name shadow":
				c.FuncCtx.LexicalTypeNames = map[string]bool{"NameRoot": true}
			case "wrong package":
				local.bindings[field] = "foreign.one.NameRoot.Layer.this"
			case "binary member spelling":
				local.bindings[field] = "proof.one.NameRoot$Layer.this"
			case "missing source binder":
				delete(local.sourceRefs, field)
			case "different source ID":
				local.sourceRefs[field] = values.NewJavaRef(coreutils.NewRootVariableId(), nil, binder.Type())
			case "nonreceiver binder":
				binder.IsThis = false
			case "unrelated qualifier":
				local.bindings[field] = "Other.Layer.this"
			case "wrong member suffix":
				local.bindings[field] = "NameRoot.Other.this"
			case "type parameter shadow":
				c.FuncCtx.TypeParams = []string{"NameRoot"}
			case "cached local role":
				local.constructor.enclosingField = "this$63"
			case "different owner definition":
				copy, e := Parse(files["proof/one/NameRoot$Layer.class"])
				if e != nil {
					t.Fatal(e)
				}
				p.lexicalObjects[local.owner.owner] = copy
			case "missing intermediate class":
				delete(p.lexicalObjects, local.object.GetClassName())
			case "failed candidate":
				p.failed = true
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			text, known := c.nativeMethodLocalAnonymousEnclosingName(local, c.FuncCtx)
			if known != (mode == "original" || mode == "package-like formal" || mode == "same-spelled member formal") {
				t.Fatalf("qualified receiver permission=%v text=%q", known, text)
			}
			if known && text != "NameRoot.Layer.this" {
				t.Fatal("source declaration identity changed")
			}
		})
	}
}

func TestAdversarialPackagedGenericLocalAnonymousKeepsObservedSuperAndEnclosingGraph(t *testing.T) {
	for _, liveParent := range []bool{false, true} {
		t.Run(map[bool]string{false: "captured map", true: "live enclosing map"}[liveParent], func(t *testing.T) {
			source := localGenericParentSource
			source = strings.Replace(source, " Object make(", " class Layer extends java.util.LinkedHashMap<K,java.util.List<V>> { Object make(", 1)
			source = strings.Replace(source, "return new Local();}\n}", "return new Local();}}\n}", 1)
			source = strings.Replace(source, "Object owner(){return GenericOwner.this;}", "Object owner(){return Layer.this;}Object open(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{input,Layer.this,GenericOwner.this};}};}", 1)
			source = strings.Replace(source, "GenericOwner<String,Integer> owner=new GenericOwner<String,Integer>();", "GenericOwner<String,Integer> owner=new GenericOwner<String,Integer>();GenericOwner<String,Integer>.Layer layer=owner.new Layer();", 1)
			source = strings.ReplaceAll(source, "owner.make(input)", "layer.make(input)")
			source = strings.ReplaceAll(source, "parent.seenOwner!=owner", "parent.seenOwner!=layer")
			source = strings.ReplaceAll(source, "local.getEnclosingClass()!=GenericOwner.class", "local.getEnclosingClass()!=GenericOwner.Layer.class")
			const deferred = `java.lang.reflect.Method open=local.getDeclaredMethod("open");open.setAccessible(true);java.util.function.Supplier<Object[]> leaf=(java.util.function.Supplier<Object[]>)open.invoke(parent);Object[] snapshot=leaf.get();if(snapshot.length!=3||snapshot[0]!=input||snapshot[1]!=layer||snapshot[2]!=owner||!leaf.getClass().isAnonymousClass()||leaf.getClass().getEnclosingClass()!=local)throw new AssertionError("deferred post-SUPER failure capture, named/local/anonymous owner graph");`
			if strings.Count(source, "rows++;") != 1 {
				t.Fatal("original driver continuation")
			}
			source = strings.Replace(source, "rows++;", deferred+"rows++;", 1)
			if liveParent {
				source = strings.ReplaceAll(source, "super(input);", "super(Layer.this);")
				source = strings.ReplaceAll(source, "parent.map!=input", "parent.map!=layer")
			}
			source = "package trial.boundary;\n" + source
			testResourceCompletionFamily(t, source, "trial/boundary/GenericOwner", "trial.boundary.GenericDriver", "4:generic-local-super:pre-super-capture:identity:failure:binding\n")
		})
	}
}

func TestAdversarialLocalMethodVariablesCannotEscapeThroughSiblingParameterNames(t *testing.T) {
	const source = `
class ScopeOwner {
 java.util.function.Supplier<Object> make(final java.util.Iterator<java.util.Map.Entry<Object,Object>> input){
  class Local implements java.util.function.Supplier<Object> {
   public Object get(){do{if(input.hasNext()){java.util.Map.Entry<Object,Object> entry=input.next();Object key=entry.getKey();if(key!=null)return entry.getValue();}else break;}while(true);return null;}
   boolean independent(java.util.Collection<?> other){return other.isEmpty();}
  }
  return new Local();
 }
}
class ScopeDriver {
 public static void main(String[]args)throws Exception {int rows=0;
  for(int selected:new int[]{-1,0,1,2})for(final boolean fail:new boolean[]{false,true}){
   final Object expected=new Object();final RuntimeException failure=new IllegalStateException("same");final int[] calls={0,0};final java.util.List<java.util.Map.Entry<Object,Object>> data=new java.util.ArrayList<java.util.Map.Entry<Object,Object>>();
   for(int i=0;i<3;i++)data.add(new java.util.AbstractMap.SimpleImmutableEntry<Object,Object>(i==selected?new Object():null,expected));
   java.util.Iterator<java.util.Map.Entry<Object,Object>> cursor=new java.util.Iterator<java.util.Map.Entry<Object,Object>>(){int at;public boolean hasNext(){calls[0]++;return at<data.size();}public java.util.Map.Entry<Object,Object> next(){calls[1]++;if(fail)throw failure;return data.get(at++);}public void remove(){throw new UnsupportedOperationException();}};
   java.util.function.Supplier<Object> result=new ScopeOwner().make(cursor);if(calls[0]!=0||calls[1]!=0)throw new AssertionError("eager iteration");Object value=null;
   try{value=result.get();if(fail)throw new AssertionError("lost original failure");}catch(RuntimeException actual){if(!fail||actual!=failure)throw new AssertionError("exception identity",actual);}
   int consumed=fail?1:selected<0?3:selected+1;int queried=fail?1:selected<0?4:selected+1;
   if(!fail&&value!=(selected<0?null:expected)||calls[0]!=queried||calls[1]!=consumed)throw new AssertionError("iterator order, count and value");
   Class<?> local=result.getClass();if(!local.isLocalClass()||local.getEnclosingClass()!=ScopeOwner.class||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("actual local declaration");java.lang.reflect.Method independent=local.getDeclaredMethod("independent",java.util.Collection.class);independent.setAccessible(true);if(!Boolean.TRUE.equals(independent.invoke(result,java.util.Collections.emptyList()))||calls[0]!=queried||calls[1]!=consumed)throw new AssertionError("independent method frame");rows++;
  }System.out.println(rows+":local:independent-method-slots:iteration:identity:failure");
 }
}`
	testResourceCompletionFamily(t, source, "ScopeOwner", "ScopeDriver", "8:local:independent-method-slots:iteration:identity:failure\n")
}

func TestNativeLocalDeclarationScopeKeepsNestedFramesOpaque(t *testing.T) {
	const declaration = "class Local<T> {\n Object get(){while(true){Object var1=capture;return var1;}}\n boolean sibling(java.util.Collection<?> var1){return var1.isEmpty();}\n}"
	for _, row := range []struct {
		name, body, want string
		owned            []string
	}{
		{"ordinary", "Object outer=capture;\n" + declaration + "\nreturn new Local();", "Object outer=changed;\n" + declaration + "\nreturn new Local();", []string{declaration}},
		{"same declaration in comment", "/*" + declaration + "*/\n" + declaration, "/*" + strings.ReplaceAll(declaration, "capture", "changed") + "*/\n" + declaration, []string{declaration}},
		{"same declaration in string", "String text=\"" + strings.ReplaceAll(declaration, "\n", "\\n") + "\";\n" + declaration, "String text=\"" + strings.ReplaceAll(strings.ReplaceAll(declaration, "capture", "changed"), "\n", "\\n") + "\";\n" + declaration, []string{declaration}},
		{"nested declarations", "class Outer {" + declaration + "}", "class Outer {" + declaration + "}", []string{declaration, "class Outer {" + declaration + "}"}},
		{"sibling declarations", declaration + "\n" + strings.ReplaceAll(declaration, "Local", "Other"), declaration + "\n" + strings.ReplaceAll(declaration, "Local", "Other"), []string{strings.ReplaceAll(declaration, "Local", "Other"), declaration}},
		{"unregistered text", declaration, strings.ReplaceAll(declaration, "capture", "changed"), nil},
		{"incomplete registered declaration", declaration, declaration, []string{strings.TrimSuffix(declaration, "}")}},
		{"foreign registered grammar", declaration, declaration, []string{"new Local(){}"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			got := nativeRewriteEnclosingScope(row.body, func(s string) string { return strings.ReplaceAll(s, "capture", "changed") }, row.owned...)
			if got != row.want {
				t.Fatalf("scope rewrite:\n%s\nwant:\n%s", got, row.want)
			}
		})
	}
	for _, kind := range []string{"lost", "duplicated", "collision"} {
		t.Run(kind, func(t *testing.T) {
			source := declaration
			if kind == "collision" {
				source += "\n/*jdec lexical declaration 0*/"
			}
			got := nativeRewriteEnclosingScope(source, func(s string) string {
				if kind == "duplicated" {
					return s + s
				}
				return strings.ReplaceAll(s, "/*jdec lexical declaration 0*/", "")
			}, declaration)
			if got != source {
				t.Fatal("accepted missing, duplicated or ambiguous declaration receipt")
			}
		})
	}
}

func TestNativeLocalDeclarationProtectionKeepsOuterRepairAndResourceBoundary(t *testing.T) {
	const declaration = "class Local {\n Object get(){while(true){Object var1=capture;return var1;}}\n boolean sibling(java.util.Collection<?> var1){return var1.isEmpty();}\n}"
	const outer = "\tif(flag){\n\t\tObject var2 = make();\n\t}\n\treturn var2;\n"
	got := nativeRewriteEnclosingScope(outer+declaration, hoistSameTypeEscapedLocals, declaration)
	if !strings.Contains(got, "Object var2 = null;") || !strings.Contains(got, "var2 = make();") || strings.Count(got, declaration) != 1 || strings.Contains(got, "Object var1 = null;") {
		t.Fatalf("outer/current and nested frames were merged:\n%s", got)
	}
	for _, kind := range []string{"ordinary", "work", "memory", "canceled", "precision"} {
		t.Run(kind, func(t *testing.T) {
			d := NewClassObjectDumper(&ClassObject{})
			d.nativeMemberRoot = &nativeMemberFamily{methodLocals: map[string]*nativeMethodLocalClass{"Local": {object: &ClassObject{}, owner: &nativeMethodLocalOwner{}, source: declaration}}}
			switch kind {
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "precision":
				d.options.Mode = Precision
			}
			calls := 0
			source := "Object outer=capture;\n" + declaration
			got := d.sourceRewrite("test-owned-frame", "class_source", source, func(s string) string { calls++; return strings.ReplaceAll(s, "capture", "changed") })
			want := source
			if kind == "ordinary" {
				want = "Object outer=changed;\n" + declaration
			}
			if got != want || calls != map[bool]int{true: 1, false: 0}[kind == "ordinary"] {
				t.Fatalf("resource/ownership boundary kind=%s calls=%d\n%s", kind, calls, got)
			}
		})
	}
}
