package javaclassparser

import (
	"encoding/binary"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

// Compiler artifacts have an EnclosingMethod owner but no executable lexical
// method. This is not anonymous-constructor ownership. The packet certificate
// establishes only provisional planning membership; a complete archive-user and
// compiler-namespace certificate must still close before publishing source.
func (c *ClassObjectDumper) nativeEnumSwitchOwnedTables(owner string) (map[string]*nativeEnumSwitchTable, bool) {
	out := map[string]*nativeEnumSwitchTable{}
	foreignArtifact := false
	resolve := c.nativeAnnotationDeclarationResolver()
	for _, a := range c.obj.Attributes {
		inner, ok := a.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		if inner == nil {
			return nil, false
		}
		for _, row := range inner.Classes {
			if row == nil || !nativeProofWork(c.Work, 1) {
				return nil, false
			}
			if row.InnerNameIndex != 0 || row.InnerClassAccessFlags != 0x1008 {
				continue
			}
			name, known := sourceBridgeClassName(c.obj, row.InnerClassInfoIndex)
			if !known {
				return nil, false
			}
			raw, known := c.foldSiblingResolver(name)
			if !known {
				return nil, false
			}
			obj, e := c.parseResolved(raw)
			if e != nil || obj.GetClassName() != name {
				return nil, false
			}
			actual, method, anon := originalAnonymousOwner(obj)
			if anon && actual != owner && method == "" {
				foreignArtifact = true
			}
			if !anon || actual != owner || method != "" {
				continue
			}
			// Empty private-constructor markers already have a separate protocol.
			// Recognizing an enum table must not consume or disable that certificate.
			if nativeMemberEmptyAccessMarker(obj, owner, c.Work) {
				continue
			}
			table := nativeEnumSwitchTableProof(obj, c.Work)
			if c.getenv("JDEC_NO_ENUM_SWITCH_FOLD") != "" || table == nil || len(out) > 0 || !nativeEnumSwitchReferencedMetadata(table, owner, resolve, c.Work) {
				return nil, false
			}
			out[name] = table
		}
	}
	return out, !foreignArtifact || len(out) == 0
}

func nativeEnumSwitchArtifactMetadata(obj *ClassObject, owner string, work *workbudget.Budget) bool {
	return nativeEnumSwitchMetadataRows(obj, owner, nil, work)
}

func nativeEnumSwitchMetadataRows(obj *ClassObject, owner string, references map[string]nativeEnumSwitchDeclaration, work *workbudget.Budget) bool {
	if obj == nil || !nativeProofWork(work, int64(len(references)+1)) {
		return false
	}
	seenInner, seenEnclosing, seenSource := false, false, false
	for _, a := range obj.Attributes {
		if !nativeProofWork(work, 1) {
			return false
		}
		switch a := a.(type) {
		case *InnerClassesAttribute:
			if a == nil || seenInner || len(a.Classes) != len(references)+1 {
				return false
			}
			seenInner = true
			seen := map[string]bool{}
			for _, row := range a.Classes {
				if row == nil || !nativeProofWork(work, 1) {
					return false
				}
				name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !known || seen[name] {
					return false
				}
				seen[name] = true
				if name == obj.GetClassName() {
					if row.OuterClassInfoIndex != 0 || row.InnerNameIndex != 0 || row.InnerClassAccessFlags != 0x1008 {
						return false
					}
					continue
				}
				declaration, exists := references[name]
				outer, outerKnown := sourceBridgeClassName(obj, row.OuterClassInfoIndex)
				local, localKnown := sourceBridgeUTF8(obj, row.InnerNameIndex)
				if !exists || !outerKnown || !localKnown || outer != declaration.owner || local != declaration.name || row.InnerClassAccessFlags != declaration.flags {
					return false
				}
			}
			if !seen[obj.GetClassName()] {
				return false
			}
		case *UnparsedAttribute:
			if a == nil || a.Name != "EnclosingMethod" || seenEnclosing || a.Length != 4 || len(a.Info) != 4 || binary.BigEndian.Uint16(a.Info[2:]) != 0 {
				return false
			}
			seenEnclosing = true
			name, known := sourceBridgeClassName(obj, binary.BigEndian.Uint16(a.Info[:2]))
			if !known || name != owner {
				return false
			}
		case *SourceFileAttribute:
			if a == nil || seenSource || a.AttrLen != 2 {
				return false
			}
			if _, known := sourceBridgeUTF8(obj, a.SourceFileIndex); !known {
				return false
			}
			seenSource = true
		default:
			return false
		}
	}
	return seenInner && seenEnclosing
}

func nativeEnumSwitchOrdinalClosed(p *nativeMemberFamily, work *workbudget.Budget) bool {
	if p == nil {
		return false
	}
	if len(p.enumSwitchTables) > 0 {
		if len(p.emptyMarkers) > 0 || len(p.rootAccessBridges) > 0 {
			return false
		}
		for _, child := range p.children {
			if len(child.accessBridges) > 0 {
				return false
			}
		}
	}
	for name, table := range p.enumSwitchTables {
		if table == nil || table.object.GetClassName() != name || !nativeProofWork(work, 1) {
			return false
		}
		group := p.anonymous
		count := 0
		if group != nil {
			count = len(group.children)
		}
		if name != p.owner+"$"+strconv.Itoa(count+1) {
			return false
		}
		for field, arr := range table.tables {
			if arr == nil || field != "$SwitchMap$"+strings.ReplaceAll(arr.enum, "/", "$") {
				return false
			}
		}
	}
	return true
}
