package statements

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"testing"
)

func TestSourceTransferSealsOperandFreeRendering(t *testing.T) {
	for _, kind := range []string{"break", "continue"} {
		for _, label := range []string{"", "LOOP_17", "scope$nested"} {
			s := NewSourceTransferStatement(kind, label)
			if s == nil || !s.HasSourceTransfer() || !s.SourceTransferOnly() {
				t.Fatal("valid leaf rejected")
			}
			if s.Name != "" || s.LoopTransferKind != "" {
				t.Fatal("source leaf manufactured a CFG target certificate")
			}
			s.StringFunc = func(*class_context.ClassContext) string { t.Fatal("opaque closure used"); return "hiddenLocal" }
			s.replaceVar = func(*utils.VariableId, *utils.VariableId) { t.Fatal("opaque binding callback used") }
			want := kind
			if label != "" {
				want += " " + label
			}
			if s.String(nil) != want {
				t.Fatal(s.String(nil))
			}
			s.ReplaceVar(utils.NewRootVariableId(), utils.NewRootVariableId())
			if !s.RetargetSourceTransfer("continue", "OUTER") || s.String(nil) != "continue OUTER" {
				t.Fatal("structured retarget lost")
			}
			if s.Name != "" || s.LoopTransferKind != "" {
				t.Fatal("retarget manufactured control proof")
			}
		}
	}
}
func TestSourceTransferRefusesInvalidOrOpaqueCertificates(t *testing.T) {
	for _, pair := range [][2]string{{"return", ""}, {"throw", ""}, {"break", "two words"}, {"continue", "x;hidden()"}, {"break", "1name"}, {"continue", "class"}, {"break", "x.y"}} {
		if NewSourceTransferStatement(pair[0], pair[1]) != nil {
			t.Fatalf("invalid leaf admitted %q", pair)
		}
	}
	opaque := NewCustomStatement(func(*class_context.ClassContext) string { return "break" }, nil)
	opaque.Name = "break"
	opaque.LoopTransferKind = "break"
	if opaque.HasSourceTransfer() || opaque.SourceTransferOnly() || opaque.RetargetSourceTransfer("continue", "OUTER") {
		t.Fatal("opaque text gained structural certificate")
	}
	s := NewSourceTransferStatement("break", "")
	if s.RetargetSourceTransfer("throw", "") || s.String(nil) != "break" {
		t.Fatal("failed retarget mutated source")
	}
	s.LoopTargetLabel = "x;hidden()"
	if !s.HasSourceTransfer() || s.SourceTransferOnly() {
		t.Fatal("mutated leaf escaped validation")
	}
	s.LoopTargetLabel = ""
	s.ThrownValue = values.JavaNull
	if s.SourceTransferOnly() {
		t.Fatal("value operand erased")
	}
	var missing *CustomStatement
	if missing.HasSourceTransfer() || missing.SourceTransferOnly() || missing.RetargetSourceTransfer("break", "") {
		t.Fatal("nil admitted")
	}
}
