package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

const nativeConstructorRegistrationPrefix = "jdec-owned-constructor:"

// Legacy javac uses one accessed-symbol sequence for private constructors and
// field/method accessors. A constructor consumes a position without producing
// an access$NNN method. The source event must come from a proved original
// marker bridge and allocation/delegation, never an invented missing number.
func nativeMemberConstructorRegistrations(p *nativeMemberFamily, work *workbudget.Budget) (map[string]bool, bool) {
	if p == nil {
		return nil, false
	}
	out := map[string]bool{}
	for owner, bridges := range p.bridgeOwners() {
		for desc, b := range bridges {
			obj := p.lexicalObjects[owner]
			if b == nil || obj == nil || !nativeProofWork(work, 1) || !nativeMemberJointBridgeEquivalent(p, obj, b.method, desc, work) {
				return nil, false
			}
			key := owner + ":" + b.target
			if out[key] {
				return nil, false
			}
			out[key] = true
		}
	}
	return out, true
}

func nativeMemberConstructorRegistration(p *nativeMemberFamily, owner, desc string) string {
	if p == nil || p.failed {
		return ""
	}
	bridge := p.constructorBridges(owner)[desc]
	if bridge == nil {
		p.failed = true
		return ""
	}
	return "/*" + nativeConstructorRegistrationPrefix + owner + ":" + bridge.target + "*/"
}
