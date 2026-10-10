package javaclassparser

import "strings"

// Hoisting a blank final primitive/String store creates a JLS constant variable:
// javac then replaces GETFIELD/GETSTATIC uses and may suppress initialization.
// Original ConstantValue evidence, rather than the assigned literal, proves that
// this source-level constant status existed. Ordinary constructors and class
// initializers can retain their stores, so they must not invent that status.
func (c *ClassObjectDumper) originalFieldInitializerStatuses() map[string]bool {
	if c.originalInitializerStatusReady {
		return c.originalInitializerStatus
	}
	c.originalInitializerStatusReady = true
	if c.Work != nil && c.Work.CheckAlloc(int64(len(c.obj.Fields))*32) != nil {
		return nil
	}
	result := map[string]bool{}
	for _, field := range c.obj.Fields {
		if !nativeProofWork(c.Work, 1) {
			return nil
		}
		if field == nil {
			continue
		}
		name, ok := sourceBridgeUTF8(c.obj, field.NameIndex)
		desc, dok := sourceBridgeUTF8(c.obj, field.DescriptorIndex)
		if !ok || !dok {
			continue
		}
		constantCapable := len(desc) == 1 && strings.Contains("ZBCSIJFD", desc) || desc == "Ljava/lang/String;"
		allowed := !constantCapable || c.isInterfaceLike()
		for _, a := range field.Attributes {
			if _, ok := a.(*ConstantValueAttribute); ok {
				allowed = true
			}
		}
		result[name] = allowed
	}
	c.originalInitializerStatus = result
	return result
}
