package core

// ShadowIRRequest is the read-only snapshot input for MethodIR construction.
// It is filled after buildSemanticCFG succeeds and must not alias mutable printer state.
type ShadowIRRequest struct {
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
	if !d.EnableShadowIR || ShadowIRBuilder == nil || d.semanticCFG == nil {
		return
	}
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
	})
	if err != nil {
		return
	}
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
