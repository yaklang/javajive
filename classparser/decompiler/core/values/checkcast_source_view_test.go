package values

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func castViewMetadata() map[string]callbinding.Class {
	return map[string]callbinding.Class{
		"java/lang/Object": {Name: "java/lang/Object", ParentsComplete: true, MembersComplete: true},
		"sample/Final":     {Name: "sample/Final", Final: true, Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true},
		"sample/Open":      {Name: "sample/Open", Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true},
		"sample/Child":     {Name: "sample/Child", Parents: []string{"sample/Open", "sample/Face"}, ParentsComplete: true, MembersComplete: true},
		"sample/Face":      {Name: "sample/Face", IsInterface: true, Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true},
		"sample/Other":     {Name: "sample/Other", Final: true, Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true},
	}
}
func TestOriginalCheckCastObjectViewRequiresDisjointProof(t *testing.T) {
	for _, tc := range []struct {
		source, target string
		want           bool
	}{
		{"Lsample/Final;", "Lsample/Face;", true}, {"Lsample/Face;", "Lsample/Final;", true},
		{"Lsample/Open;", "Lsample/Face;", false}, {"Lsample/Child;", "Lsample/Face;", false},
		{"Lsample/Child;", "Lsample/Open;", false}, {"Lsample/Open;", "Lsample/Child;", false},
		{"Lsample/Final;", "Lsample/Other;", true}, {"Lsample/Final;", "Ljava/lang/Object;", false},
		{"Ljava/lang/Object;", "Lsample/Final;", false}, {"[I", "[J", true}, {"[I", "[I", false},
		{"[Lsample/Final;", "[Lsample/Other;", true}, {"[Lsample/Child;", "[Lsample/Open;", false},
		{"[[I", "[Ljava/lang/Cloneable;", false}, {"[[I", "[Ljava/lang/Object;", false},
		{"[I", "Lsample/Face;", true}, {"Lsample/Face;", "[I", true},
		{"[I", "Ljava/lang/Cloneable;", false}, {"Ljava/io/Serializable;", "[I", false},
		{"[I", "[Lsample/Final;", true}, {"Lmissing/Unknown;", "Lsample/Face;", false},
	} {
		t.Run(tc.source+"-"+tc.target, func(t *testing.T) {
			meta := castViewMetadata()
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }}
			s, e := types.ParseDescriptor(tc.source)
			if e != nil {
				t.Fatal(e)
			}
			target, e := types.ParseDescriptor(tc.target)
			if e != nil {
				t.Fatal(e)
			}
			v := NewJavaRef(utils.NewRootVariableId(), nil, s)
			c := NewOriginalCheckCast(v, target, 17)
			if got := c.needsObjectCheckCastView(v, ctx); got != tc.want {
				t.Fatalf("view%v want%v", got, tc.want)
			}
			text := c.String(ctx)
			if strings.Contains(text, "java.lang.Object") == false && tc.want {
				t.Fatal(text)
			}
			if c.Value != v || c.OriginPC != 17 || c.TargetType != target {
				t.Fatal("mutated original cast ownership")
			}
		})
	}
}
func TestOriginalCheckCastObjectViewRejectsIncompleteOrSyntheticProof(t *testing.T) {
	for _, name := range []string{"synthetic", "binding", "negativePC", "changedTarget", "noFinal", "missingParent", "incompleteParent", "cycle", "wrongIdentity", "invalidFinalInterface", "nodesBudget", "edgesBudget", "nilContext", "shortName", "opaqueProducer", "inlineGeneric", "genericFactory", "missingDeclaration", "wrongDescriptor", "dynamic"} {
		t.Run(name, func(t *testing.T) {
			meta := castViewMetadata()
			source := types.NewJavaClass("sample.Final")
			target := types.NewJavaClass("sample.Face")
			var v JavaValue = NewJavaRef(utils.NewRootVariableId(), nil, source)
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }}
			c := NewOriginalCheckCast(v, target, 17)
			switch name {
			case "synthetic":
				c.OriginalCheckCast = false
			case "binding":
				c.Binding = true
			case "negativePC":
				c.OriginPC = -1
			case "changedTarget":
				c.TargetType = types.NewJavaClass("sample.Other")
			case "noFinal":
				x := meta["sample/Final"]
				x.Final = false
				meta[x.Name] = x
			case "missingParent":
				delete(meta, "java/lang/Object")
			case "incompleteParent":
				x := meta["java/lang/Object"]
				x.ParentsComplete = false
				meta[x.Name] = x
			case "cycle":
				x := meta["java/lang/Object"]
				x.Parents = []string{"sample/Final"}
				meta[x.Name] = x
			case "wrongIdentity":
				x := meta["sample/Final"]
				x.Name = "wrong/Final"
				meta["sample/Final"] = x
			case "invalidFinalInterface":
				x := meta["sample/Final"]
				x.IsInterface = true
				meta[x.Name] = x
			case "nodesBudget":
				x := meta["sample/Final"]
				x.Parents = []string{"node/C0"}
				meta[x.Name] = x
				for i := 0; i < 130; i++ {
					n := fmt.Sprintf("node/C%d", i)
					meta[n] = callbinding.Class{Name: n, Parents: []string{fmt.Sprintf("node/C%d", i+1)}, ParentsComplete: true}
				}
			case "edgesBudget":
				x := meta["sample/Final"]
				for i := 0; i < 1025; i++ {
					x.Parents = append(x.Parents, "java/lang/Object")
				}
				meta[x.Name] = x
			case "nilContext":
				ctx = nil
			case "shortName":
				v = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("Final"))
				c.Value = v
			case "opaqueProducer":
				v = NewCustomValue(func(*class_context.ClassContext) string { return "opaque()" }, func() types.JavaType { return source })
				c.Value = v
			default:
				desc := "()Lsample/Final;"
				ft, _ := types.ParseMethodDescriptor(desc)
				f := &FunctionCallExpression{ClassName: "sample.Final", Object: NewJavaClassValue(types.NewJavaClass("sample.Final")), FunctionName: "make", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, FuncType: ft.FunctionType(), OriginPC: 3}
				x := meta["sample/Final"]
				x.Methods = []callbinding.Method{{Name: "make", Desc: desc, Generic: name == "genericFactory" || name == "inlineGeneric", Signature: map[bool]string{true: "<T:Ljava/lang/Object;>()TT;"}[name == "genericFactory" || name == "inlineGeneric"]}}
				meta[x.Name] = x
				if name == "missingDeclaration" {
					x.Methods = nil
					meta[x.Name] = x
				}
				if name == "wrongDescriptor" {
					f.Descriptor = "()Ljava/lang/Object;"
				}
				if name == "dynamic" {
					f.Kind = InvokeDynamic
				}
				v = f
				if name == "inlineGeneric" {
					r := NewJavaRef(utils.NewRootVariableId(), nil, source)
					r.StackVar = f
					v = r
				}
				c.Value = v
			}
			if c.needsObjectCheckCastView(v, ctx) {
				t.Fatal("unproved source view accepted")
			}
		})
	}
}
func TestOriginalCheckCastFixedProducerAndArrayIdentity(t *testing.T) {
	meta := castViewMetadata()
	ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }}
	desc := "()Lsample/Final;"
	ft, _ := types.ParseMethodDescriptor(desc)
	f := &FunctionCallExpression{ClassName: "sample.Final", Object: NewJavaClassValue(types.NewJavaClass("sample.Final")), FunctionName: "make", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, FuncType: ft.FunctionType(), OriginPC: 3}
	x := meta["sample/Final"]
	x.Methods = []callbinding.Method{{Name: "make", Desc: desc}}
	meta[x.Name] = x
	c := NewOriginalCheckCast(f, types.NewJavaClass("sample.Face"), 9)
	if !c.needsObjectCheckCastView(f, ctx) || strings.Count(c.String(ctx), "make(") != 1 {
		t.Fatal("fixed producer was lost or duplicated")
	}
	cyclic := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
	a := cyclic.RawType().(*types.JavaArrayType)
	a.JavaType = cyclic
	if checkCastReferenceDescriptor(cyclic, ctx, 0) != "" {
		t.Fatal("cyclic type accepted")
	}
	malformed := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
	malformed.RawType().(*types.JavaArrayType).Dimension = 256
	if checkCastReferenceDescriptor(malformed, ctx, 0) != "" {
		t.Fatal("rank overflow accepted")
	}
}
