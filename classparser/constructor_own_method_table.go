package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/callbinding"

type constructorOwnMethodSignature struct {
	name, descriptor string
}

// This table belongs to one fresh parse of bytes owned by the current proof.
// It indexes declarations, not dispatch or body certificates. The caller's
// mutable root and uncertified resolver objects must always be scanned anew;
// their absence/dispatch observations are never answered by this table.
func (c *ClassObjectDumper) constructorReceiverOwnMethod(object *ClassObject, name, descriptor string, remaining *int) (*MemberInfo, bool) {
	if c == nil || object == nil || name == "" || remaining == nil || *remaining <= 0 {
		return nil, false
	}
	e := c.constructorProfileEvidence
	owned := e != nil && e.inBody && e.eligible && !e.inconsistent && object != c.obj && e.parsedOriginals[object]
	key := constructorOwnMethodSignature{name, descriptor}
	if owned {
		if table, known := e.ownMethods[object]; known {
			*remaining--
			if *remaining < 0 || !nativeProofWork(c.Work, 1) || !c.constructorOwnMethodRetentionAllowed(0) {
				return nil, false
			}
			return table[key], true
		}
	}
	// Charge every original declaration exactly as a fresh scan does. Publish
	// an index only after the whole table is unambiguous and well formed.
	var target *MemberInfo
	var table map[constructorOwnMethodSignature]*MemberInfo
	retained := int64(128)
	if owned {
		if !c.constructorOwnMethodRetentionAllowed(retained) {
			return nil, false
		}
		table = make(map[constructorOwnMethodSignature]*MemberInfo)
	}
	for _, method := range object.Methods {
		*remaining--
		if *remaining < 0 || method == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		n, nameErr := object.getUtf8(method.NameIndex)
		d, descriptorErr := object.getUtf8(method.DescriptorIndex)
		parameters, _, err := callbinding.Descriptor(d)
		if nameErr != nil || descriptorErr != nil || n == "" || err != nil || !nativeProofWork(c.Work, int64(len(n)+len(d))) {
			return nil, false
		}
		width := nativeMemberParameterWidth(parameters)
		if method.AccessFlags&8 == 0 {
			width++
		}
		if width > 255 {
			return nil, false
		}
		signature := constructorOwnMethodSignature{n, d}
		if owned {
			if _, duplicate := table[signature]; duplicate {
				return nil, false
			}
			entry := int64(96) + int64(len(n)) + int64(len(d))
			if retained > int64(^uint64(0)>>1)-entry || !c.constructorOwnMethodRetentionAllowed(retained+entry) {
				return nil, false
			}
			retained += entry
			table[signature] = method
		}
		if signature == key {
			if target != nil {
				return nil, false
			}
			target = method
		}
	}
	if owned {
		if e.ownMethods == nil {
			e.ownMethods = map[*ClassObject]map[constructorOwnMethodSignature]*MemberInfo{}
		}
		e.ownMethods[object] = table
		// Account for table retention together with the existing body evidence;
		// a later class/body must still fit the same cumulative allocation bound.
		e.bodyRetention += retained
	}
	return target, true
}

func (c *ClassObjectDumper) constructorOwnMethodRetentionAllowed(additional int64) bool {
	e := c.constructorProfileEvidence
	max := int64(^uint64(0) >> 1)
	if e == nil || additional < 0 || e.parsedRetention < 0 || e.bodyRetention < 0 || e.parsedRetention > max-e.bodyRetention || additional > max-e.parsedRetention-e.bodyRetention {
		return false
	}
	return c.Work == nil || c.Work.CheckAlloc(e.parsedRetention+e.bodyRetention+additional) == nil
}
