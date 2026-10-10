package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestCaughtExceptionEntrySealsOperandFreeRendererAndBinding(t *testing.T) {
	v := NewCaughtExceptionValue(17, types.NewJavaClass("java.lang.RuntimeException"))
	v.StringFunc = func(*class_context.ClassContext) string { t.Fatal("opaque text executed"); return "hiddenArray()" }
	v.WriteFunc = func(*class_context.ClassContext, *workbudget.Writer) error {
		t.Fatal("opaque writer executed")
		return nil
	}
	v.ReplaceFunc = func(*utils.VariableId, *utils.VariableId) { t.Fatal("opaque binding executed") }
	ctx := &class_context.ClassContext{CatchEntryNames: map[int]string{17: "actualException", 18: "otherException"}}
	if pc, known := v.SourceCaughtExceptionEntry(); !known || pc != 17 {
		t.Fatal("original handler identity lost")
	}
	if operands, known := Children(v); !known || len(operands) != 0 {
		t.Fatal("caught seed invented Java operands")
	}
	if effect, _ := InspectValue(v); effect&EffectOpaque == 0 {
		t.Fatal("operand completeness licensed moving the handler-entry value")
	}
	if v.String(ctx) != "actualException" || v.String(nil) != "Exception" {
		t.Fatal("handler source identifier lost")
	}
	v.ReplaceVar(utils.NewRootVariableId(), utils.NewRootVariableId())
	ctx.CatchEntryNames[17] = "renamedException"
	if v.String(ctx) != "renamedException" {
		t.Fatal("source renderer retained a stale handler name")
	}
	copy := v.WithType(func() types.JavaType { return types.NewJavaClass("java.lang.Throwable") })
	if copy.String(ctx) != "renamedException" {
		t.Fatal("type view erased structured handler binding")
	}
	if _, known := copy.SourceCaughtExceptionEntry(); !known {
		t.Fatal("type view lost dependency certificate")
	}
}

func TestCaughtExceptionEntryRefusesOpaqueAndChangedOriginAnnotations(t *testing.T) {
	for _, change := range []string{"opaque flag", "missing PC", "different PC", "negative PC", "out of range", "missing type", "changed kind"} {
		t.Run(change, func(t *testing.T) {
			v := NewCaughtExceptionValue(17, types.NewJavaClass("java.lang.RuntimeException"))
			switch change {
			case "opaque flag":
				v = NewCustomValue(func(*class_context.ClassContext) string { return "hiddenArray()" }, func() types.JavaType { return types.NewJavaClass("java.lang.RuntimeException") })
				v.Flag = "exception"
				v.OriginPC = 17
				v.HasOriginPC = true
			case "missing PC":
				v.HasOriginPC = false
			case "different PC":
				v.OriginPC = 18
			case "negative PC":
				v = NewCaughtExceptionValue(-1, v.Type())
			case "out of range":
				v = NewCaughtExceptionValue(65536, v.Type())
			case "missing type":
				v = NewCaughtExceptionValue(17, nil)
			case "changed kind":
				v.Flag = "unproved"
			}
			if _, known := v.SourceCaughtExceptionEntry(); known {
				t.Fatal("unproved handler seed accepted")
			}
			if _, known := Children(v); known {
				t.Fatal("unknown renderer dependencies discarded")
			}
		})
	}
}

func TestCaughtExceptionIdentifierHonorsOutputAndASTBudgets(t *testing.T) {
	for _, name := range []string{"a_valid_handler", strings.Repeat("x", 4096), "x;hiddenArray()"} {
		work := workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 32, MaxASTDepth: 4})
		ctx := &class_context.ClassContext{Work: work, CatchEntryNames: map[int]string{17: name}}
		got := NewCaughtExceptionValue(17, types.NewJavaClass("java.lang.RuntimeException")).String(ctx)
		if name == "a_valid_handler" {
			if got != name || work.Err() != nil {
				t.Fatal("bounded handler identifier rejected")
			}
		} else if got != "" || work.Err() == nil {
			t.Fatal("invalid/oversized source identifier escaped limits")
		}
	}
}
