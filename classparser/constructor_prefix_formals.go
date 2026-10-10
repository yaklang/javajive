package javaclassparser

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A static carrier has its own formal scope. Re-declare the complete original
// class scope and explicitly instantiate it at the call site. The first
// delegation operand must be an unchanged constructor parameter: returning its
// original source type preserves its value and adds no erased payload check.
// Method-formal shadowing, injected scopes and competing constructors need a
// different binding proof and are deliberately not certified here.
func (c *ClassObjectDumper) constructorPrefixFormalBinding(p *constructorSourceBoundary, method *MemberInfo) (string, []string, []types.JavaType, types.JavaType, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || method == nil || p == nil || p.delegate == nil || len(p.delegate.Arguments) == 0 || c.FuncCtx.InvocationMetadata == nil {
		return "", nil, nil, nil, false
	}
	ctx := c.FuncCtx
	classSig, seen := "", false
	for _, attr := range c.obj.Attributes {
		if sig, ok := attr.(*SignatureAttribute); ok {
			if seen {
				return "", nil, nil, nil, false
			}
			seen = true
			var valid bool
			classSig, valid = sourceBridgeUTF8(c.obj, sig.SignatureIndex)
			if !valid {
				return "", nil, nil, nil, false
			}
		}
	}
	if classSig == "" || classSig != ctx.ClassSig {
		return "", nil, nil, nil, false
	}
	names, erasures, ok := constructorFormalScope(classSig)
	if !ok || len(names) == 0 || len(names) != len(ctx.ClassTypeParams) || len(names) != len(ctx.TypeParams) {
		return "", nil, nil, nil, false
	}
	for i, name := range names {
		if ctx.ClassTypeParams[i] != name || ctx.TypeParams[i] != name {
			return "", nil, nil, nil, false
		}
	}
	parent, interfaces := types.ParseClassSignatureSupers(classSig)
	rawParent, known := sourceBridgeClassName(c.obj, c.obj.SuperClass)
	if erased, ok := constructorFormalTypeErasure(parent, erasures, 128); !known || !ok || erased != "L"+rawParent+";" || len(interfaces) != len(c.obj.Interfaces) {
		return "", nil, nil, nil, false
	}
	for i, iface := range interfaces {
		raw, known := sourceBridgeClassName(c.obj, c.obj.Interfaces[i])
		if erased, ok := constructorFormalTypeErasure(iface, erasures, 128); !known || !ok || erased != "L"+raw+";" {
			return "", nil, nil, nil, false
		}
	}
	sig, seen := "", false
	for _, attr := range method.Attributes {
		if a, ok := attr.(*SignatureAttribute); ok {
			if seen {
				return "", nil, nil, nil, false
			}
			seen = true
			var valid bool
			sig, valid = sourceBridgeUTF8(c.obj, a.SignatureIndex)
			if !valid {
				return "", nil, nil, nil, false
			}
		}
	}
	if sig == "" || !constructorClosedMethodSignature(sig, erasures) {
		return "", nil, nil, nil, false
	}
	_, params, result := types.ParseMethodSignatureFull(sig, ctx)
	desc, valid := sourceBridgeUTF8(c.obj, method.DescriptorIndex)
	if !valid {
		return "", nil, nil, nil, false
	}
	raw, ret, e := callbinding.Descriptor(desc)
	if e != nil || ret != "V" || result == nil || result.String(ctx) != "void" || len(raw) != len(params) || len(params) != len(p.params) {
		return "", nil, nil, nil, false
	}
	first, ok := values.UnpackSoltValue(p.delegate.Arguments[0]).(*values.JavaRef)
	if !ok || first == nil || first.Id == nil || first.CustomValue != nil || first.StackVar != nil || first.IsThis {
		return "", nil, nil, nil, false
	}
	firstIndex := -1
	for i, param := range p.params {
		if param == nil || param.Type() == nil || params[i] == nil || !reflect.DeepEqual(param.Type().RawType(), params[i].RawType()) {
			return "", nil, nil, nil, false
		}
		if d, ok := constructorFormalTypeErasure(params[i], erasures, 128); !ok || d != raw[i] {
			return "", nil, nil, nil, false
		}
		if param == first {
			firstIndex = i
		}
	}
	if firstIndex < 0 {
		return "", nil, nil, nil, false
	}
	// A narrower helper result must not select another constructor overload.
	// The complete original family proves that this source call has one target.
	owner, known := ctx.InvocationMetadata(strings.ReplaceAll(p.delegate.ClassName, ".", "/"))
	targetParams, _, e := callbinding.Descriptor(p.delegate.Descriptor)
	if !known || !owner.MembersComplete || owner.Name != strings.ReplaceAll(p.delegate.ClassName, ".", "/") || e != nil {
		return "", nil, nil, nil, false
	}
	matches := 0
	for _, m := range owner.Methods {
		if m.Name != "<init>" {
			continue
		}
		ps, _, e := callbinding.Descriptor(m.Desc)
		if e != nil || m.Varargs {
			return "", nil, nil, nil, false
		}
		if m.Desc == p.delegate.Descriptor {
			matches++
		} else if len(ps) == len(targetParams) {
			return "", nil, nil, nil, false
		}
	}
	if matches != 1 {
		return "", nil, nil, nil, false
	}
	header := types.ParseClassSignature(classSig, ctx)
	if header == "" {
		return "", nil, nil, nil, false
	}
	return header + " ", names, params, params[firstIndex], true
}

// Bound erasures must be explicit, complete and closed. Unlike the permissive
// legacy parser, a missing/dependent first bound never defaults to Object.
// Original formal names are retained only when they are legal source names.
func constructorFormalScope(sig string) ([]string, map[string]string, bool) {
	cursor := constructorSignatureCursor{source: sig, remaining: 4096}
	if !cursor.take('<') {
		return nil, nil, false
	}
	var names []string
	scope := map[string]string{}
	for !cursor.at('>') {
		start := cursor.pos
		for cursor.pos < len(sig) && sig[cursor.pos] != ':' {
			cursor.pos++
		}
		if cursor.pos == len(sig) {
			return nil, nil, false
		}
		name := sig[start:cursor.pos]
		if !constructorFormalIdentifier(name) || scope[name] != "" || len(names) >= 32 || !cursor.take(':') {
			return nil, nil, false
		}
		first := ""
		// A missing class bound is allowed only with a real first interface bound.
		if !cursor.at(':') {
			if !cursor.at('L') {
				return nil, nil, false
			}
			var ok bool
			first, ok = cursor.field(false, 0)
			if !ok {
				return nil, nil, false
			}
		}
		for cursor.take(':') {
			if !cursor.at('L') {
				return nil, nil, false
			}
			bound, ok := cursor.field(false, 0)
			if !ok {
				return nil, nil, false
			}
			if first == "" {
				first = bound
			}
		}
		if first == "" {
			return nil, nil, false
		}
		names = append(names, name)
		scope[name] = strings.ReplaceAll(first[1:len(first)-1], "/", ".")
	}
	if !cursor.take('>') || len(names) == 0 || !cursor.at('L') {
		return nil, nil, false
	}
	for cursor.pos < len(sig) {
		if !cursor.at('L') {
			return nil, nil, false
		}
		if _, ok := cursor.field(false, 0); !ok {
			return nil, nil, false
		}
	}
	if !cursor.closed(scope) {
		return nil, nil, false
	}
	return names, scope, true
}

// Validate the full JVM grammar before asking the legacy renderer to print it.
// The renderer intentionally accepts partial signatures; proof code must not.
// Bounds containing foreign variables, trailing garbage, deep or cyclic input,
// and illegal primitive/wildcard positions cannot establish a formal scope.
type constructorSignatureCursor struct {
	source         string
	pos, remaining int
	refs           []string
}

func (p *constructorSignatureCursor) at(ch byte) bool {
	return p.pos < len(p.source) && p.source[p.pos] == ch
}
func (p *constructorSignatureCursor) take(ch byte) bool {
	if !p.at(ch) {
		return false
	}
	p.pos++
	return true
}
func (p *constructorSignatureCursor) closed(scope map[string]string) bool {
	if p.remaining < 0 || p.pos != len(p.source) {
		return false
	}
	for _, name := range p.refs {
		if scope[name] == "" {
			return false
		}
	}
	return true
}
func (p *constructorSignatureCursor) field(primitive bool, depth int) (string, bool) {
	p.remaining--
	if p.remaining < 0 || len(p.source) > 4096 || depth > 32 || p.pos >= len(p.source) {
		return "", false
	}
	ch := p.source[p.pos]
	p.pos++
	switch ch {
	case 'T':
		start := p.pos
		for p.pos < len(p.source) && p.source[p.pos] != ';' {
			p.pos++
		}
		name := p.source[start:p.pos]
		if !constructorFormalIdentifier(name) || !p.take(';') {
			return "", false
		}
		p.refs = append(p.refs, name)
		return "T" + name + ";", true
	case '[':
		component, ok := p.field(true, depth+1)
		return "[" + component, ok
	case 'L':
		raw := ""
		for {
			start := p.pos
			for p.pos < len(p.source) && !strings.ContainsRune("<.;", rune(p.source[p.pos])) {
				p.pos++
			}
			name := p.source[start:p.pos]
			if name == "" {
				return "", false
			}
			for _, part := range strings.Split(name, "/") {
				if !constructorFormalIdentifier(part) {
					return "", false
				}
			}
			raw += name
			if p.take('<') {
				count := 0
				for !p.at('>') {
					count++
					if p.take('*') {
						continue
					}
					if !p.take('+') {
						p.take('-')
					}
					if _, ok := p.field(false, depth+1); !ok {
						return "", false
					}
				}
				if count == 0 || !p.take('>') {
					return "", false
				}
			}
			if p.take(';') {
				return "L" + raw + ";", true
			}
			if !p.take('.') {
				return "", false
			}
			raw += "$"
		}
	default:
		if primitive && strings.ContainsRune("BCDFIJSZ", rune(ch)) {
			return string(ch), true
		}
	}
	return "", false
}
func constructorClosedMethodSignature(sig string, scope map[string]string) bool {
	p := constructorSignatureCursor{source: sig, remaining: 4096}
	if !p.take('(') {
		return false
	}
	for !p.at(')') {
		if _, ok := p.field(true, 0); !ok {
			return false
		}
	}
	if !p.take(')') || !p.take('V') {
		return false
	}
	for p.take('^') {
		if !p.at('L') && !p.at('T') {
			return false
		}
		if _, ok := p.field(false, 0); !ok {
			return false
		}
	}
	return p.closed(scope)
}

func constructorFormalIdentifier(name string) bool {
	// The source renderer's identifier mapping must not rename a formal while
	// leaving its references unchanged. It also rejects source keyword names.
	return class_context.SafeIdentifier(name) == name
}

func constructorFormalTypeErasure(t types.JavaType, scope map[string]string, remaining int) (string, bool) {
	if t == nil || remaining <= 0 {
		return "", false
	}
	switch raw := t.RawType().(type) {
	case *types.JavaPrimer:
		// Primitive descriptors are canonical and cannot be type-variable names.
		switch t.String(nil) {
		case "boolean":
			return "Z", true
		case "byte":
			return "B", true
		case "char":
			return "C", true
		case "short":
			return "S", true
		case "int":
			return "I", true
		case "long":
			return "J", true
		case "float":
			return "F", true
		case "double":
			return "D", true
		}
	case *types.JavaArrayType:
		element, ok := constructorFormalTypeErasure(raw.JavaType, scope, remaining-1)
		if ok && raw.Dimension > 0 && raw.Dimension <= 255 {
			return strings.Repeat("[", raw.Dimension) + element, true
		}
	case *types.JavaClass:
		name := raw.Name
		if bound, ok := scope[name]; ok {
			name = bound
		} else if !strings.ContainsAny(name, "./") {
			return "", false
		}
		return "L" + strings.ReplaceAll(name, ".", "/") + ";", true
	case *types.JavaParameterizedType:
		if raw.RawClassName == "" {
			return "", false
		}
		return "L" + strings.ReplaceAll(raw.RawClassName, ".", "/") + ";", true
	}
	return "", false
}
