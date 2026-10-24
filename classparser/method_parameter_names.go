package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Ordinary method formals have a one-to-one relation to descriptor parameters.
// A constructor participates only after its original declaration proves that
// it has no compiler-owned enclosing-instance, enum or local capture formals.
// Neither descriptor arity nor a source spelling proves a hidden-argument role.
// A present table must be completely expressible by the source compiler. A
// missing name or duplicate spelling cannot be repaired while retaining the
// original Parameter.getName()/isNamePresent() result.
func (c *ClassObjectDumper) originalOrdinaryMethodParameterNames(method *MemberInfo, descriptor string) ([]string, bool) {
	if c == nil || c.obj == nil || method == nil || !nativeProofWork(c.Work, 1) {
		return nil, false
	}
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	actual, typed := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	physical, _, err := callbinding.Descriptor(descriptor)
	if !named || !typed || actual != descriptor || err != nil || len(physical) > 255 {
		return nil, false
	}
	if name == "<clinit>" || method.AccessFlags&(0x1000|0x0040|0x0100) != 0 {
		return nil, true
	}
	var table *UnparsedAttribute
	for _, a := range method.Attributes {
		if !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		if candidate, ok := a.(*UnparsedAttribute); ok {
			if candidate == nil {
				return nil, false
			}
			if candidate.Name == "MethodParameters" {
				if table != nil {
					return nil, false
				}
				table = candidate
			}
		}
	}
	if table == nil {
		return nil, true
	}
	// There is no constructor naming projection to prove when the original
	// has no MethodParameters attribute. Inspect the physical owner only for
	// a present table; unrelated InnerClasses rows must not consume that work.
	if name == "<init>" && !c.constructorHasOnlySourceParameters(method, descriptor) {
		return nil, true
	}
	if c.obj.MajorVersion < 45 || table.Length != uint32(len(table.Info)) || len(table.Info) != 1+4*len(physical) || int(table.Info[0]) != len(physical) || !nativeProofWork(c.Work, int64(len(table.Info))) || c.Work != nil && c.Work.CheckAlloc(int64(len(physical))*192) != nil {
		return nil, false
	}
	// Bind only the unique original declaration, not a detached or duplicate
	// packet that happens to carry the same table and erased descriptor.
	matches := 0
	for _, original := range c.obj.Methods {
		if original == nil || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		n, nk := sourceBridgeUTF8(c.obj, original.NameIndex)
		d, dk := sourceBridgeUTF8(c.obj, original.DescriptorIndex)
		if !nk || !dk {
			return nil, false
		}
		if n == name && d == descriptor {
			if original != method {
				return nil, false
			}
			matches++
		}
	}
	if matches != 1 {
		return nil, false
	}
	names := make([]string, len(physical))
	seen := map[string]bool{}
	for i := range names {
		entry := table.Info[1+4*i : 5+4*i]
		index, flags := binary.BigEndian.Uint16(entry[:2]), binary.BigEndian.Uint16(entry[2:])
		text, known := sourceBridgeUTF8(c.obj, index)
		if !known || text == "" || flags & ^uint16(0x10) != 0 || !nativeProofWork(c.Work, int64(len(text))+1) || c.Work != nil && c.Work.CheckAlloc(int64(len(text))*8+128) != nil || !class_context.PreservesJava8Identifier(text) || seen[text] {
			return nil, false
		}
		names[i], seen[text] = text, true
	}
	return names, true
}

type ordinaryMethodParameterNameBinding struct {
	object     *ClassObject
	method     *MemberInfo
	name       string
	descriptor string
	names      []string
	identities []*coreutils.VariableId
}

// Entry refs carry original logical-parameter identities before source finality
// separates later slot storage. Set their preferred names before capture and
// local namespace allocation; the final header must retain those same IDs.
// This grants no invocation, capture, initialization, or ownership permission.
func (c *ClassObjectDumper) bindOrdinaryMethodParameterNames(method *MemberInfo, descriptor string, names []string, header []values.JavaValue) (*ordinaryMethodParameterNameBinding, bool) {
	fresh, known := c.originalOrdinaryMethodParameterNames(method, descriptor)
	if !known {
		return nil, false
	}
	if names == nil {
		return nil, fresh == nil
	}
	physical, _, err := callbinding.Descriptor(descriptor)
	name, named := sourceBridgeUTF8(c.obj, method.NameIndex)
	if !known || !named || !slices.Equal(names, fresh) || err != nil || len(names) != len(header) || len(header) != len(physical) || c.FuncCtx == nil || c.CurrentMethod != method || c.FuncCtx.FunctionName != name || c.FuncCtx.CurrentMethodDesc != descriptor || c.FuncCtx.IsStatic != (method.AccessFlags&8 != 0) || c.Work != nil && c.Work.CheckAlloc(int64(len(header))*128) != nil {
		return nil, false
	}
	binding := &ordinaryMethodParameterNameBinding{object: c.obj, method: method, name: name, descriptor: descriptor, names: slices.Clone(names)}
	seen := map[*coreutils.VariableId]bool{}
	slot := 0
	if method.AccessFlags&8 == 0 {
		slot = 1
	}
	for i, parameter := range header {
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id == nil || seen[ref.Id] || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		original, sealed := ref.OriginalParameterSlot()
		if !sealed || original != slot {
			return nil, false
		}
		if prior, present := c.FuncCtx.LocalNames[ref.Id]; present && prior != names[i] {
			return nil, false
		}
		binding.identities = append(binding.identities, ref.Id)
		seen[ref.Id] = true
		slot++
		if physical[i] == "J" || physical[i] == "D" {
			slot++
		}
	}
	// A complete proof precedes mutation, including simultaneous name cycles
	// such as original var1,var0. Later local declarations get fresh suffixes.
	for i, identity := range binding.identities {
		identity.SetName(names[i])
	}
	return binding, true
}

func (c *ClassObjectDumper) ordinaryMethodParameterNamesClosed(binding *ordinaryMethodParameterNameBinding, header []values.JavaValue) bool {
	if binding == nil {
		return true
	}
	if c == nil || c.obj == nil || binding.method == nil || len(binding.names) != len(binding.identities) {
		return false
	}
	fresh, known := c.originalOrdinaryMethodParameterNames(binding.method, binding.descriptor)
	name, named := sourceBridgeUTF8(c.obj, binding.method.NameIndex)
	if !known || !named || c.obj != binding.object || name != binding.name || !slices.Equal(binding.names, fresh) || len(header) != len(binding.identities) || c.FuncCtx == nil || c.CurrentMethod != binding.method || c.FuncCtx.FunctionName != binding.name || c.FuncCtx.CurrentMethodDesc != binding.descriptor || c.FuncCtx.IsStatic != (binding.method.AccessFlags&8 != 0) {
		return false
	}
	for i, parameter := range header {
		ref, ok := parameter.(*values.JavaRef)
		if !ok || ref == nil || ref.Id != binding.identities[i] || !ref.IsParam || ref.IsThis || ref.CustomValue != nil || ref.StackVar != nil || !nativeProofWork(c.Work, 1) || ref.String(c.FuncCtx) != binding.names[i] {
			return false
		}
	}
	return true
}

// Abstract source formals have declaration identities but no original JVM slot
// seed. Never mint an OriginalParameterSlot witness for a header without Code.
func (c *ClassObjectDumper) abstractMethodParameterNameIDs(names []string, count int) ([]*coreutils.VariableId, bool) {
	if count < 0 || count > 255 || names != nil && len(names) != count || !nativeProofWork(c.Work, int64(count)+1) || c.Work != nil && c.Work.CheckAlloc(int64(count)*128) != nil {
		return nil, false
	}
	identities := make([]*coreutils.VariableId, count)
	for i := range identities {
		name := fmt.Sprintf("var%d", i)
		if names != nil {
			name = names[i]
		}
		identities[i] = coreutils.NewRootVariableId()
		identities[i].SetName(name)
	}
	return identities, true
}
