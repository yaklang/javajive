package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestAdversarialCatchHeadersParticipateInLexicalAlphaRenaming(t *testing.T) {
	for _, read := range []bool{false, true} {
		name := "unused"
		if read {
			name = "read"
		}
		t.Run(name, func(t *testing.T) {
			ctx := &class_context.ClassContext{}
			d := &ClassObjectDumper{FuncCtx: ctx}
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			ref.Id.SetName("var1")
			handler := &statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{nil}}
			if read {
				handler.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(ref)}
			}
			if !d.prepareNativeSourceNamesInNamespace([]statements.Statement{handler}, nil, map[string]bool{"var1": true}, nil, "jdec$lambda$1$") {
				t.Fatal("complete catch namespace")
			}
			if ref.String(ctx) == "var1" || ctx.LocalNames[ref.Id] == "" {
				t.Fatal("catch declaration kept outer name")
			}
			if read && handler.CatchBodies[0][0].String(ctx) != ref.String(ctx) {
				t.Fatal("catch read lost declaration identity")
			}
			roots, _, known := catchSourceChildren(handler)
			if !known || len(roots) != 0 {
				t.Fatal("name traversal leaked into write/motion proof")
			}
		})
	}
	for _, bad := range []string{"nil", "missing id", "parameter", "receiver"} {
		t.Run(bad, func(t *testing.T) {
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			switch bad {
			case "nil":
				ref = nil
			case "missing id":
				ref.Id = nil
			case "parameter":
				ref.IsParam = true
			case "receiver":
				ref.IsThis = true
			}
			_, _, known := nativeSourceNameChildren(&statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{nil}})
			if known {
				t.Fatal("unbound catch header")
			}
		})
	}
}
