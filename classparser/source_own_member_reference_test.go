package javaclassparser

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialOwnMemberReferenceCompletionPreservesFreshCode(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	old, raw, artifact := constructorSuffixFamilyRoundTrip(t, ownMemberReferenceFixture, "none", []string{"SuffixOwner", "SuffixOwner$Cell"}, "30:own:member:owner:binding:overflow\n")
	base := artifact.source.jar.sourceConstructorLowering
	prepared, err := restoreReconstructionSyntheticFlags(old, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"control", "idempotent", "opaque attribute", "unreturned member", "pending member", "wrong own argument", "raw member", "wrong method bound", "fresh type annotation", "changed method flag", "new synthetic marker", "canceled", "budget"} {
		t.Run(variant, func(t *testing.T) {
			active := map[string]constructorNativeSourceOrigin{}
			for k, v := range base.genericActive {
				active[k] = v
			}
			a := &sourceConstructorLoweringAuthority{jar: base.jar, referenceActive: base.referenceActive, genericActive: active}
			obj := mustParseLowering(t, prepared)
			var method *MemberInfo
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "identity" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("missing identity")
			}
			var work *workbudget.Budget
			switch variant {
			case "unreturned member":
				delete(active, "SuffixOwner$Cell")
			case "pending member":
				a.genericPending = active
				a.genericActive = nil
			case "wrong own argument", "raw member", "wrong method bound":
				for _, attr := range method.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						s, _ := sourceBridgeUTF8(obj, sig.SignatureIndex)
						switch variant {
						case "wrong own argument":
							s = strings.ReplaceAll(s, "$Cell<TX;>", "$Cell<Ljava/lang/CharSequence;>")
						case "raw member":
							s = strings.ReplaceAll(s, "$Cell<TX;>", "$Cell")
						case "wrong method bound":
							s = strings.ReplaceAll(s, "X::Ljava/lang/CharSequence;", "X:Ljava/lang/String;")
						}
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(s))
					}
				}
			case "opaque attribute", "fresh type annotation":
				name, data := "OpaqueOwnBinding", []byte{1, 3, 5}
				if variant == "fresh type annotation" {
					name, data = "RuntimeVisibleTypeAnnotations", []byte{0, 0}
				}
				obj.ConstantPoolManager.AddUtf8Info(name)
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: name, Length: uint32(len(data)), Info: data})
			case "changed method flag":
				method.AccessFlags |= StaticFlag
			case "new synthetic marker":
				method.AccessFlags |= syntheticMethodFlag
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			input := obj.Bytes()
			if variant == "idempotent" {
				input, err = a.restoreMemberReferenceSignatures(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(input)
			out, e := a.restoreMemberReferenceSignatures(old, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("input modified")
			}
			if variant == "budget" || variant == "canceled" {
				if e == nil || out != nil {
					t.Fatal("budget ignored")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			positive := variant == "control" || variant == "idempotent" || variant == "opaque attribute"
			if !positive {
				// Other independently proved members may complete, but the
				// changed identity method must retain its exact fresh metadata.
				after := mustParseLowering(t, out)
				for _, m := range after.Methods {
					name, _ := sourceBridgeUTF8(after, m.NameIndex)
					if name == "identity" {
						a, ap, av := sourceMethodSignatureKey(obj, method)
						b, bp, bv := sourceMethodSignatureKey(after, m)
						if !av || !bv || ap != bp || a != b {
							t.Fatal("unproved identity signature changed")
						}
					}
				}
				return
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			original := mustParseLowering(t, old)
			var originalIdentity string
			for _, m := range original.Methods {
				name, _ := sourceBridgeUTF8(original, m.NameIndex)
				if name == "identity" {
					originalIdentity, _, _ = sourceMethodSignatureKey(original, m)
				}
			}
			for _, m := range after.Methods {
				name, _ := sourceBridgeUTF8(after, m.NameIndex)
				if name == "identity" {
					signature, present, valid := sourceMethodSignatureKey(after, m)
					if !valid || !present || signature != originalIdentity {
						t.Fatal("original owner/member binding not restored")
					}
				}
			}
			if len(before.Methods) != len(after.Methods) || !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) {
				t.Fatal("fresh method or constant universe changed")
			}
			for i, m := range before.Methods {
				n := after.Methods[i]
				if m.NameIndex != n.NameIndex || m.DescriptorIndex != n.DescriptorIndex || m.AccessFlags != n.AccessFlags || len(m.Attributes) != len(n.Attributes) {
					t.Fatal("fresh declaration changed")
				}
				for j, attr := range m.Attributes {
					if _, signature := attr.(*SignatureAttribute); !signature && !reflect.DeepEqual(attr, n.Attributes[j]) {
						t.Fatal("fresh Code or opaque data changed")
					}
				}
			}
			if again, e := a.restoreMemberReferenceSignatures(old, out, nil); e != nil || !bytes.Equal(again, out) {
				t.Fatal("not idempotent", e)
			}
		})
	}
}

func TestAdversarialOwnMemberReferenceRequiresReturnedClosedDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, ownMemberReferenceFixture)
	for _, variant := range []string{"control", "dependency ancestor", "unreturned", "missing member", "missing owner", "wrong identity", "missing capture", "static member", "duplicate signature", "unbound signature", "preview", "native source", "native family", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			member := mustParseLowering(t, files["SuffixOwner$Cell.class"])
			switch variant {
			case "wrong identity":
				member.ThisClass = uint16(member.ConstantPoolManager.AddNewClassInfo("OtherCell"))
			case "missing capture":
				member.Methods = nil
			case "static member":
				for _, attr := range member.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							if row.InnerClassInfoIndex == member.ThisClass {
								row.InnerClassAccessFlags |= StaticFlag
							}
						}
					}
				}
			case "duplicate signature":
				member.Attributes = append(member.Attributes, &SignatureAttribute{SignatureIndex: uint16(member.ConstantPoolManager.AddUtf8Info("<B:Ljava/lang/Object;>Ljava/lang/Object;"))})
			case "unbound signature":
				for _, attr := range member.Attributes {
					if signature, ok := attr.(*SignatureAttribute); ok {
						signature.SignatureIndex = uint16(member.ConstantPoolManager.AddUtf8Info("<B:TUnknown;>Ljava/lang/Object;"))
					}
				}
			case "preview":
				member.MinorVersion = 65535
			}
			// The querying class is independent. If it were the owner itself,
			// its original bytes would already supply the supposedly absent owner.
			d := NewClassObjectDumper(mustParseLowering(t, files["SuffixDriver.class"]))
			d.FuncCtx = &class_context.ClassContext{}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "SuffixOwner$Cell" {
					return member.Bytes(), variant != "missing member"
				}
				if name == "SuffixOwner" && (variant == "missing owner" || variant == "dependency ancestor") {
					return nil, false
				}
				raw, known := files[name+".class"]
				return raw, known
			}
			if variant == "dependency ancestor" {
				d.declarationResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			}
			if variant == "native source" {
				d.FuncCtx.DeclarationSourceName = func(string) (string, bool) { return "SuffixOwner.Cell", true }
			}
			if variant == "native family" {
				d.nativeMemberLookup = func(string) *nativeMemberClass { return &nativeMemberClass{} }
			}
			if variant == "budget" {
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			projection := d.buildSiblingReferenceSourceFormals(func(string, []byte) bool { return variant != "unreturned" })
			names, known := projection("SuffixOwner$Cell")
			want := variant == "control" || variant == "dependency ancestor"
			if known != want || known && !reflect.DeepEqual(names, []string{"B"}) {
				t.Fatalf("source declaration %v,%v want=%v", names, known, want)
			}
			// This metadata capability must not broaden expression inference.
			if _, leaked := d.buildSiblingSourceClassFormals()("SuffixOwner$Cell"); leaked {
				t.Fatal("own declaration leaked into enclosing-formal inference")
			}
		})
	}
}

func TestAdversarialOwnMemberReferenceProjectionKeepsOrderedTypeGraph(t *testing.T) {
	files := nativeCompileClasses(t, `class Owner<A extends Number>{class Cell<B extends CharSequence,C extends B>{}}`)
	for _, variant := range []string{"direct", "array", "nested", "upper wildcard", "lower wildcard", "same text", "swapped own arguments", "raw member", "wrong variance", "wrong array rank", "changed method binder", "changed throws", "unknown binder", "missing owner", "missing member", "unknown source", "wrong source formals", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d := NewClassObjectDumper(mustParseLowering(t, files["Owner.class"]))
			d.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			load := d.nativeAnnotationDeclarationResolver()
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "missing owner" && name == "Owner" || variant == "missing member" && name == "Owner$Cell" {
					return nil, false
				}
				return load(name)
			}
			projected := d.buildSiblingReferenceSourceFormals(func(string, []byte) bool { return true })
			if variant == "unknown source" {
				projected = func(string) ([]string, bool) { return nil, false }
			} else if variant == "wrong source formals" {
				projected = func(string) ([]string, bool) { return []string{"C", "B"}, true }
			}
			oldType, newType := "LOwner<TA;>.Cell<TX;TY;>;", "LOwner$Cell<TX;TY;>;"
			prefix := "<X:Ljava/lang/CharSequence;Y:TX;>"
			want := false
			switch variant {
			case "direct":
				want = true
			case "array":
				oldType, newType, want = "["+oldType, "["+newType, true
			case "nested", "upper wildcard", "lower wildcard", "wrong variance":
				variance := ""
				if variant == "upper wildcard" || variant == "wrong variance" {
					variance = "+"
				}
				if variant == "lower wildcard" {
					variance = "-"
				}
				oldType, newType, want = "Ljava/util/List<"+variance+oldType+">;", "Ljava/util/List<"+variance+newType+">;", true
				if variant == "wrong variance" {
					newType, want = strings.Replace(newType, "+", "-", 1), false
				}
			case "same text":
				newType = oldType
			case "swapped own arguments":
				newType = "LOwner$Cell<TY;TX;>;"
			case "raw member":
				newType = "LOwner$Cell;"
			case "wrong array rank":
				oldType, newType = "["+oldType, "[["+newType
			case "unknown binder":
				newType = strings.Replace(newType, "TY;", "TMissing;", 1)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			old, fresh := prefix+"("+oldType+")"+oldType, prefix+"("+newType+")"+newType
			if variant == "changed method binder" {
				fresh = strings.Replace(fresh, "Y:TX;", "Y:Ljava/lang/String;", 1)
			}
			if variant == "changed throws" {
				old += "^Ljava/io/IOException;"
				fresh += "^Ljava/lang/Exception;"
			}
			scopes := []types.LexicalTypeScope{{Signature: "<A:Ljava/lang/Number;>Ljava/lang/Object;"}}
			descriptor, _, valid := types.EraseLexicalScopedMethodSignatureWithThrows(scopes, old)
			if !valid {
				t.Fatal("invalid original fixture")
			}
			if got := memberReferenceSignatureProjection(old, fresh, descriptor, scopes, projected, resolve, d.Work); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
		})
	}
}
