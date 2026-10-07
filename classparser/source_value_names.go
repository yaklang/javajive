package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Source fields occupy the value namespace even when no get/put instruction
// references them. Retain declarations across known ancestors and enclosing
// scopes; a private ancestor field can conservatively cause one harmless cast.
// Unknown declaration bytes supply no positive field-name witness.
func (c *ClassObjectDumper) buildSourceValueNameShadow() func(string) bool {
	if c == nil || c.obj == nil {
		return nil
	}
	names := map[string]bool{}
	ready := false
	outer := c.nativeOuterContext
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	return func(name string) bool {
		if outer != nil && outer.SourceValueNameShadow != nil && outer.SourceValueNameShadow(name) {
			return true
		}
		if !ready {
			ready = true
			seen := map[string]bool{}
			pending := []*ClassObject{c.obj}
			for len(pending) > 0 {
				obj := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				owner := obj.GetClassName()
				if seen[owner] {
					continue
				}
				if c.Work != nil {
					if c.Work.CheckAlloc(int64(len(seen)+len(names)+len(pending)+len(obj.Fields)+len(obj.Interfaces)+2)*128) != nil || c.Work.Charge(workbudget.CounterGraphScans, int64(1+len(obj.Fields)+len(obj.Interfaces))) != nil {
						return false
					}
				}
				seen[owner] = true
				for _, field := range obj.Fields {
					if text, err := obj.getUtf8(field.NameIndex); err == nil {
						names[class_context.SafeIdentifier(text)] = true
					}
				}
				parents := append([]string{obj.GetSupperClassName()}, obj.GetInterfacesName()...)
				for _, parent := range parents {
					if parent == "" || seen[parent] {
						continue
					}
					var raw []byte
					var ok bool
					if c.foldSiblingResolver != nil {
						raw, ok = c.foldSiblingResolver(parent)
					}
					if !ok && c.archiveDeclarationResolver != nil {
						raw, ok = c.archiveDeclarationResolver(parent)
					}
					if !ok && c.declarationResolver != nil {
						raw, ok = c.declarationResolver(parent)
					}
					if !ok {
						raw, ok = jdkConstructorClassBytes(parent, target)
					}
					if !ok {
						continue
					}
					parsed, err := c.parseResolved(raw)
					if err == nil && parsed.GetClassName() == parent {
						pending = append(pending, parsed)
					}
				}
			}
		}
		return names[name]
	}
}
