package values

import "github.com/yaklang/javajive/classparser/decompiler/core/class_context"

// The source frontend proves that an immutable original hidden field is the
// compiler storage for an enclosing effectively-final variable. This receipt
// transports that single source port; it grants no ordinary-field purity,
// body, access, constructor or factory permission.
type LambdaLexicalFieldCapture struct {
	self                     *LambdaLexicalFieldCapture
	factoryPC, index, readPC int
	snapshot                 *JavaRef
	seed                     JavaValue
	field                    *RefMember
	receiver                 *JavaRef
	owner, name, descriptor  string
}

func NewLambdaLexicalFieldCapture(snapshot *JavaRef, owner, name, descriptor string, readPC int) *LambdaLexicalFieldCapture {
	if snapshot == nil || snapshot.Id == nil || owner == "" || name == "" || descriptor == "" || len(owner)+len(name)+len(descriptor) > 1<<18 {
		return nil
	}
	pc, index, pending := snapshot.OriginalPendingDynamicOperand(snapshot.Val)
	v, known := lambdaLexicalFieldSeed(snapshot.Val)
	field, typed := v.(*RefMember)
	if !pending || !known || !typed || field == nil {
		return nil
	}
	v, known = lambdaLexicalFieldSeed(field.Object)
	receiver, typed := v.(*JavaRef)
	if !known || !typed || receiver == nil {
		return nil
	}
	p := &LambdaLexicalFieldCapture{factoryPC: pc, index: index, readPC: readPC, snapshot: snapshot, seed: snapshot.Val, field: field, receiver: receiver, owner: owner, name: name, descriptor: descriptor}
	p.self = p
	if !p.pending() {
		return nil
	}
	return p
}

// Slot wrappers are transport, not a different field or receiver. Do not
// follow local histories, casts, callbacks or arbitrary cycles here.
func lambdaLexicalFieldSeed(value JavaValue) (JavaValue, bool) {
	for depth := 0; depth < 32 && !isNilJavaValue(value); depth++ {
		if slot, ok := value.(*SlotValue); ok {
			value = slot.GetValue()
			continue
		}
		return value, true
	}
	return nil, false
}

func (p *LambdaLexicalFieldCapture) pending() bool {
	if p == nil || p.self != p || p.snapshot == nil || p.field == nil || p.receiver == nil || !sameOriginalValueIdentity(p.snapshot.Val, p.seed) || p.readPC < 0 || p.readPC >= p.factoryPC {
		return false
	}
	pc, index, pending := p.snapshot.OriginalPendingDynamicOperand(p.seed)
	field, known := lambdaLexicalFieldSeed(p.seed)
	receiver, own := lambdaLexicalFieldSeed(p.field.Object)
	slot, initialized := p.receiver.OriginalReceiverSlot()
	return pending && pc == p.factoryPC && index == p.index && known && field == p.field && own && receiver == p.receiver && initialized && slot == 0 && p.receiver.IsThis && p.field.OriginalInstanceFieldRead(p.readPC, p.owner, p.name, p.descriptor)
}

func (v *CustomValue) BindLambdaLexicalFieldCaptures(ports []*LambdaLexicalFieldCapture) bool {
	if len(ports) == 0 {
		return true
	}
	if v == nil || v.Flag != "lambda" || v.IsMethodRef || !v.lambdaParameterNamesRequired || v.lambdaSourceIdentity == nil || v.lambdaSourceIdentity.self != v.lambdaSourceIdentity || v.lambdaLexicalFieldCaptures != nil || !v.CapturesKnown || len(ports) > 64 || len(v.Captures) > 64 {
		return false
	}
	bindings := map[int]*LambdaLexicalFieldCapture{}
	for _, port := range ports {
		if !port.pending() || port.index < 0 || port.index >= len(v.Captures) || v.Captures[port.index] != port.snapshot || bindings[port.index] != nil {
			return false
		}
		bindings[port.index] = port
	}
	v.lambdaLexicalFieldCaptures = bindings
	v.lambdaLexicalFieldIdentity = v.lambdaSourceIdentity
	return true
}

// A source port alone cannot elide an eager operand. The validated adapter
// must first seal the exact original factory and ordered captured snapshots.
func (v *CustomValue) OriginalLambdaLexicalFieldReceiver(pc, index int, snapshot *JavaRef, seed JavaValue, context *class_context.ClassContext) (*JavaRef, bool) {
	if v == nil || context == nil || context.IsStatic || v.IsMethodRef || !v.lambdaParameterNamesRequired || v.lambdaSourceIdentity == nil || v.lambdaSourceIdentity.self != v.lambdaSourceIdentity || v.lambdaLexicalFieldIdentity != v.lambdaSourceIdentity {
		return nil, false
	}
	port := v.lambdaLexicalFieldCaptures[index]
	location, captures, sealed := v.OriginalLambdaFactoryEvaluation()
	if !port.pending() || !sealed || location != pc || port.factoryPC != pc || index < 0 || index >= len(captures) || captures[index] != snapshot || port.snapshot != snapshot || !sameOriginalValueIdentity(seed, port.seed) || !sameOriginalFieldOwner(context.ClassName, port.owner) {
		return nil, false
	}
	descriptor, typed := SourceTypeErasure(port.field.JavaType, context)
	if !typed || descriptor != port.descriptor {
		return nil, false
	}
	return port.receiver, true
}
