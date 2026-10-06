package statements

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestErasedThrowableViewRequiresSameMethodFormalErasure(t *testing.T) {
	for _, tt := range []struct {
		name, signature, operand string
		want                     bool
	}{
		{"Throwable formal", "<E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;", "java.lang.Throwable", true},
		{"renamed formal", "<failure:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^Tfailure;", "java.lang.Throwable", true},
		{"unrelated formals", "<T:Ljava/lang/Object;E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;", "java.lang.Throwable", true},
		{"narrow IOException bound", "<E:Ljava/io/IOException;>(Ljava/lang/Throwable;)V^TE;", "java.lang.Throwable", false},
		{"class formal", "(Ljava/lang/Throwable;)V^TE;", "java.lang.Throwable", false},
		{"concrete throws", "(Ljava/lang/Throwable;)V^Ljava/lang/Throwable;", "java.lang.Throwable", false},
		{"multiple throws", "<E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;^Ljava/io/IOException;", "java.lang.Throwable", false},
		{"different operand erasure", "<E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;", "java.lang.Object", false},
		{"already type variable", "<E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;", "E", false},
		{"missing signature", "", "java.lang.Throwable", false},
		{"trailing unknown data", "<E:Ljava/lang/Throwable;>(Ljava/lang/Throwable;)V^TE;garbage", "java.lang.Throwable", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{CurrentMethodSig: tt.signature, TypeParams: []string{"E", "failure", "T"}}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(tt.operand))
			before := ref.Type()
			name, ok := erasedThrowableTypeVariableView(ctx, ref)
			if ok != tt.want || (ok && name == "") || ref.Type() != before {
				t.Fatalf("view=%q proved=%v expected=%v; operand view must stay unchanged", name, ok, tt.want)
			}
		})
	}
	if _, ok := erasedThrowableTypeVariableView(nil, nil); ok {
		t.Fatal("missing evidence cannot prove a throw view")
	}
}

func TestErasedCheckedThrowViewRequiresExactBoundAndOriginalThrowableHierarchy(t *testing.T) {
	metadata := map[string]callbinding.Class{
		"java/io/IOException": {Name: "java/io/IOException", Parents: []string{"java/lang/Exception"}},
		"java/lang/Exception": {Name: "java/lang/Exception", Parents: []string{"java/lang/Throwable"}},
		"proof/Failure":       {Name: "proof/Failure", Parents: []string{"java/lang/Exception"}},
		"java/lang/String":    {Name: "java/lang/String", Parents: []string{"java/lang/Object"}},
	}
	for _, row := range []struct {
		name, sig, operand string
		metadata, want     bool
	}{
		{"checked same bound", "<E:Ljava/io/IOException;>()V^TE;", "java.io.IOException", true, true},
		{"custom same bound", "<E:Lproof/Failure;>()V^TE;", "proof.Failure", true, true},
		{"method shadows class bound", "<E:Ljava/io/IOException;>()V^TE;", "java.io.IOException", true, true},
		{"Throwable cannot narrow", "<E:Ljava/io/IOException;>()V^TE;", "java.lang.Throwable", true, false},
		{"runtime exception cannot narrow", "<E:Ljava/io/IOException;>()V^TE;", "java.lang.RuntimeException", true, false},
		{"subclass is different erasure", "<E:Ljava/io/IOException;>()V^TE;", "java.io.FileNotFoundException", true, false},
		{"unknown hierarchy", "<E:Lproof/Failure;>()V^TE;", "proof.Failure", false, false},
		{"non throwable bound", "<E:Ljava/lang/String;>()V^TE;", "java.lang.String", true, false},
		{"class formal cannot borrow method", "()V^TE;", "java.io.IOException", true, false},
		{"multiple throws ambiguous", "<E:Ljava/io/IOException;>()V^TE;^Ljava/io/IOException;", "java.io.IOException", true, false},
		{"already formal view", "<E:Ljava/io/IOException;>()V^TE;", "E", true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{CurrentMethodSig: row.sig, ClassSig: "<E:Ljava/lang/Throwable;>Ljava/lang/Object;", TypeParams: []string{"E"}}
			if row.metadata {
				ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(row.operand))
			before := ref.Type()
			name, ok := erasedThrowableTypeVariableView(ctx, ref)
			if ok != row.want || ok && name != "E" || ref.Type() != before {
				t.Fatalf("view=%q,%v want=%v operand mutated=%v", name, ok, row.want, ref.Type() != before)
			}
		})
	}
}

func TestErasedCheckedThrowViewKeepsResourceAndHierarchyFailures(t *testing.T) {
	for _, variant := range []string{"budget", "memory", "canceled", "cycle", "foreign identity", "wide parents"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{CurrentMethodSig: "<E:Lproof/Failure;>()V^TE;", TypeParams: []string{"E"}}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
				return callbinding.Class{Name: n, Parents: []string{"java/lang/Throwable"}}, true
			}
			switch variant {
			case "budget":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "cycle":
				ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					return callbinding.Class{Name: n, Parents: []string{n}}, true
				}
			case "foreign identity":
				ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					return callbinding.Class{Name: "proof/Foreign", Parents: []string{"java/lang/Throwable"}}, true
				}
			case "wide parents":
				ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					p := make([]string, 65)
					for i := range p {
						p[i] = "java/lang/Throwable"
					}
					return callbinding.Class{Name: n, Parents: p}, true
				}
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Failure"))
			if _, ok := erasedThrowableTypeVariableView(ctx, ref); ok {
				t.Fatal("unproved view")
			}
			if ctx.Work != nil && ctx.Work.Err() == nil {
				t.Fatal("lost resource failure")
			}
		})
	}
}
