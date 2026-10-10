package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestCheckedEscapeTypedHandlerMonitorStillRequiresUncheckedThrowEvidence(t *testing.T) {
	for _, scenario := range []string{"certified", "unknown monitor", "edited receiver", "unchecked evidence missing", "throw origin missing", "opaque completion", "catchall", "different throw origin"} {
		t.Run(scenario, func(t *testing.T) {
			ex := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Exception"))
			thrown := &statements.CustomStatement{ThrownValue: ex, HasOriginPC: true, OriginPC: 41}
			sync := statements.NewSynchronizedStatementFromMonitor(statements.NewOriginalMonitorStatement("monitor_enter", values.JavaNull, 30, 30), []statements.Statement{thrown})
			unchecked := map[int]bool{41: true}
			handler := statements.CatchHandler{EntryPC: 20}
			switch scenario {
			case "unknown monitor":
				sync = statements.NewSynchronizedStatement(values.JavaNull, sync.Body)
			case "edited receiver":
				sync.Argument = nil
			case "unchecked evidence missing":
				unchecked = nil
			case "throw origin missing":
				thrown.HasOriginPC = false
			case "opaque completion":
				sync.Body = []statements.Statement{statements.NewCustomStatement(nil, nil)}
			case "catchall":
				handler.CatchAll = true
			case "different throw origin":
				thrown.OriginPC = 42
			}
			tr := statements.NewTryCatchStatement(nil, [][]statements.Statement{{sync}})
			tr.Exception = []*values.JavaRef{ex}
			tr.Handlers = []statements.CatchHandler{handler}
			got := typedAbsorbingHandlers([]statements.Statement{tr}, unchecked)
			if got[20] != (scenario == "certified") {
				t.Fatalf("absorbs=%v", got)
			}
		})
	}
}
