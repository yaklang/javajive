package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"strings"
)

// buildInvocationMetadata reads declaration tables, not just referenced CP entries.
// Accessibility here is relative to the output compilation unit's package.
func (c *ClassObjectDumper) buildInvocationMetadata() callbinding.Provider {
	cache := map[string]callbinding.Class{}
	misses := map[string]bool{}
	return func(n string) (callbinding.Class, bool) {
		n = strings.ReplaceAll(n, ".", "/")
		if v, ok := cache[n]; ok {
			return v, true
		}
		if misses[n] {
			return callbinding.Class{}, false
		}
		if n == "java/lang/Object" {
			v := callbinding.Class{Name: n, Public: true, MembersComplete: true, ParentsComplete: true}
			for _, m := range []struct {
				name, desc string
				public     bool
			}{
				{"getClass", "()Ljava/lang/Class;", true}, {"hashCode", "()I", true},
				{"equals", "(Ljava/lang/Object;)Z", true}, {"toString", "()Ljava/lang/String;", true},
				{"notify", "()V", true}, {"notifyAll", "()V", true}, {"wait", "()V", true},
				{"wait", "(J)V", true}, {"wait", "(JI)V", true}, {"clone", "()Ljava/lang/Object;", false}, {"finalize", "()V", false},
			} {
				v.Methods = append(v.Methods, callbinding.Method{Name: m.name, Desc: m.desc, Public: m.public, Generic: m.name == "getClass"})
			}
			cache[n] = v
			return v, true
		}
		obj := c.obj
		if obj.GetClassName() != n {
			if c.foldSiblingResolver == nil {
				misses[n] = true
				return callbinding.Class{}, false
			}
			data, ok := c.foldSiblingResolver(n)
			if !ok {
				misses[n] = true
				return callbinding.Class{}, false
			}
			var err error
			obj, err = c.parseResolved(data)
			if err != nil || obj.GetClassName() != n {
				misses[n] = true
				return callbinding.Class{}, false
			}
		}
		pkg := ""
		if i := strings.LastIndexByte(n, '/'); i >= 0 {
			pkg = strings.ReplaceAll(n[:i], "/", ".")
		}
		samePackage := pkg == c.PackageName
		v := callbinding.Class{Name: n, Public: obj.AccessFlags&1 != 0 || samePackage, MembersComplete: true, ParentsComplete: true, IsInterface: obj.AccessFlags&0x200 != 0}
		if sup := obj.GetSupperClassName(); sup != "" {
			v.Parents = append(v.Parents, sup)
		}
		v.Parents = append(v.Parents, obj.GetInterfacesName()...)
		for _, m := range obj.Methods {
			name, e := obj.getUtf8(m.NameIndex)
			if e != nil {
				v.MembersComplete = false
				continue
			}
			desc, e := obj.getUtf8(m.DescriptorIndex)
			if e != nil {
				v.MembersComplete = false
				continue
			}
			if name == "<init>" || name == "<clinit>" {
				continue
			}
			x := callbinding.Method{Name: name, Desc: desc, Public: m.AccessFlags&1 != 0 || (samePackage && m.AccessFlags&2 == 0), Static: m.AccessFlags&8 != 0, Varargs: m.AccessFlags&0x80 != 0, Bridge: m.AccessFlags&0x40 != 0}
			for _, a := range m.Attributes {
				if _, ok := a.(*SignatureAttribute); ok {
					x.Generic = true
				}
			}
			v.Methods = append(v.Methods, x)
		}
		cache[n] = v
		return v, true
	}
}
