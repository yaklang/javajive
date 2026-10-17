package javaclassparser

import "strconv"

// Enum constant bodies and ordinary anonymous expressions share javac's
// ordinal namespace, but have different allocation/source ownership proofs.
// Only the independently proved, source-ordered constant-body prefix reserves
// ordinals here. An arbitrary unnamed row cannot fill a missing expression.
func (c *ClassObjectDumper) nativeAnonymousEnumConstantPrefix(p *nativeMemberFamily) (int, bool) {
	if c == nil || c.obj == nil {
		return 0, false
	}
	if !isGenuineEnum(c.obj) {
		return 0, true
	}
	if p == nil || p.failed {
		return 0, false
	}
	owner := c.obj.GetClassName()
	parent := p.children[owner]
	if parent == nil || parent.object != c.obj || parent.enumSynthesis == nil {
		return 0, false
	}
	// The canonical no-source-argument packet is already proved directly
	// against <clinit>; it has no per-allocation map or constant bodies.
	if parent.enumSynthesis.constants == nil {
		return 0, len(parent.enumSynthesis.bodies) == 0
	}
	count := 0
	for _, field := range c.obj.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			return 0, false
		}
		if field.AccessFlags&0x4000 == 0 {
			continue
		}
		name, known := sourceBridgeUTF8(c.obj, field.NameIndex)
		plan, found := parent.enumSynthesis.constants[name]
		if !known || !found {
			return 0, false
		}
		if plan.allocatedClass == owner {
			continue
		}
		body := p.enumConstants[plan.allocatedClass]
		count++
		if body == nil || body.owner != owner || plan.allocatedClass != owner+"$"+strconv.Itoa(count) || !c.nativeMemberEnumConstantAnonymousRole(p, body.object) {
			return 0, false
		}
	}
	return count, true
}
