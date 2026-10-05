package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"testing"
)

func TestNativeAnonymousProjectionRequiresCurrentMethodBindingPhase(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "Preparation"}
	family := &nativeAnonymousFamily{owner: "Preparation", children: map[string]*nativeAnonymousClass{"Preparation$1": {}}}
	c := &ClassObjectDumper{FuncCtx: ctx, nativeAnonymousRoot: family}
	c.wireNativeAnonymousSource()
	if ctx.SourceAnonymousCandidate == nil {
		t.Fatal("candidate protocol absent")
	}
	if ctx.SourceAnonymousCandidate("Preparation$1") || family.failed {
		t.Fatal("provisional IR rendering committed an ownership decision")
	}
	// Even a capture-free allocation has a current method binding phase. The
	// callback denies all locals: readiness never substitutes for a local proof.
	ctx.SourceCaptureStable = func(int, *coreutils.VariableId) bool { return false }
	if !ctx.SourceAnonymousCandidate("Preparation$1") || ctx.SourceAnonymousCandidate("Other$1") {
		t.Fatal("current binding phase lost original owned identity")
	}
	id := coreutils.NewRootVariableId()
	if ctx.SourceCaptureStable(7, id) {
		t.Fatal("readiness licensed an unproved local")
	}
	ctx.SourceCaptureStable = nil
	if ctx.SourceAnonymousCandidate("Preparation$1") || family.failed {
		t.Fatal("a sibling method inherited the completed source phase")
	}
}
