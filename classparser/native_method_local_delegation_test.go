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
