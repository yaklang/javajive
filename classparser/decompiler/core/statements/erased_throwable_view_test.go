package statements

import (
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
