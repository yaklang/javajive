package javaclassparser

import (
	"encoding/binary"
	"sort"
)

type constructorClosedBodyKey struct {
	leaf     bool
	object   *ClassObject
	target   *MemberInfo
	code     *CodeAttribute
	operands string
	context  string
	depth    int
}
type constructorClosedBodyMemo struct {
	providerEpoch             uint64
	selfStored, referenceRead bool
}

// Binding, method flags, descriptor, argument widths, recursion and the exact
// Code attribute are checked before this lookup. The memo is local to one
// profile/request and accepts only fresh parsed originals, never caller-owned
// trees. A first body must make no external or root-table observation. On later
// calls pure root-table binding queries are repeated, while any provider attempt
// invalidates the body certificate. No bytecode/runtime operation is removed.
func (c *ClassObjectDumper) constructorClosedBodyKey(object *ClassObject, target *MemberInfo, code *CodeAttribute, arguments []constructorEffectValue, writes, active map[string]bool, remaining *int, depth int) (constructorClosedBodyKey, bool) {
	e := c.constructorProfileEvidence
	if e == nil || !e.inBody || !e.eligible || e.inconsistent || object == c.obj || !e.parsedOriginals[object] {
		return constructorClosedBodyKey{}, false
	}
	cost := len(arguments) + len(writes) + 1
	*remaining -= cost
	if *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) {
		return constructorClosedBodyKey{}, false
	}
	size := 64 + len(arguments)*12 + len(writes)*16
	for key := range writes {
		size += len(key) + 5
	}
	if !nativeProofWork(c.Work, int64(size)) || !c.constructorOwnMethodRetentionAllowed(int64(size)) {
		return constructorClosedBodyKey{}, false
	}
	// Actuals and moved storage are shared by the leaf and full-context
	// alternatives. Encode them once, independently of the changing context.
	state := []byte(constructorEffectFrameKey(0, nil, arguments, true, nil, 0))
	for _, set := range []map[string]bool{writes} {
		keys := make([]string, 0, len(set))
		for key := range set {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		state = binary.BigEndian.AppendUint32(state, uint32(len(keys)))
		for _, key := range keys {
			state = binary.BigEndian.AppendUint32(state, uint32(len(key)))
			state = append(state, key...)
			if set[key] {
				state = append(state, 1)
			} else {
				state = append(state, 0)
			}
		}
	}
	key := constructorClosedBodyKey{object: object, target: target, code: code, operands: string(state)}
	if active != nil {
		return c.constructorClosedBodyContext(key, active, remaining, depth)
	}
	key.depth = depth
	return key, true
}

func (c *ClassObjectDumper) constructorClosedBodyContext(base constructorClosedBodyKey, active map[string]bool, remaining *int, depth int) (constructorClosedBodyKey, bool) {
	e := c.constructorProfileEvidence
	if remaining == nil || e == nil || !e.inBody || !e.eligible || e.inconsistent || base.object == c.obj || !e.parsedOriginals[base.object] {
		return constructorClosedBodyKey{}, false
	}
	*remaining -= len(active) + 1
	if *remaining < 0 || !nativeProofWork(c.Work, int64(len(active)+1)) {
		return constructorClosedBodyKey{}, false
	}
	size := int64(64 + len(active)*16 + len(base.operands))
	for name := range active {
		size += int64(len(name)) + 5
	}
	if !nativeProofWork(c.Work, size) || !c.constructorOwnMethodRetentionAllowed(size) {
		return constructorClosedBodyKey{}, false
	}
	names := make([]string, 0, len(active))
	for name := range active {
		names = append(names, name)
	}
	sort.Strings(names)
	state := binary.BigEndian.AppendUint32(nil, uint32(len(names)))
	for _, name := range names {
		state = binary.BigEndian.AppendUint32(state, uint32(len(name)))
		state = append(state, name...)
		if active[name] {
			state = append(state, 1)
		} else {
			state = append(state, 0)
		}
	}
	base.leaf, base.depth, base.context = false, depth, string(state)
	return base, true
}
