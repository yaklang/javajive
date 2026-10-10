package javaclassparser

import "github.com/yaklang/javajive/internal/workbudget"

type nativeEnumSwitchDeclaration struct {
	owner, name string
	flags       uint16
}

// InnerClasses carries references as well as a class's own declaration. Bind
// foreign rows to the enum types actually read by the proved initializer and
// their original named-member ancestors. This is a reference closure, never an
// ownership edge: every row must match an original self declaration exactly.
// Full archive-user, enum synthesis and compiler namespace proofs still apply.
func nativeEnumSwitchReferencedMetadata(table *nativeEnumSwitchTable, owner string, resolve func(string) (*ClassObject, bool), work *workbudget.Budget) bool {
	if table == nil || table.object == nil || resolve == nil || len(table.tables) == 0 || len(table.tables) > 16 {
		return false
	}
	references := map[string]nativeEnumSwitchDeclaration{}
	for _, array := range table.tables {
		if array == nil {
			return false
		}
		name := array.enum
		path := map[string]bool{}
		for {
			if !nativeProofWork(work, 1) || len(path) >= 64 || path[name] {
				return false
			}
			if work != nil && work.CheckAlloc(int64(len(path)+len(references)+1)*128) != nil {
				return false
			}
			path[name] = true
			declaration, known := resolve(name)
			if !known || declaration == nil || declaration.GetClassName() != name || !nativeMemberVersionMetadata(declaration, work) {
				return false
			}
			innerTables := 0
			for _, attribute := range declaration.Attributes {
				if !nativeProofWork(work, 1) {
					return false
				}
				if rows, ok := attribute.(*InnerClassesAttribute); ok {
					if rows == nil || !nativeProofWork(work, int64(len(rows.Classes)+1)) {
						return false
					}
					innerTables++
				}
			}
			if innerTables > 1 {
				return false
			}
			outer, local, flags, member := originalMemberOwner(declaration)
			if !member {
				if !nativeMemberTopLevelEvidence(declaration, work) {
					return false
				}
				break
			}
			row := nativeEnumSwitchDeclaration{outer, local, flags}
			if old, seen := references[name]; seen && old != row {
				return false
			}
			if _, seen := references[name]; !seen && len(references) >= 64 {
				return false
			}
			references[name] = row
			name = outer
		}
	}
	return nativeEnumSwitchMetadataRows(table.object, owner, references, work)
}
