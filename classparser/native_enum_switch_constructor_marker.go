package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

// With no real root anonymous expression, javac's first generated class can
// simultaneously be the enum-switch table and an unused constructor marker.
// Naming a marker in a descriptor never initializes that class: the private
// bridge separately proves that its null tail is not read. The table packet,
// every table use, and every marker reference must all still close independently.
// An actual anonymous expression or separate empty marker changes the compiler
// numbering protocol, so this certificate cannot be borrowed by those layouts.
func nativeMemberJointSwitchTableMarker(p *nativeMemberFamily, marker string, work *workbudget.Budget) bool {
	if p == nil || p.failed || marker != p.owner+"$1" || len(p.emptyMarkers) != 0 || len(p.enumSwitchTables) != 1 || p.anonymous != nil && len(p.anonymous.children) != 0 || !nativeProofWork(work, 1) {
		return false
	}
	table := p.enumSwitchTables[marker]
	if table == nil || table.object == nil || table.object.GetClassName() != marker {
		return false
	}
	owner, method, known := originalAnonymousOwner(table.object)
	if !known || owner != p.owner || method != "" {
		return false
	}
	for _, attribute := range table.object.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch a := attribute.(type) {
		case *InnerClassesAttribute:
			if a == nil {
				return false
			}
			for _, row := range a.Classes {
				if row == nil {
					return false
				}
				name, known := sourceBridgeClassName(table.object, row.InnerClassInfoIndex)
				if !known || name == marker && row.InnerClassAccessFlags != 0x1008 {
					return false
				}
			}
		case *SourceFileAttribute:
			if a == nil || a.AttrLen != 2 {
				return false
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "EnclosingMethod" {
				return false
			}
		default:
			return false
		}
	}
	original := nativeEnumSwitchTableProof(table.object, work)
	if original == nil || len(original.tables) != len(table.tables) {
		return false
	}
	for field, packet := range original.tables {
		cached := table.tables[field]
		if cached == nil || cached.enum != packet.enum || len(cached.entries) != len(packet.entries) {
			return false
		}
		for key, constant := range packet.entries {
			if !nativeProofWork(work, 1) || cached.entries[key] != constant {
				return false
			}
		}
	}
	return true
}
