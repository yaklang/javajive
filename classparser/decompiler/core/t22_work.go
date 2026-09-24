package core

import (
	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

func (d *Decompiler) getenv(key string) string {
	if d != nil && d.Env != nil {
		return d.Env(key)
	}
	if d != nil && d.FunctionContext != nil {
		return d.FunctionContext.Getenv(key)
	}
	return jdecenv.Get(key)
}

// BuildSemanticCFG is the package-level alias used by T22 tests.
// Production T24/T32 timing uses (*Decompiler).BuildSemanticCFG.
func BuildSemanticCFG(d *Decompiler) (*SemanticCFG, error) {
	if d == nil {
		return nil, nil
	}
	return d.BuildSemanticCFG()
}

func (d *Decompiler) TestBytecodes() []byte {
	if d == nil {
		return nil
	}
	return d.bytecodes
}

func (d *Decompiler) OpcodeByPC(pc uint16) *OpCode {
	if d == nil {
		return nil
	}
	idx, ok := d.offsetToOpcodeIndex[pc]
	if !ok || idx < 0 || idx >= len(d.opCodes) {
		return nil
	}
	return d.opCodes[idx]
}

func (d *Decompiler) chargeNodeCopies(n int) error {
	if d == nil || d.Work == nil || n <= 0 {
		return nil
	}
	return d.Work.Charge(workbudget.CounterNodeCopies, int64(n))
}

func (d *Decompiler) TestInlineJSR() error {
	return d.inlineJSRSubroutines()
}
