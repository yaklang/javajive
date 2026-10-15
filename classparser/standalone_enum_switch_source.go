package javaclassparser

import "strings"

// A resolver-backed class request can regenerate its own compiler table when
// the entire declaration/initializer/ordinal and original switch operands are
// representable in this one source unit. This is not an archive certificate:
// usersClosedRoot stays unset, and no separate source entry is suppressed or
// exposed through a member lookup. Other lexical declarations need JarFS's
// complete ownership transaction instead.
func (c *ClassObjectDumper) standaloneEnumSwitchSource() (string, bool, error) {
	if c == nil || c.obj == nil || c.foldSiblingResolver == nil || c.nativeMemberLookup != nil ||
		c.nativeMemberRoot != nil || c.nativeAnonymousRoot != nil || c.getenv("JDEC_NO_ENUM_SWITCH_FOLD") != "" {
		return "", false, nil
	}
	candidateRow := false
	for _, attribute := range c.obj.Attributes {
		if rows, ok := attribute.(*InnerClassesAttribute); ok && rows != nil {
			for _, row := range rows.Classes {
				if row != nil && row.InnerNameIndex == 0 && row.InnerClassAccessFlags == 0x1008 {
					candidateRow = true
				}
			}
		}
	}
	if !candidateRow || c.isGenuineEnum() || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return "", false, c.checkWork()
	}
	d := NewClassObjectDumper(c.obj)
	d.options, d.Work = c.options, c.Work
	d.foldSiblingResolver, d.declarationResolver = c.foldSiblingResolver, c.declarationResolver
	d.nativeStandaloneAssertionOff = c.nativeStandaloneAssertionOff
	p := d.planNativeMemberFamily()
	if p == nil || p.failed || len(p.enumSwitchTables) == 0 || len(p.children) != 0 || len(p.methodLocals) != 0 ||
		len(p.enumConstants) != 0 || len(p.anonymousUnits) != 0 || p.anonymous != nil || p.anonymousForest != nil ||
		len(p.modernNestObjects) != 0 || len(p.emptyMarkers) != 0 || len(p.rootAccessBridges) != 0 ||
		len(p.getters) != 0 || len(p.retainedAccessors) != 0 || !nativeEnumSwitchOrdinalClosed(p, d.Work) {
		return "", false, c.checkWork()
	}
	own := map[string]*ClassObject{p.owner: c.obj}
	foreign := map[string]bool{}
	for name, table := range p.enumSwitchTables {
		if !nativeEnumSwitchInitializationClosed(table, d.Work) {
			return "", false, c.checkWork()
		}
		for _, array := range table.tables {
			raw, known := d.foldSiblingResolver(array.enum)
			if !known {
				return "", false, c.checkWork()
			}
			enum, err := d.parseResolved(raw)
			if err != nil || enum.GetClassName() != array.enum || nativeMemberEnumSynthesisProof(enum, 0x4019, d.Work) == nil {
				return "", false, c.checkWork()
			}
			constants := map[string]bool{}
			for _, field := range enum.Fields {
				if field.AccessFlags&0x4000 != 0 {
					value, known := sourceBridgeUTF8(enum, field.NameIndex)
					if !known {
						return "", false, c.checkWork()
					}
					constants[value] = true
				}
			}
			for _, value := range array.entries {
				if !constants[value] {
					return "", false, c.checkWork()
				}
			}
		}
		table.uses = map[string]map[string]map[int]*nativeEnumSwitchUse{}
		count, known := d.nativeEnumSwitchObjectUses(p, table, name, own, foreign, true)
		if !known || count == 0 {
			return "", false, c.checkWork()
		}
	}
	// Candidate rendering has private ledgers. A failed proof must not leak
	// method statuses, rules or diagnostics into the independently rendered
	// fallback. Work is shared and never refunded, including failed attempts.
	if c.report != nil {
		copy := *c.report
		copy.Members = append([]MemberRecord(nil), copy.Members...)
		copy.RulesApplied = append([]RewriteRecord(nil), copy.RulesApplied...)
		copy.Diagnostics = append([]DecompileDiagnostic(nil), copy.Diagnostics...)
		d.report = &copy
	}
	d.nativeMemberRoot = p
	source, err := d.DumpClass()
	if err != nil || p.failed || !nativeEnumSwitchSourceComplete(p, source, d.Work) ||
		!nativeMemberPrivateGetterSourceClosed(p, source, d.Work) || strings.Contains(source, DecompileStubMarker) {
		return "", false, c.checkWork()
	}
	if c.report != nil {
		*c.report = *d.report
		d.report = c.report
	}
	*c = *d
	return source, true, nil
}
