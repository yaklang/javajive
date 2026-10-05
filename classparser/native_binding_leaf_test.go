package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeBindingAnchorKeepsLocalIdentityAndRejectsOpaqueCallbacks(t *testing.T) {
	for _, kind := range []string{"anchor", "nil anchor", "opaque empty", "hidden local", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.Id.SetName("reserved")
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			c := &ClassObjectDumper{FuncCtx: ctx}
			var leaf statements.Statement = statements.NewSourceAnchorStatement()
			switch kind {
			case "nil anchor":
				leaf = (*statements.SourceAnchorStatement)(nil)
			case "opaque empty":
				leaf = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "" }, nil)
			case "hidden local":
				leaf = statements.NewCustomStatement(func(ctx *class_context.ClassContext) string { return ref.String(ctx) }, nil)
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			c.prepareNativeSourceNames([]statements.Statement{leaf, statements.NewReturnStatement(ref)}, nil, map[string]bool{"reserved": true}, nil)
			valid := kind == "anchor"
			if c.nativeCaptureFailed == valid {
				t.Fatalf("failure=%v", c.nativeCaptureFailed)
			}
			if valid {
				if ref.String(ctx) != "jdec$local$reserved" || ref.Id.String() != "reserved" {
					t.Fatal("source binding changed logical identity")
				}
				if _, _, control := catchSourceChildren(leaf); control {
					t.Fatal("source anchor granted instruction effect/region permission")
				}
			}
			if !valid && len(ctx.LocalNames) != 0 {
				t.Fatal("partial proof committed names")
			}
		})
	}
}
