package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestForwardProtectedBlockRetainsEveryTransferOwner(t *testing.T) {
	for _, variant := range []string{"plain", "labeled", "nested loop", "free break", "free continue", "catch break", "unknown handler", "unknown range", "inverted range", "missing exception", "opaque", "recursive"} {
		t.Run(variant, func(t *testing.T) {
			leaf := statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
			st := statements.NewTryCatchStatement([]statements.Statement{leaf}, [][]statements.Statement{{leaf}})
			st.Exception = []*values.JavaRef{layerTestRef("java.lang.Exception")}
			st.Handlers = []statements.CatchHandler{{EntryPC: 30, ProtectedRanges: [][2]int{{10, 20}}}}
			want := variant == "plain" || variant == "labeled" || variant == "nested loop"
			switch variant {
			case "labeled":
				st.TryBody = []statements.Statement{statements.NewSourceTransferStatement("break", "original")}
			case "nested loop":
				st.TryBody = []statements.Statement{statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{statements.NewSourceTransferStatement("continue", "")})}
			case "free break":
				st.TryBody = []statements.Statement{statements.NewSourceTransferStatement("break", "")}
			case "free continue":
				st.TryBody = []statements.Statement{statements.NewSourceTransferStatement("continue", "")}
			case "catch break":
				st.CatchBodies[0] = []statements.Statement{statements.NewSourceTransferStatement("break", "")}
			case "unknown handler":
				st.Handlers = nil
			case "unknown range":
				st.Handlers[0].ProtectedRanges = nil
			case "inverted range":
				st.Handlers[0].ProtectedRanges = [][2]int{{20, 10}}
			case "missing exception":
				st.Exception[0] = nil
			case "opaque":
				st.TryBody = []statements.Statement{&statements.CustomStatement{}}
			case "recursive":
				st.TryBody = []statements.Statement{st}
			}
			budget := 4096
			if got := forwardBlockKeepsTransferBindings(st, 0, 0, &budget); got != want {
				t.Fatalf("accepted=%v want=%v", got, want)
			}
		})
	}
}
