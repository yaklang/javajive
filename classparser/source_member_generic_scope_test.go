package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialMemberScopePublicationRequiresFinalSource(t *testing.T) {
	_, _, artifact := constructorSuffixFamilyRoundTrip(t, externalOwnerBoundFixture("Number"), "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Member"
	source, err := artifact.ReadFile(owner + ".class")
	if err != nil {
		t.Fatal(err)
	}
	origin, known := base.genericActive[owner]
	if !known || origin.sourceHash != sha256.Sum256(source) {
		t.Fatal("exact final source receipt")
	}
	a := &sourceConstructorLoweringAuthority{jar: base.jar, genericPending: map[string]constructorNativeSourceOrigin{owner: origin}}
	changed := append(bytes.Clone(source), '\n')
	if err := a.sourceReturned(owner, changed); err != nil || len(a.genericActive) != 0 {
		t.Fatal("changed source gained scope authority", err)
	}
	if err := a.sourceReturned(owner, source); err != nil || a.genericActive[owner] != origin {
		t.Fatal("complete source not activated", err)
	}
	if err := a.sourceReturned(owner, changed); err == nil || a.genericActive[owner] != origin {
		t.Fatal("active scope receipt changed")
	}
	conflict := &sourceConstructorLoweringAuthority{
		jar:            base.jar,
		genericPending: map[string]constructorNativeSourceOrigin{owner: origin},
		bindingActive:  map[string]constructorNativeSourceOrigin{owner: origin},
	}
	if err := conflict.sourceReturned(owner, source); err == nil || len(conflict.genericActive) != 0 {
		t.Fatal("another source obligation allowed partial scope activation")
	}
}

func TestAdversarialMemberScopeCompletionRequiresClosedFreshDeclarations(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, externalOwnerBoundFixture("Number"), "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Member"
	origin, known := base.genericActive[owner]
	if !known {
		t.Fatal("completed source scope receipt")
	}
	for _, variant := range []string{"control", "opaque metadata", "idempotent", "parent inherited", "unreturned", "pending", "pending shadows parent", "foreign namespace", "stale original", "wrong bound", "extra bound", "missing signature", "duplicate signature", "wrong target", "foreign membership", "missing method", "duplicate method", "changed constructor", "extra field", "class type annotation", "budget", "canceled", "output budget"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, genericActive: map[string]constructorNativeSourceOrigin{owner: origin}}
			old := bytes.Clone(original)
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var sig *SignatureAttribute
			var ctor *MemberInfo
			for _, attr := range obj.Attributes {
				if s, ok := attr.(*SignatureAttribute); ok {
					sig = s
				}
			}
			for _, m := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
					ctor = m
				}
			}
			if sig == nil || ctor == nil {
				t.Fatal("fresh generic declaration and constructor")
			}
			var work *workbudget.Budget
			switch variant {
			case "opaque metadata":
				obj.ConstantPoolManager.AddUtf8Info("OpaqueScopeData")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "OpaqueScopeData", Length: 3, Info: []byte{5, 7, 9}})
			case "unreturned":
				a.genericActive = nil
			case "pending":
				a.genericPending, a.genericActive = a.genericActive, nil
			case "parent inherited":
				a.parent, a.genericActive = base, nil
			case "pending shadows parent":
				a.parent, a.genericActive = base, nil
				a.genericPending = map[string]constructorNativeSourceOrigin{owner: {}}
			case "foreign namespace":
				a.parent, a.genericActive = base, nil
				a.jar = &JarFS{sourceCompiler: ModernJavac, targetSourceVersion: 11}
			case "stale original":
				before := mustParseLowering(t, old)
				before.ConstantPoolManager.AddUtf8Info("different immutable original")
				old = before.Bytes()
			case "wrong bound":
				sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Object;>Ljava/lang/Object;"))
			case "extra bound":
				sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<T:Ljava/lang/Number;:Ljava/io/Serializable;>Ljava/lang/Object;"))
			case "missing signature":
				attrs := obj.Attributes[:0]
				for _, attr := range obj.Attributes {
					if attr != sig {
						attrs = append(attrs, attr)
					}
				}
				obj.Attributes = attrs
			case "duplicate signature":
				obj.Attributes = append(obj.Attributes, sig)
			case "wrong target":
				obj.MajorVersion++
			case "foreign membership":
				obj.ConstantPoolManager.AddUtf8Info("InnerClasses")
				name := uint16(obj.ConstantPoolManager.AddUtf8Info("Member"))
				obj.Attributes = append(obj.Attributes, &InnerClassesAttribute{AttrLen: 10, NumberOfClasses: 1, Classes: []*InnerClassInfo{{InnerClassInfoIndex: obj.ThisClass, OuterClassInfoIndex: obj.SuperClass, InnerNameIndex: name}}})
			case "missing method":
				obj.Methods = []*MemberInfo{ctor}
			case "duplicate method":
				obj.Methods = append(obj.Methods, ctor)
			case "changed constructor":
				code := constructorLoweringCode(ctor)
				code.Code[len(code.Code)-1] = 0xbf // ATHROW is not the proved RETURN.
			case "extra field":
				copy := *obj.Fields[0]
				copy.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("unrelatedField"))
				obj.Fields = append(obj.Fields, &copy)
			case "class type annotation":
				obj.ConstantPoolManager.AddUtf8Info("RuntimeVisibleTypeAnnotations")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			input := obj.Bytes()
			if variant == "idempotent" {
				var err error
				input, err = a.restoreMemberGenericScope(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Parse(input); err != nil && variant != "duplicate signature" {
				t.Fatal("negative must reach proof", err)
			}
			saved := bytes.Clone(input)
			out, err := a.restoreMemberGenericScope(old, input, work)
			if !bytes.Equal(saved, input) {
				t.Fatal("caller bytes changed")
			}
			if variant == "unreturned" || variant == "pending" || variant == "stale original" || variant == "pending shadows parent" || variant == "foreign namespace" {
				if err != nil || !bytes.Equal(out, input) {
					t.Fatal("unpublished source gained edit authority", err)
				}
				return
			}
			if variant != "control" && variant != "opaque metadata" && variant != "idempotent" && variant != "parent inherited" {
				if err == nil || out != nil {
					t.Fatal("unproved scope completion", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			parse := func(data []byte) (*ClassObject, *compiledMetadataLayout) {
				r, err := NewClassReaderWithBudget(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				l := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
				o, err := parseWithMetadataLayout(r, nil, l)
				if err != nil {
					t.Fatal(err)
				}
				return o, l
			}
			before, bl := parse(input)
			after, al := parse(out)
			for i, span := range bl.constants {
				other := al.constants[i]
				if !bytes.Equal(input[span.start:span.end], out[other.start:other.end]) {
					t.Fatal("existing constant changed")
				}
			}
			if len(before.Methods) != len(after.Methods) {
				t.Fatal("method universe changed")
			}
			for i, m := range before.Methods {
				x, y := bl.methods[m], al.methods[after.Methods[i]]
				if !bytes.Equal(input[x.start:x.end], out[y.start:y.end]) {
					t.Fatal("fresh executable or method metadata changed")
				}
			}
			if again, err := a.restoreMemberGenericScope(old, out, nil); err != nil || !bytes.Equal(again, out) {
				t.Fatal("non-idempotent completion", err)
			}
			if variant == "opaque metadata" {
				found := false
				for _, attr := range after.Attributes {
					if u, ok := attr.(*UnparsedAttribute); ok && u.Name == "OpaqueScopeData" {
						found = bytes.Equal(u.Info, []byte{5, 7, 9})
					}
				}
				if !found {
					t.Fatal("unrelated metadata removed")
				}
			}
		})
	}
}
