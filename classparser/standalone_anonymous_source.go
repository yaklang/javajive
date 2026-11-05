package javaclassparser

// A resolver-backed request may emit its closed anonymous expressions in the
// owning source unit. The JVM capture parameters are not Java constructor
// arguments: the existing physical constructor/capture proof must regenerate
// them. This publishes no archive ownership or suppression permission. Forests,
// partial ordinal prefixes and declarations needing joint access remain the
// responsibility of the archive transaction.
func (c *ClassObjectDumper) standaloneAnonymousSource() (string, bool, error) {
	if c == nil || c.obj == nil || c.foldSiblingResolver == nil || c.nativeMemberLookup != nil ||
		c.nativeMemberRoot != nil || c.nativeAnonymousRoot != nil || c.nativeAnonymousForest != nil ||
		c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil || c.isGenuineEnum() ||
		c.getenv("JDEC_NATIVE_ANONYMOUS_OFF") != "" {
		return "", false, nil
	}
	candidate := false
	for _, attribute := range c.obj.Attributes {
		if rows, ok := attribute.(*InnerClassesAttribute); ok && rows != nil {
			for _, row := range rows.Classes {
				if !nativeProofWork(c.Work, 1) {
					return "", false, c.checkWork()
				}
				candidate = candidate || row != nil && row.InnerNameIndex == 0 && row.InnerClassAccessFlags&0x1000 == 0
			}
		}
	}
	if !candidate || !nativeMemberTopLevelEvidence(c.obj, c.Work) {
		return "", false, c.checkWork()
	}
	d := NewClassObjectDumper(c.obj)
	d.options, d.Work = c.options, c.Work
	d.foldSiblingResolver, d.declarationResolver = c.foldSiblingResolver, c.declarationResolver
	d.nativeStandaloneAssertionOff = c.nativeStandaloneAssertionOff
	p := d.planNativeAnonymousFamilyWithinMembers(nil)
	if p == nil || p.failed || len(p.children) == 0 || p.forest != nil || p.independentRoot != nil ||
		p.enumPrefix != 0 || len(p.standalone) != 0 || len(p.bridges) != 0 {
		return "", false, c.checkWork()
	}
	for _, child := range p.children {
		if child == nil || child.lambdaImplementation != nil || child.memberSuper != nil || child.artifactStaticMetadata {
			return "", false, c.checkWork()
		}
	}
	// Rendering is speculative until every physical allocation was consumed by
	// exactly one source expression. Failed candidates keep the caller's ledger
	// and ownership unchanged; their shared work charges are not refunded.
	if c.report != nil {
		copy := *c.report
		copy.Members = append([]MemberRecord(nil), copy.Members...)
		copy.RulesApplied = append([]RewriteRecord(nil), copy.RulesApplied...)
		copy.Diagnostics = append([]DecompileDiagnostic(nil), copy.Diagnostics...)
		d.report = &copy
	}
	d.nativeAnonymousRoot = p
	source, err := d.DumpClass()
	if err != nil || !p.completeSource(source) || d.sourceDegraded {
		return "", false, c.checkWork()
	}
	if c.report != nil {
		*c.report = *d.report
		d.report = c.report
	}
	*c = *d
	return source, true, nil
}
