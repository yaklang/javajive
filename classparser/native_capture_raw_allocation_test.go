package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCapturedRawAllocationRequiresOriginalNewAndConstructor(t *testing.T) {
	files := nativeCompileClasses(t, `interface RawProofWide<T>{} class RawProofNarrow<T> implements RawProofWide<T>{} class RawProofOwner {RawProofWide<String> make(){return new RawProofNarrow<String>();}}`)
	for _, variant := range []string{"original", "nil caller", "nil declaration", "nonallocation", "solved type", "missing NEW origin", "wrong NEW PC", "missing constructor", "wrong invoke PC", "missing invoke origin", "wrong owner", "wrong descriptor", "wrong name", "static invocation", "wrong invoke kind", "wrong receiver", "missing receiver", "changed original signature", "foreign metadata identity", "changed interface flag", "changed visibility flag", "changed parent", "wrong method", "duplicate method", "missing code", "duplicate code", "unknown allocation", "foreign allocation", "incomplete metadata", "unknown target", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["RawProofOwner.class"])
			d := z.nativeMemberReader(root)
			d.FuncCtx = &class_context.ClassContext{ClassName: root.GetClassName(), FunctionName: "make", CurrentMethodDesc: "()LRawProofWide;"}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			provider := d.FuncCtx.InvocationMetadata
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range root.Methods {
				if name, _ := sourceBridgeUTF8(root, m.NameIndex); name == "make" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			bindings, known := d.nativeMemberAllocationInvocations(method, code)
			if !known || len(bindings) != 1 {
				t.Fatal("original typed allocation")
			}
			var pc int
			var binding nativeMemberAllocationInvocation
			for p, b := range bindings {
				pc, binding = p, b
			}
			seed := &values.NewExpression{JavaType: types.NewJavaClass("RawProofNarrow"), OriginPC: pc, HasOriginPC: true}
			call := &values.FunctionCallExpression{ClassName: binding.owner, FunctionName: "<init>", Descriptor: binding.descriptor, Kind: values.InvokeSpecial, Object: seed, OriginPC: binding.pc, HasOriginPC: true}
			seed.ConstructorCall = call
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, seed.Type().Copy())
			declaration := statements.NewAssignStatement(ref, seed, true)
			actual, expected := "LRawProofNarrow;", "LRawProofWide;"
			switch variant {
			case "nil caller":
				d.obj = nil
			case "nil declaration":
				declaration = nil
			case "nonallocation":
				declaration.JavaValue = ref
			case "solved type":
				seed.JavaType = types.NewParameterizedType("RawProofNarrow", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "missing NEW origin":
				seed.HasOriginPC = false
			case "wrong NEW PC":
				seed.OriginPC++
			case "missing constructor":
				seed.ConstructorCall = nil
			case "wrong invoke PC":
				call.OriginPC++
			case "missing invoke origin":
				call.HasOriginPC = false
			case "wrong owner":
				call.ClassName = "Foreign"
			case "wrong descriptor":
				call.Descriptor = "(I)V"
			case "wrong name":
				call.FunctionName = "make"
			case "static invocation":
				call.IsStatic = true
			case "wrong invoke kind":
				call.Kind = values.InvokeVirtual
			case "wrong receiver":
				call.Object = ref
			case "missing receiver":
				call.Object = nil
			case "changed original signature", "foreign metadata identity", "changed interface flag", "changed visibility flag", "changed parent":
				original := provider
				provider = func(n string) (callbinding.Class, bool) {
					m, ok := original(n)
					if n == "RawProofNarrow" {
						switch variant {
						case "changed original signature":
							m.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;LRawProofWide<Ljava/lang/String;>;"
						case "foreign metadata identity":
							m.Name = "Foreign"
						case "changed interface flag":
							m.IsInterface = true
						case "changed visibility flag":
							m.Public = !m.Public
						case "changed parent":
							m.Parents = []string{"java/lang/Object"}
						}
					}
					return m, ok
				}
			case "wrong method":
				d.FuncCtx.CurrentMethodDesc = "()V"
			case "duplicate method":
				root.Methods = append(root.Methods, method)
			case "missing code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "unknown allocation":
				actual = "LMissing;"
			case "foreign allocation":
				actual = "LRawProofOwner;"
			case "incomplete metadata":
				original := provider
				provider = func(n string) (callbinding.Class, bool) {
					m, ok := original(n)
					if n == "RawProofNarrow" {
						m.ParentsComplete = false
					}
					return m, ok
				}
			case "unknown target":
				expected = "LMissing;"
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(c, workbudget.Limits{})
			}
			got, ok := d.nativeCapturedRawAllocationType(declaration, actual, expected, provider)
			if ok != (variant == "original") {
				t.Fatalf("allocation proof=%v", ok)
			}
			if ok {
				if name, known := types.RawClassFQN(got); !known || name != "RawProofWide" {
					t.Fatal("original raw supertype", got)
				}
			}
		})
	}
}
