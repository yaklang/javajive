package core

import "fmt"

type ShadowObservation struct {
	Method  string `json:"method"`
	Status  string `json:"status"`
	Hash    string `json:"hash,omitempty"`
	Version uint64 `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ShadowIRRequest is the read-only snapshot input for MethodIR construction.
// It is filled after buildSemanticCFG succeeds and must not alias mutable printer state.
// CodeLimits carries authoritative Code-attribute bounds into analysis. The
// Present bit distinguishes a declared zero limit from an unspecified test IR.
type CodeLimits struct {
	Present             bool
	MaxLocals, MaxStack int
	DirectSuperClass    string
}

type ShadowIRRequest struct {
	Limits     CodeLimits
	CFG        *SemanticCFG
	ClassName  string
	MethodName string
	Descriptor string
	Bytecode   []byte
	IsStatic   bool
	D          *Decompiler
}

// ShadowIRBuilder is installed by package methodir. Nil means the shadow path is a no-op.
var ShadowIRBuilder func(ShadowIRRequest) (hash string, version uint64, err error)

// CaptureShadowIR is the production hook used after buildSemanticCFG.
func (d *Decompiler) CaptureShadowIR() { d.captureShadowIR() }

func (d *Decompiler) captureShadowIR() {
	d.ShadowObservation = ShadowObservation{Status: "disabled"}
	d.ShadowIRHash = ""
	d.ShadowIRVersion = 0
	if !d.EnableShadowIR {
		return
	}
	d.ShadowObservation.Status = "unavailable"
	if d.FunctionContext != nil {
		d.ShadowObservation.Method = d.FunctionContext.FunctionName + d.FunctionContext.CurrentMethodDesc
	}
	if ShadowIRBuilder == nil {
		d.ShadowObservation.Error = "shadow builder is not installed"
		return
	}
	if d.semanticCFG == nil {
		d.ShadowObservation.Error = "semantic CFG is unavailable"
		return
	}
	defer func() {
		if v := recover(); v != nil {
			d.ShadowObservation.Status = "failed"
			d.ShadowObservation.Error = fmt.Sprint(v)
			d.ShadowIRHash = ""
			d.ShadowIRVersion = 0
		}
	}()
	className, methodName, desc := "", "", ""
	static := false
	if d.FunctionContext != nil {
		className = d.FunctionContext.ClassName
		methodName = d.FunctionContext.FunctionName
		desc = d.FunctionContext.CurrentMethodDesc
		static = d.FunctionContext.IsStatic
	}
	bc := make([]byte, len(d.bytecodes))
	copy(bc, d.bytecodes)
	hash, ver, err := ShadowIRBuilder(ShadowIRRequest{
		CFG:        d.semanticCFG,
		ClassName:  className,
		MethodName: methodName,
		Descriptor: desc,
		Bytecode:   bc,
		IsStatic:   static,
		D:          d,
		Limits:     d.CodeLimits,
	})
	if err != nil {
		d.ShadowObservation.Status = "failed"
		d.ShadowObservation.Error = err.Error()
		return
	}
	if hash == "" || ver == 0 {
		d.ShadowObservation.Status = "failed"
		d.ShadowObservation.Error = "builder returned empty snapshot"
		return
	}
	d.ShadowObservation.Status = "ok"
	d.ShadowObservation.Hash = hash
	d.ShadowObservation.Version = ver
	d.ShadowIRHash = hash
	d.ShadowIRVersion = ver
}

// SnapshotSemanticCFG exposes the existing immutable CFG builder for MethodIR tests.
func SnapshotSemanticCFG(d *Decompiler) (*SemanticCFG, error) {
	return d.buildSemanticCFG()
}

// MethodBytecode copies the method's raw bytecode.
func MethodBytecode(d *Decompiler) []byte {
	out := make([]byte, len(d.bytecodes))
	copy(out, d.bytecodes)
	return out
}
