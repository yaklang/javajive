package core

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The original reference inventory is a necessary precondition for discovery.
// An irrelevant source graph must not consume a candidate traversal's storage
// allowance merely because the containing method has many instructions.
func TestPrivateArrayDiscoveryWithoutDuplicatedReferencesSkipsSourceGraph(t *testing.T) {
	for _, inventory := range []bool{false, true} {
		name := "empty inventory"
		if inventory {
			name = "ordinary reference"
		}
		t.Run(name, func(t *testing.T) {
			d := &Decompiler{opCodes: make([]*OpCode, 8192)}
			if inventory {
				ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
				d.opcodeIdToRef = map[*OpCode][][2]any{op(OP_ALOAD_0, 0): {{ref, nil}}}
			}
			// This cyclic graph would be inappropriate to inspect without any
			// duplicated-reference candidates. Discovery should leave it alone.
			d.RootNode = &Node{}
			d.RootNode.Next = []*Node{d.RootNode}
			d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1, MaxGraphScans: 2})
			d.RegisterNestedBranchArrayCalls()
			if d.Work.Err() != nil || len(d.branchArrayCalls) != 0 {
				t.Fatal("inapplicable pass used source storage or registered a call", d.Work.Err())
			}
			if d.Work.Used(workbudget.CounterIntermediateBytes) != 0 {
				t.Fatal("inapplicable pass reserved traversal storage")
			}
		})
	}
	t.Run("cancellation still checked", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		d := &Decompiler{Work: workbudget.New(ctx, workbudget.Limits{})}
		d.RegisterNestedBranchArrayCalls()
		if d.Work.Err() == nil {
			t.Fatal("empty candidate set concealed cancellation")
		}
	})
}

func TestPrivateArrayConstructorDiscoveryNilRootUsesNoStorage(t *testing.T) {
	d := &Decompiler{Work: workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1, MaxGraphScans: 1})}
	if roots := d.branchArraySourceConstructors(nil); len(roots) != 0 || d.Work.Err() != nil {
		t.Fatal("absent operand reserved constructor traversal", d.Work.Err())
	}
	if d.Work.Used(workbudget.CounterIntermediateBytes) != 0 || d.Work.Used(workbudget.CounterGraphScans) != 0 {
		t.Fatal("absent operand consumed discovery work")
	}
}
