package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeBindingMonitorExitNeedsItsSealedNoSourceOperand(t *testing.T) {
	for _, kind := range []string{"owned exit", "forged exit", "changed flag", "hidden data", "enter", "nil middle"} {
		t.Run(kind, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.Id.SetName("reserved")
			var leaf statements.Statement = statements.NewOriginalMonitorStatement("monitor_exit", ref, 14, 2)
			switch kind {
			case "forged exit":
				leaf = statements.NewMiddleStatement("monitor_exit", nil)
			case "changed flag":
				leaf.(*statements.MiddleStatement).Flag = "return reserved"
			case "hidden data":
				leaf.(*statements.MiddleStatement).Data = ref
			case "enter":
				leaf = statements.NewOriginalMonitorStatement("monitor_enter", ref, 2, 2)
			case "nil middle":
				leaf = (*statements.MiddleStatement)(nil)
			}
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			c := &ClassObjectDumper{FuncCtx: ctx}
			c.prepareNativeSourceNames([]statements.Statement{leaf, statements.NewReturnStatement(ref)}, nil, map[string]bool{"reserved": true}, nil)
			valid := kind == "owned exit"
			if c.nativeCaptureFailed == valid {
				t.Fatalf("failure=%v", c.nativeCaptureFailed)
			}
			if valid {
				if ref.String(ctx) != "jdec$local$reserved" || ref.Id.String() != "reserved" {
					t.Fatal("lost identity")
				}
				if _, _, known := catchSourceChildren(leaf); known {
					t.Fatal("namespace proof granted control/effect permission")
				}
			}
			if !valid && len(ctx.LocalNames) != 0 {
				t.Fatal("partial names committed")
			}
		})
	}
}
