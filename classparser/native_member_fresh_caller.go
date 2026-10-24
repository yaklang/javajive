package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// This certificate controls local producer restoration only. It never enrolls
// an external caller in the family's private lexical scope.
type nativeFreshCallerIdentity struct {
	object  *ClassObject
	family  *nativeMemberFamily
	name    string
	lexical bool
}

func nativeFreshBindCaller(obj *ClassObject, p *nativeMemberFamily, work *workbudget.Budget) *nativeFreshCallerIdentity {
	if obj == nil || p == nil || !nativeProofWork(work, 1) || work != nil && work.CheckAlloc(64) != nil {
		return nil
	}
	name, known := sourceBridgeClassName(obj, obj.ThisClass)
	if !known || name == "" {
		return nil
	}
	original, lexical := p.lexicalObjects[name]
	if lexical && original != obj || !lexical && nativeFreshFamilyClaimsName(p, name) {
		return nil
	}
	return &nativeFreshCallerIdentity{obj, p, name, lexical}
}
func (c *ClassObjectDumper) nativeFreshCallerClosed() bool {
	proof := c.nativeFreshCaller
	if proof == nil || proof.object != c.obj || proof.family != c.nativeMemberRoot || !nativeProofWork(c.Work, 1) {
		return false
	}
	name, known := sourceBridgeClassName(c.obj, c.obj.ThisClass)
	if !known || name != proof.name {
		return false
	}
	original, listed := proof.family.lexicalObjects[name]
	if proof.lexical {
		return listed && original == c.obj
	}
	return !listed && !nativeFreshFamilyClaimsName(proof.family, name)
}

func nativeFreshFamilyClaimsName(p *nativeMemberFamily, name string) bool {
	if p == nil {
		return false
	}
	if name == p.owner {
		return true
	}
	_, member := p.children[name]
	_, anonymous := p.anonymousUnits[name]
	_, constant := p.enumConstants[name]
	_, local := p.methodLocals[name]
	return member || anonymous || constant || local
}
