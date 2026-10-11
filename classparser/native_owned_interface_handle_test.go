package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Every case starts from the actual compiled interface-handle archive. Cached
// source ownership cannot replace its tag, declaration flags or generic erasure.
func TestNativeOwnedInterfaceHandleRequiresOriginalPhysicalDeclaration(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"InterfaceHandleOwner.java": interfaceHandleProtocolFixture}, "none", "8")
	variants := []string{"original", "no source member", "nil source member", "foreign source object", "foreign owner", "foreign name", "foreign flags", "cached static", "captured field", "cached formals", "wrong CP tag", "class invocation", "special invocation", "static invocation", "missing method", "duplicate method", "private method", "static method", "synthetic method", "bridge method", "class header", "unknown descriptor", "wrong erasure", "primitive argument", "free formal", "method formal", "duplicate signature", "nil signature", "opaque signature", "class formal", "class signature superclass", "class signature interface count", "class signature method", "class signature primitive argument", "nil class signature", "duplicate class signature", "duplicate self", "abstract code", "throws mismatch", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(append([]byte(nil), files["InterfaceHandleOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original interface member plan")
			}
			member := p.children["InterfaceHandleOwner$Callback"]
			if member == nil {
				t.Fatal("original member")
			}
			obj := member.object
			index := *z.originalMemberIndex()
			targets := append([]nativeMemberHandleTarget(nil), index.handleTargets[obj.GetClassName()]...)
			if len(targets) != 1 || targets[0].kind != 9 || targets[0].methodRef {
				t.Fatal("original interface physical target", targets)
			}
			target := &targets[0]
			var method *MemberInfo
			var sig *SignatureAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if n == target.name && desc == target.descriptor {
					method = m
					for _, a := range m.Attributes {
						if s, ok := a.(*SignatureAttribute); ok {
							sig = s
						}
					}
				}
			}
			if method == nil || sig == nil {
				t.Fatal("original direct generic declaration")
			}
			sources := []*nativeMemberClass{member}
			var work *workbudget.Budget
			switch variant {
			case "no source member":
				sources = nil
			case "nil source member":
				sources[0] = nil
			case "foreign source object":
				copy := *obj
				member.object = &copy
			case "foreign owner":
				member.owner = "Foreign"
			case "foreign name":
				member.name = "Foreign"
			case "foreign flags":
				member.flags ^= 1
			case "cached static":
				member.static = false
			case "captured field":
				member.field = "this$0"
			case "cached formals":
				member.formalCount = 1
			case "wrong CP tag":
				target.methodRef = true
			case "class invocation":
				target.kind = 5
			case "special invocation":
				target.kind = 7
			case "static invocation":
				target.kind = 6
			case "missing method":
				var out []*MemberInfo
				for _, m := range obj.Methods {
					if m != method {
						out = append(out, m)
					}
				}
				obj.Methods = out
			case "duplicate method":
				obj.Methods = append(obj.Methods, method)
			case "private method":
				method.AccessFlags = 2
			case "static method":
				method.AccessFlags = 9
			case "synthetic method":
				method.AccessFlags |= 0x1000
			case "bridge method":
				method.AccessFlags |= 0x40
			case "class header":
				obj.AccessFlags = 0x21
			case "unknown descriptor":
				target.descriptor = "(Ljava/util/List;)V"
			case "wrong erasure":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "(Ljava/util/List<Ljava/lang/String;>;)V")
			case "primitive argument":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "(Ljava/util/Collection<I>;)V")
			case "free formal":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "(Ljava/util/Collection<TMissing;>;)V")
			case "method formal":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "<T:Ljava/lang/Object;>(Ljava/util/Collection<TT;>;)V")
			case "duplicate signature":
				method.Attributes = append(method.Attributes, sig)
			case "nil signature":
				method.Attributes = append(method.Attributes, (*SignatureAttribute)(nil))
			case "opaque signature":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "opaque")
			case "class formal":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "<T:Ljava/lang/Object;>Ljava/lang/Object;")})
			case "class signature superclass":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "Ljava/lang/String;")})
			case "class signature interface count":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "Ljava/lang/Object;Ljava/lang/Runnable;")})
			case "class signature method":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "()V")})
			case "class signature primitive argument":
				obj.Attributes = append(obj.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "Ljava/lang/Object;Ljava/util/Collection<I>;")})
			case "nil class signature":
				obj.Attributes = append(obj.Attributes, (*SignatureAttribute)(nil))
			case "duplicate class signature":
				a := &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, obj, "Ljava/lang/Object;")}
				obj.Attributes = append(obj.Attributes, a, a)
			case "duplicate self":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == obj.GetClassName() {
								table.Classes = append(table.Classes, row)
								break
							}
						}
					}
				}
			case "abstract code":
				method.Attributes = append(method.Attributes, &CodeAttribute{})
			case "throws mismatch":
				sig.SignatureIndex = sourceBridgePoolString(t, obj, "(Ljava/util/Collection<Ljava/lang/String;>;)V^Ljava/io/IOException;")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			index.handleTargets = map[string][]nativeMemberHandleTarget{obj.GetClassName(): targets}
			if got := nativeMemberOrdinaryHandlesClosed(obj, &index, work, sources...); got != (variant == "original") {
				t.Fatalf("interface physical binding=%v", got)
			}
		})
	}
}
