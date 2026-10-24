package class_context

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSourceProofContextSnapshotKeepsItsOriginalMethod(t *testing.T) {
	id := new(coreutils.VariableId)
	version := "original"
	live := &ClassContext{
		FunctionName: "first", CurrentMethodDesc: "(I)Ljava/lang/Object;", CurrentMethodSig: "<E:Ljava/lang/Throwable;>(I)Ljava/lang/Object;",
		Arguments: []string{"input"}, TypeParams: []string{"E"}, LocalNames: map[*coreutils.VariableId]string{id: "input"},
		CatchEntryNames: map[int]string{7: "caught"}, MethodSignatures: map[string]string{"first": "first-signature"},
		InvocationMetadata: func(name string) (callbinding.Class, bool) {
			return callbinding.Class{Name: name, Signature: version}, true
		},
		SourceAllocationRender:       func(AllocationRenderView) (string, string) { return "live", "live" },
		SourceConstructedInvocation:  func(ConstructedInvocationView) {},
		SourceMethodReference:        func(any, string) {},
		SourceGenericArgument:        func(any, int) (string, bool) { return "live", true },
		SourceGenericReceiver:        func(any) (string, bool) { return "live", true },
		SourceGenericInvocationBound: func(any) bool { return true },
		SourceGenericInvocation:      func(_ any, source string, _ bool) string { return source },
	}
	work := workbudget.New(nil, workbudget.Limits{})
	saved, known := live.SnapshotForSourceProof(work)
	if !known || saved == nil || saved == live || work.Used(workbudget.CounterNodeCopies) == 0 {
		t.Fatal("original context was not independently snapshotted")
	}
	live.FunctionName, live.CurrentMethodDesc, live.CurrentMethodSig = "second", "()V", ""
	live.Arguments[0], live.TypeParams[0], live.LocalNames[id] = "other", "Other", "other"
	live.CatchEntryNames[7], live.MethodSignatures["first"] = "other", "changed"
	if saved.FunctionName != "first" || saved.CurrentMethodDesc != "(I)Ljava/lang/Object;" || saved.CurrentMethodSig == "" || saved.Arguments[0] != "input" || saved.TypeParams[0] != "E" || saved.LocalNames[id] != "input" || saved.CatchEntryNames[7] != "caught" || saved.MethodSignatures["first"] != "first-signature" {
		t.Fatal("a sibling's live coordinates changed the earlier publisher")
	}
	if saved.SourceAllocationRender != nil || saved.SourceConstructedInvocation != nil || saved.SourceGenericArgument != nil || saved.SourceGenericReceiver != nil || saved.SourceGenericInvocationBound != nil || saved.SourceGenericInvocation != nil || live.SourceAllocationRender == nil || live.SourceConstructedInvocation == nil || live.SourceGenericArgument == nil || live.SourceGenericReceiver == nil || live.SourceGenericInvocationBound == nil || live.SourceGenericInvocation == nil {
		t.Fatal("a publication snapshot retained or changed live render authority")
	}
	if saved.SourceMethodReference != nil || live.SourceMethodReference == nil {
		t.Fatal("a proof snapshot retained or erased the live reference observer")
	}
	// Metadata queries deliberately stay live. Freezing the resolver's answers
	// would conceal the stale-input change that the retained proof must reject.
	version = "changed-original-metadata"
	if got, ok := saved.InvocationMetadata("p/Target"); !ok || got.Signature != version {
		t.Fatal("the snapshot manufactured a stale metadata certificate")
	}
}

func TestSourceProofContextSnapshotPreflightsCopiesAndCancellation(t *testing.T) {
	live := &ClassContext{Arguments: []string{"a", "b"}, TypeParams: []string{"E"}}
	for _, name := range []string{"copy limit", "storage limit", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := workbudget.Limits{}
			switch name {
			case "copy limit":
				limits.MaxNodeCopies = 1
			case "storage limit":
				limits.MaxOutputBytes = 1
			case "canceled":
				cancel()
			}
			work := workbudget.New(ctx, limits)
			if copied, known := live.SnapshotForSourceProof(work); known || copied != nil || work.Err() == nil {
				t.Fatal("an unavailable copy granted a source snapshot")
			}
			if live.Arguments[0] != "a" || live.TypeParams[0] != "E" {
				t.Fatal("failed preflight changed the live context")
			}
		})
	}
	work := workbudget.New(nil, workbudget.Limits{MaxNodeCopies: 7})
	if _, known := live.SnapshotForSourceProof(work); !known || work.Used(workbudget.CounterNodeCopies) != 4 {
		t.Fatal("first copy did not charge its containers")
	}
	if _, known := live.SnapshotForSourceProof(work); known || work.Err() == nil || work.Used(workbudget.CounterNodeCopies) < 4 {
		t.Fatal("a repeated snapshot reset or escaped the shared copy allowance")
	}
}
