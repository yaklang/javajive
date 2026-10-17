package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/fieldbinding"
)

// Read complete field declarations lazily. Invocation metadata intentionally
// describes methods and cannot prove the absence of a hiding field.
func (c *ClassObjectDumper) buildFieldBindingMetadata() fieldbinding.Provider {
	cache := map[string]fieldbinding.Class{}
	missing := map[string]bool{}
	return func(name string) (fieldbinding.Class, bool) {
		if v, ok := cache[name]; ok {
			return v, true
		}
		if missing[name] {
			return fieldbinding.Class{}, false
		}
		obj := c.obj
		if obj.GetClassName() != name {
			var raw []byte
			var known bool
			for _, resolve := range []func(string) ([]byte, bool){c.foldSiblingResolver, c.archiveDeclarationResolver, c.declarationResolver} {
				if resolve != nil {
					raw, known = resolve(name)
					if known {
						break
					}
				}
			}
			if !known {
				missing[name] = true
				return fieldbinding.Class{}, false
			}
			var err error
			obj, err = c.parseResolved(raw)
			if err != nil || obj.GetClassName() != name {
				missing[name] = true
				return fieldbinding.Class{}, false
			}
		}
		pkg := ""
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			pkg = strings.ReplaceAll(name[:i], "/", ".")
		}
		v := fieldbinding.Class{Name: name, Super: obj.GetSupperClassName(), Interfaces: obj.GetInterfacesName(), Accessible: obj.AccessFlags&1 != 0 || pkg == c.PackageName, Complete: true}
		seen := map[string]bool{}
		for _, field := range obj.Fields {
			if field == nil || !nativeProofWork(c.Work, 1) {
				missing[name] = true
				return fieldbinding.Class{}, false
			}
			fn, nk := sourceBridgeUTF8(obj, field.NameIndex)
			fd, dk := sourceBridgeUTF8(obj, field.DescriptorIndex)
			_, result, err := callbinding.Descriptor("()" + fd)
			if !nk || !dk || fn == "" || err != nil || result == "V" || seen[fn+"\x00"+fd] {
				missing[name] = true
				return fieldbinding.Class{}, false
			}
			seen[fn+"\x00"+fd] = true
			v.Fields = append(v.Fields, fieldbinding.Field{Name: fn, Descriptor: fd, Access: field.AccessFlags})
		}
		cache[name] = v
		return v, true
	}
}
