package javaclassparser

import "strings"

// Dependency bytes prove declarations but do not enroll a class in the output
// family. Recover named nesting from InnerClasses, including literal dollar
// signs in valid identifiers. Neither package equality nor '$' alone proves
// that an external class will be emitted as a flattened sibling.
func (c *ClassObjectDumper) buildDeclarationSourceNames() func(string) (string, bool) {
	if c.declarationResolver == nil {
		return nil
	}
	type result struct {
		name  string
		known bool
	}
	cache := map[string]result{}
	return func(binary string) (string, bool) {
		binary = strings.ReplaceAll(binary, ".", "/")
		if !strings.Contains(binary, "$") {
			return "", false
		}
		if r, ok := cache[binary]; ok {
			return r.name, r.known
		}
		cache[binary] = result{}
		if c.foldSiblingResolver != nil {
			if raw, ok := c.foldSiblingResolver(binary); ok && len(raw) > 0 {
				return "", false
			}
		}
		if c.archiveDeclarationResolver != nil {
			if raw, known := c.archiveDeclarationResolver(binary); known && len(raw) > 0 {
				return "", false
			}
		}
		seen := map[string]bool{}
		name := binary
		parts := []string{}
		for len(seen) < 32 {
			if seen[name] {
				return "", false
			}
			seen[name] = true
			raw, ok := c.declarationResolver(name)
			if !ok {
				return "", false
			}
			obj, err := c.parseResolved(raw)
			if err != nil || obj.GetClassName() != name {
				return "", false
			}
			outer, inner := "", ""
			matches := 0
			className := func(index uint16) string {
				constant, err := obj.getConstantInfo(index)
				if err != nil {
					return ""
				}
				cls, ok := constant.(*ConstantClassInfo)
				if !ok {
					return ""
				}
				n, err := obj.getUtf8(cls.NameIndex)
				if err != nil {
					return ""
				}
				return n
			}
			for _, a := range obj.Attributes {
				if table, ok := a.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						if row == nil || className(row.InnerClassInfoIndex) != name {
							continue
						}
						matches++
						outer = className(row.OuterClassInfoIndex)
						inner, _ = obj.getUtf8(row.InnerNameIndex)
					}
				}
			}
			if matches == 0 {
				if len(parts) == 0 {
					return "", false
				}
				source := strings.ReplaceAll(name, "/", ".")
				for i := len(parts) - 1; i >= 0; i-- {
					source += "." + parts[i]
				}
				cache[binary] = result{source, true}
				return source, true
			}
			if matches != 1 || outer == "" || inner == "" || name != outer+"$"+inner {
				return "", false
			}
			parts = append(parts, inner)
			name = outer
		}
		return "", false
	}
}
