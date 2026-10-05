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

func TestNativeLoopTransferBindingRequiresStructuredLeaf(t *testing.T) {
	for _, scenario := range []string{"break", "continue", "invalid label with legacy name", "thrown operand with legacy name", "opaque text", "nil"} {
		t.Run(scenario, func(t *testing.T) {
			s := statements.NewSourceTransferStatement("break", "OUTER")
			want := false
			switch scenario {
			case "break":
				want = true
			case "continue":
				s.RetargetSourceTransfer("continue", "OUTER")
				want = true
			case "invalid label with legacy name":
				s.LoopTargetLabel = "x;hidden()"
				s.Name = "break"
			case "thrown operand with legacy name":
				s.ThrownValue = values.JavaNull
				s.Name = "break"
			case "opaque text":
				s = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "continue OUTER" }, nil)
			case "nil":
				s = nil
			}
			roots, children, known := nativeSourceNameChildren(s)
			if known != want || len(roots) != 0 || len(children) != 0 {
				t.Fatalf("known=%v roots=%d children=%d", known, len(roots), len(children))
			}
			if want {
				_, _, control := catchSourceChildren(s)
				if control {
					t.Fatal("name-only proof broadened control-region permission")
				}
			}
		})
	}
}
func TestNativeLoopTransferBindingPreservesIdentityAndRefusesIncompleteWalk(t *testing.T) {
	for _, scenario := range []string{"structured", "protected parameter", "opaque", "invalid label", "statement cycle", "budget", "depth", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.Id.SetName("count")
			ctx := &class_context.ClassContext{LocalNames: map[*utils.VariableId]string{}}
			c := &ClassObjectDumper{FuncCtx: ctx}
			transfer := statements.NewSourceTransferStatement("continue", "OUTER")
			loop := &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), Body: []statements.Statement{statements.NewAssignStatement(ref, values.NewJavaLiteral(7, ref.Type()), true), transfer}}
			body := []statements.Statement{loop, statements.NewReturnStatement(ref)}
			protected := map[*utils.VariableId]bool{}
			switch scenario {
			case "protected parameter":
				protected[ref.Id] = true
			case "opaque":
				loop.Body[1] = statements.NewCustomStatement(func(*class_context.ClassContext) string { return "continue OUTER" }, nil)
			case "invalid label":
				transfer.LoopTargetLabel = "OUTER;count"
				transfer.Name = "continue"
			case "statement cycle":
				loop.Body = append(loop.Body, loop)
			case "budget":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "depth":
				c.Work = workbudget.New(nil, workbudget.Limits{MaxASTDepth: 1})
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Work = workbudget.New(cancelCtx, workbudget.Limits{})
			}
			c.prepareNativeSourceNames(body, nil, map[string]bool{"count": true}, protected)
			valid := scenario == "structured" || scenario == "protected parameter"
			if c.nativeCaptureFailed == valid {
				t.Fatalf("failed=%v", c.nativeCaptureFailed)
			}
			if valid {
				want := "jdec$local$count"
				if scenario == "protected parameter" {
					want = "count"
				}
				if ref.String(ctx) != want || ref.Id.String() != "count" {
					t.Fatal("logical identity or protected binding changed")
				}
				if transfer.String(ctx) != "continue OUTER" {
					t.Fatal("label edited while renaming locals")
				}
			} else if len(ctx.LocalNames) != 0 {
				t.Fatal("partial proof committed names")
			}
		})
	}
}
