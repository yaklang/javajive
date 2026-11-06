package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func ssaSwitchFixture(t *testing.T) []byte {
	t.Helper()
	const source = `
public class SsaSwitch {
    static String f(int x) {
        return switch (x) {
            case 0 -> "zero";
            case 1 -> "one";
            default -> "many";
        };
    }
}
`
	return compileJavaClassRelease(t, "SsaSwitch", source, "21")
}

func TestSSAStackJoinEvidenceRunsInProduction(t *testing.T) {
	raw := ssaSwitchFixture(t)
	previous := core.StackPhiEvidenceProvider
	calls := 0
	core.StackPhiEvidenceProvider = func(d *core.Decompiler, merge *core.OpCode) (core.StackPhiEvidence, bool) {
		calls++
		if previous != nil {
			return previous(d, merge)
		}
		return core.StackPhiEvidence{}, false
	}
	defer func() { core.StackPhiEvidenceProvider = previous }()

	result, err := DecompileWithOptions(raw, DecompileOptions{TargetSourceVersion: 21})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("production decompilation never queried SSA stack-join evidence")
	}
	if result.Status != "complete" && result.Status != "partial" {
		t.Fatalf("unexpected status %s", result.Status)
	}
	if strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatalf("SSA evidence path stubbed a valid switch expression:\n%s", result.Source)
	}
}

func TestSSAStackJoinEvidenceContradictionFailsClosed(t *testing.T) {
	raw := ssaSwitchFixture(t)
	previous := core.StackPhiEvidenceProvider
	core.StackPhiEvidenceProvider = func(*core.Decompiler, *core.OpCode) (core.StackPhiEvidence, bool) {
		return core.StackPhiEvidence{Kind: core.StackPhiPrimitive}, true
	}
	defer func() { core.StackPhiEvidenceProvider = previous }()

	result, err := DecompileWithOptions(raw, DecompileOptions{TargetSourceVersion: 21})
	if err == nil {
		// A method-level stub is acceptable only when it carries the SSA join
		// diagnostic; otherwise a contradiction was silently ignored.
		if !strings.Contains(diagnosticText(result), "ssa stack join type mismatch") {
			t.Fatalf("SSA contradiction was ignored: status=%s source=%s", result.Status, result.Source)
		}
		return
	}
	if !strings.Contains(err.Error(), "ssa stack join type mismatch") && !strings.Contains(diagnosticText(result), "ssa stack join type mismatch") {
		t.Fatalf("unexpected failure for SSA contradiction: %v", err)
	}
}

func diagnosticText(result DecompileResult) string {
	var b strings.Builder
	for _, diagnostic := range result.Diagnostics {
		b.WriteString(diagnostic.Code)
		b.WriteByte(':')
		b.WriteString(diagnostic.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
