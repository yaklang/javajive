package javaclassparser

// A proved enum-switch cache has an EnclosingMethod owner but no executable
// anonymous expression. Discovery and grouping must agree on that distinction:
// otherwise a table-only root requires a group which grouping correctly omits.
// This role grants no source body, private access, numbering or table-use
// permission; those obligations still close in the complete source transaction.
func (c *ClassObjectDumper) nativeMemberEnumSwitchAnonymousRole(p *nativeMemberFamily, object *ClassObject) bool {
	if c == nil || p == nil || p.failed || object == nil || c.obj == nil || p.lexicalObjects[p.owner] != c.obj || c.obj.GetClassName() != p.owner || !nativeProofWork(c.Work, 1) {
		return false
	}
	name := object.GetClassName()
	major, minor := object.MajorVersion, object.MinorVersion
	cached := p.enumSwitchTables[name]
	if cached == nil || cached.object == nil || cached.object.GetClassName() != name || object.MajorVersion != cached.object.MajorVersion || object.MinorVersion != cached.object.MinorVersion || object.MajorVersion != c.obj.MajorVersion || object.MinorVersion != c.obj.MinorVersion {
		return false
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	// Reopen both the provider observation and the planned object's metadata.
	// A name in a cached map is not a certificate for a changed original body.
	fresh := nativeEnumSwitchTableProof(object, c.Work)
	original := nativeEnumSwitchTableProof(cached.object, c.Work)
	if fresh == nil || original == nil || !nativeEnumSwitchReferencedMetadata(fresh, p.owner, resolve, c.Work) || !nativeEnumSwitchReferencedMetadata(original, p.owner, resolve, c.Work) {
		return false
	}
	// Metadata resolution can call the provider. Recheck instruction packets
	// afterward, including the physical initialization order and every case.
	fresh = nativeEnumSwitchTableProof(object, c.Work)
	original = nativeEnumSwitchTableProof(cached.object, c.Work)
	if !nativeEnumSwitchSameInitialization(cached, fresh, c) || !nativeEnumSwitchSameInitialization(cached, original, c) || p.enumSwitchTables[name] != cached {
		return false
	}
	for _, candidate := range []*ClassObject{object, cached.object} {
		owner, method, known := originalAnonymousOwner(candidate)
		if !known || owner != p.owner || method != "" {
			return false
		}
	}
	// Keep the reciprocal current root row, not merely the helper's self row.
	matches := 0
	for _, attribute := range c.obj.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return false
		}
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return false
			}
			for _, row := range table.Classes {
				if row == nil || !nativeProofWork(c.Work, 1) {
					return false
				}
				binary, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
				if !known {
					return false
				}
				if binary == name {
					if row.OuterClassInfoIndex != 0 || row.InnerNameIndex != 0 || row.InnerClassAccessFlags != 0x1008 {
						return false
					}
					matches++
				}
			}
		}
	}
	return matches == 1 && !p.failed && p.lexicalObjects[p.owner] == c.obj && c.obj.GetClassName() == p.owner &&
		object.MajorVersion == major && object.MinorVersion == minor && cached.object.MajorVersion == major && cached.object.MinorVersion == minor && c.obj.MajorVersion == major && c.obj.MinorVersion == minor
}

func nativeEnumSwitchSameInitialization(planned, observed *nativeEnumSwitchTable, c *ClassObjectDumper) bool {
	if !nativeEnumSwitchInitializationClosed(planned, c.Work) || !nativeEnumSwitchInitializationClosed(observed, c.Work) || len(planned.tables) != len(observed.tables) {
		return false
	}
	for i, field := range planned.initializationOrder {
		if !nativeProofWork(c.Work, 1) || observed.initializationOrder[i] != field {
			return false
		}
		a, b := planned.tables[field], observed.tables[field]
		if a == nil || b == nil || a.enum != b.enum || len(a.entries) != len(b.entries) {
			return false
		}
		for key, constant := range a.entries {
			if !nativeProofWork(c.Work, 1) || b.entries[key] != constant {
				return false
			}
		}
	}
	return true
}
