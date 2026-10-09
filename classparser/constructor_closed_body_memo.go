package javaclassparser

import (
	"encoding/binary"
	"sort"
)

type constructorClosedBodyKey struct {
	object   *ClassObject
	target   *MemberInfo
	code     *CodeAttribute
	operands string
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
	cost := len(arguments) + len(writes) + len(active) + 1
	*remaining -= cost
	if *remaining < 0 || !nativeProofWork(c.Work, int64(cost)) {
		return constructorClosedBodyKey{}, false
	}
	size := 64 + len(arguments)*12 + (len(writes)+len(active))*16
	for key := range writes {
		size += len(key) + 5
	}
	for key := range active {
		size += len(key) + 5
	}
	if !nativeProofWork(c.Work, int64(size)) || c.Work != nil && c.Work.CheckAlloc(e.parsedRetention+e.bodyRetention+int64(size)) != nil {
		return constructorClosedBodyKey{}, false
	}
	state := []byte(constructorEffectFrameKey(depth, nil, arguments, true, nil, 0))
	for _, set := range []map[string]bool{writes, active} {
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
	return constructorClosedBodyKey{object: object, target: target, code: code, operands: string(state)}, true
}
