package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Java cannot spell ACC_SYNTHETIC. A retained compiler storage field therefore
// becomes ordinary source storage and can clash with a generated field in an
// ancestor or descendant. Alpha conversion keeps the original raw program;
// the explicit artifact restores its symbolic field identity after compilation.
// This first client requires the complete original assertion-storage protocol.
type sourceAssertionStoragePlan struct {
	owner, alias             string
	originalHash, sourceHash [32]byte
}

type sourceAssertionStorageIndex struct {
	once    sync.Once
	valid   bool
	aliases map[string]string
}

func (a *sourceFieldMetadataAuthority) assertionStorageAliases(work *workbudget.Budget) map[string]string {
	if a == nil || a.jar == nil || !nativeProofWork(work, 1) {
		return nil
	}
	index := &a.assertionIndex
	index.once.Do(func() {
		policy, err := a.jar.boundJarReleaseSelection()
		if err != nil {
			return
		}
		entries, err := policy.catalog()
		if err != nil {
			return
		}
		release, err := a.jar.effectiveJarRelease()
		if err != nil {
			return
		}
		type declaration struct {
			super      string
			interfaces []string
			declares   bool
		}
		type reference struct{ from, owner string }
		classes := map[string]declaration{}
		names := map[string]bool{}
		var refs []reference
		seen := map[string]bool{}
		total := int64(0)
		for _, path := range entries {
			if !jarCanonicalEntry(path) || !strings.HasSuffix(path, ".class") {
				continue
			}
			logical := path
			if strings.HasPrefix(path, "META-INF/") {
				version, candidate, ok := physicalClassNamespace(path)
				if !ok || version > release {
					continue
				}
				logical = candidate
			}
			if seen[logical] {
				continue
			}
			seen[logical] = true
			if len(seen) > 16384 || !nativeProofWork(work, 1) {
				return
			}
			raw, err := a.jar.readJarEntry(logical)
			if err != nil {
				return
			}
			total += int64(len(raw))
			if total > 128<<20 || !nativeProofWork(work, int64(len(raw))) || work != nil && work.CheckAlloc(total*4) != nil {
				return
			}
			object, err := ParseWithBudget(raw, work)
			if err != nil || object.GetClassName()+".class" != logical {
				return
			}
			owner := object.GetClassName()
			d := declaration{}
			if object.SuperClass != 0 {
				d.super, _ = sourceBridgeClassName(object, object.SuperClass)
			}
			for _, iface := range object.Interfaces {
				n, ok := sourceBridgeClassName(object, iface)
				if !ok {
					return
				}
				d.interfaces = append(d.interfaces, n)
			}
			for _, field := range object.Fields {
				if field == nil || !nativeProofWork(work, 1) {
					return
				}
				n, nk := sourceBridgeUTF8(object, field.NameIndex)
				desc, dk := sourceBridgeUTF8(object, field.DescriptorIndex)
				if !nk || !dk {
					return
				}
				names[n] = true
				if n == nativeAssertionField && desc == "Z" {
					if d.declares {
						return
					}
					d.declares = true
				}
			}
			classes[owner] = d
			for _, constant := range object.ConstantPool {
				if !nativeProofWork(work, 1) {
					return
				}
				field, ok := constant.(*ConstantFieldrefInfo)
				if !ok {
					continue
				}
				if field == nil || field.NameAndTypeIndex == 0 || int(field.NameAndTypeIndex) > len(object.ConstantPool) {
					return
				}
				nt, ok := object.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					return
				}
				n, nk := sourceBridgeUTF8(object, nt.NameIndex)
				desc, dk := sourceBridgeUTF8(object, nt.DescriptorIndex)
				if !nk || !dk {
					return
				}
				// A symbolic owner can inherit storage from a binary dependency
				// outside this archive. Its member spelling is still occupied:
				// introducing a same-named own field would capture that source use.
				names[n] = true
				if n != nativeAssertionField || desc != "Z" {
					continue
				}
				target, ok := sourceBridgeClassName(object, field.ClassIndex)
				if !ok {
					return
				}
				refs = append(refs, reference{owner, target})
			}
		}
		// A foreign or inherited symbolic read must never keep the old spelling
		// while its declaration changes. Resolve before permitting a local alias.
		unsafe := map[string]bool{}
		var resolve func(string, map[string]bool) (string, bool)
		resolve = func(owner string, active map[string]bool) (string, bool) {
			if owner == "" || owner == "java/lang/Object" {
				return "", true
			}
			if active[owner] || len(active) >= 64 || !nativeProofWork(work, 1) {
				return "", false
			}
			d, ok := classes[owner]
			if !ok {
				return "", false
			}
			if d.declares {
				return owner, true
			}
			active[owner] = true
			defer delete(active, owner)
			found := ""
			for _, iface := range d.interfaces {
				candidate, ok := resolve(iface, active)
				if !ok || found != "" && candidate != "" && found != candidate {
					return "", false
				}
				if candidate != "" {
					found = candidate
				}
			}
			if found != "" {
				return found, true
			}
			return resolve(d.super, active)
		}
		for _, ref := range refs {
			target, ok := resolve(ref.owner, map[string]bool{})
			if !ok || target == "" {
				return
			}
			if target != ref.from || target != ref.owner {
				unsafe[target] = true
			}
		}
		owners := make([]string, 0, len(classes))
		for owner := range classes {
			owners = append(owners, owner)
		}
		sort.Strings(owners)
		index.aliases = map[string]string{}
		for ordinal, owner := range owners {
			if !classes[owner].declares || unsafe[owner] {
				continue
			}
			base := fmt.Sprintf("$jdec$storage$%d", ordinal)
			alias := base
			suffix := 0
			for names[alias] {
				if !nativeProofWork(work, 1) {
					return
				}
				alias = fmt.Sprintf("%s$%d", base, suffix)
				suffix++
			}
			names[alias] = true
			index.aliases[owner] = alias
		}
		index.valid = true
	})
	if !index.valid || !nativeProofWork(work, 1) {
		return nil
	}
	return index.aliases
}

func (c *ClassObjectDumper) prepareRawAssertionStorage() {
	if c != nil && c.rawAssertionStorage != nil {
		if c.FuncCtx != nil {
			delete(c.FuncCtx.StaticFieldAliases, c.rawAssertionStorage.owner+"\x00"+nativeAssertionField)
		}
		c.rawAssertionStorage = nil
	}
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.options.sourceFieldMetadata == nil || c.nativeAssertionProtocol() != nil || c.nativeCaptureFields != nil || c.nativeMemberCurrent != nil || c.nativeMethodLocalCurrent != nil || c.nativeEnumConstantCurrent != nil {
		return
	}
	// Method-local and anonymous declarations have independent constructor,
	// capture and lexical publication protocols. A flat storage projection is
	// permitted only for an originally named declaration outside those scopes.
	for _, attribute := range c.obj.Attributes {
		if raw, ok := attribute.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" {
			return
		}
		if table, ok := attribute.(*InnerClassesAttribute); ok {
			if table == nil {
				return
			}
			for _, row := range table.Classes {
				if row == nil || row.InnerClassInfoIndex == c.obj.ThisClass && row.InnerNameIndex == 0 {
					return
				}
			}
		}
	}
	a := c.options.sourceFieldMetadata
	owner := c.obj.GetClassName()
	present := false
	for _, field := range c.obj.Fields {
		if field == nil || !nativeProofWork(c.Work, 1) {
			return
		}
		name, ok := sourceBridgeUTF8(c.obj, field.NameIndex)
		present = present || ok && name == nativeAssertionField
	}
	if !present {
		return
	}
	raw, err := a.jar.readJarEntry(owner + ".class")
	if err != nil || !nativeProofWork(c.Work, int64(len(raw))*2) || !bytes.Equal(raw, c.obj.Bytes()) {
		return
	}
	status := ""
	for _, method := range c.obj.Methods {
		n, ok := sourceBridgeUTF8(c.obj, method.NameIndex)
		if !ok || n != "<clinit>" {
			continue
		}
		ops, ok := nativeEnumMethodOps(c.obj, method, c.Work)
		if !ok || len(ops) == 0 {
			return
		}
		first := ops[0]
		pool := 0
		if nativeEnumOpcode(first, core.OP_LDC) && len(first.Data) == 1 {
			pool = int(first.Data[0])
		} else if nativeEnumOpcode(first, core.OP_LDC_W) && len(first.Data) == 2 {
			pool = int(binary.BigEndian.Uint16(first.Data))
		} else {
			return
		}
		status, _ = sourceBridgeClassName(c.obj, uint16(pool))
	}
	packet, ok := nativeMemberAssertionProofMode(c.obj, status, c.Work, true)
	if !ok || packet == nil {
		return
	}
	alias := a.assertionStorageAliases(c.Work)[owner]
	if alias == "" {
		return
	}
	c.rawAssertionStorage = &sourceAssertionStoragePlan{owner: owner, alias: alias, originalHash: sha256.Sum256(raw)}
	c.FuncCtx.StaticFieldAliases = map[string]string{owner + "\x00" + nativeAssertionField: alias}
}

func (c *ClassObjectDumper) publishRawAssertionStorage(source string) bool {
	p := c.rawAssertionStorage
	if p == nil {
		return true
	}
	if c.sourceDegraded || legacySourceHasFailureMarker(source) || sha256.Sum256(c.obj.Bytes()) != p.originalHash || c.Work != nil && c.Work.Err() != nil {
		return false
	}
	copy := *p
	copy.sourceHash = sha256.Sum256([]byte(source))
	a := c.options.sourceFieldMetadata
	a.aliasMu.Lock()
	defer a.aliasMu.Unlock()
	if a.aliasPending == nil {
		a.aliasPending = map[string]*sourceAssertionStoragePlan{}
	}
	a.aliasPending[p.owner] = &copy
	return true
}

func (a *sourceFieldMetadataAuthority) assertionSourceReturned(owner string, source []byte) error {
	a.aliasMu.Lock()
	defer a.aliasMu.Unlock()
	p := a.aliasPending[owner]
	if p == nil || p.sourceHash != sha256.Sum256(source) {
		if a.aliasActive[owner] != nil {
			return fmt.Errorf("field identity: changed source requires a fresh artifact")
		}
		return nil
	}
	if prior := a.aliasActive[owner]; prior != nil && *prior != *p {
		return fmt.Errorf("field identity: conflicting source requires a fresh artifact")
	}
	if a.aliasActive == nil {
		a.aliasActive = map[string]*sourceAssertionStoragePlan{}
	}
	a.aliasActive[owner] = p
	return nil
}

func (a *sourceFieldMetadataAuthority) restoreAssertionStorage(original, compiled []byte, work *workbudget.Budget) ([]byte, error) {
	object, err := ParseWithBudget(original, work)
	if err != nil {
		return nil, err
	}
	plan := a.assertionStoragePlan(object.GetClassName(), sha256.Sum256(original), work)
	if plan == nil {
		return compiled, nil
	}
	if plan.originalHash != sha256.Sum256(original) {
		return nil, fmt.Errorf("field identity: original authority changed")
	}
	if !nativeProofWork(work, int64(len(compiled))+1) {
		return nil, work.Err()
	}
	rebuilt, err := ParseWithBudget(compiled, work)
	if err != nil {
		return nil, err
	}
	if rebuilt.GetClassName() != plan.owner {
		return nil, fmt.Errorf("field identity: compiled owner changed")
	}
	expected, err := sourceFieldIdentities(object, work)
	if err != nil {
		return nil, err
	}
	actual, err := sourceFieldIdentities(rebuilt, work)
	if err != nil {
		return nil, err
	}
	wanted := sourceFieldIdentity{nativeAssertionField, "Z"}
	renamed := sourceFieldIdentity{plan.alias, "Z"}
	_, aliased := actual[renamed]
	if aliased {
		if _, duplicate := actual[wanted]; duplicate {
			return nil, fmt.Errorf("field identity: duplicate original and source declaration")
		}
		actual[wanted] = actual[renamed]
		delete(actual, renamed)
	}
	if len(actual) != len(expected) {
		return nil, fmt.Errorf("field identity: field universe changed")
	}
	for key, flags := range expected {
		got, ok := actual[key]
		if !ok || flags & ^syntheticFieldFlag != got & ^syntheticFieldFlag || got&syntheticFieldFlag != 0 && flags&syntheticFieldFlag == 0 {
			return nil, fmt.Errorf("field identity: declaration contract changed for %s %s (flags %#x, want %#x)", key.name, key.descriptor, got, flags)
		}
	}
	for _, constant := range rebuilt.ConstantPool {
		if !nativeProofWork(work, 1) {
			return nil, work.Err()
		}
		ref, ok := constant.(*ConstantFieldrefInfo)
		if !ok {
			continue
		}
		if ref == nil || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(rebuilt.ConstantPool) {
			return nil, fmt.Errorf("field identity: invalid field reference")
		}
		nt, ok := rebuilt.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			return nil, fmt.Errorf("field identity: invalid name-and-type")
		}
		name, _ := sourceBridgeUTF8(rebuilt, nt.NameIndex)
		if name != plan.alias {
			continue
		}
		owner, ok := sourceBridgeClassName(rebuilt, ref.ClassIndex)
		desc, dk := sourceBridgeUTF8(rebuilt, nt.DescriptorIndex)
		if !aliased || !ok || owner != plan.owner || !dk || desc != "Z" {
			return nil, fmt.Errorf("field identity: unowned source spelling")
		}
	}
	if !aliased {
		return compiled, nil
	}
	extra := 1 // one new NameAndType; UTF8 entries can already be shared.
	for _, text := range []string{nativeAssertionField, "Z"} {
		if rebuilt.ConstantPoolManager.SearchUtf8Index(text) == 0 {
			extra++
		}
	}
	if len(rebuilt.ConstantPool)+extra > 65534 {
		return nil, fmt.Errorf("field identity: constant pool limit")
	}
	if work != nil {
		if err := work.CheckAlloc(int64(len(compiled))*3 + 1024); err != nil {
			return nil, err
		}
	}
	nameIndex := uint16(rebuilt.ConstantPoolManager.AddUtf8Info(nativeAssertionField))
	descriptorIndex := uint16(rebuilt.ConstantPoolManager.AddUtf8Info("Z"))
	ntIndex := uint16(rebuilt.ConstantPoolManager.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: nameIndex, DescriptorIndex: descriptorIndex}))
	for _, field := range rebuilt.Fields {
		name, _ := sourceBridgeUTF8(rebuilt, field.NameIndex)
		if name == plan.alias {
			field.NameIndex = nameIndex
		}
	}
	for _, constant := range rebuilt.ConstantPool {
		ref, ok := constant.(*ConstantFieldrefInfo)
		if !ok || ref == nil {
			continue
		}
		nt, ok := rebuilt.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			continue
		}
		name, _ := sourceBridgeUTF8(rebuilt, nt.NameIndex)
		if name == plan.alias {
			ref.NameAndTypeIndex = ntIndex
		}
	}
	result := rebuilt.Bytes()
	if work != nil {
		if err := work.Charge(workbudget.CounterOutputBytes, int64(len(result))); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (a *sourceFieldMetadataAuthority) hasAssertionStorage(owner string) bool {
	a.aliasMu.Lock()
	present := a.aliasActive[owner] != nil
	a.aliasMu.Unlock()
	return present || a.aliasParent != nil && a.aliasParent.hasAssertionStorage(owner)
}

// A versioned javac pass can recompile a BASE source implicitly. That does not
// publish a new source occurrence in the versioned view. Rebind the original
// publication only for byte-identical original authority and a freshly closed
// field-use namespace; different versions and foreign reads confer no edit.
func (a *sourceFieldMetadataAuthority) assertionStoragePlan(owner string, originalHash [32]byte, work *workbudget.Budget) *sourceAssertionStoragePlan {
	a.aliasMu.Lock()
	own := a.aliasActive[owner]
	a.aliasMu.Unlock()
	if own != nil {
		return own
	}
	for parent := a.aliasParent; parent != nil; parent = parent.aliasParent {
		if a.jar.sourceCompiler.effective() != parent.jar.sourceCompiler.effective() || a.jar.targetSourceVersion != parent.jar.targetSourceVersion {
			return nil
		}
		parent.aliasMu.Lock()
		plan := parent.aliasActive[owner]
		parent.aliasMu.Unlock()
		if plan != nil && plan.originalHash == originalHash {
			if a.assertionStorageAliases(work)[owner] != "" {
				return plan
			}
			return nil
		}
	}
	return nil
}
