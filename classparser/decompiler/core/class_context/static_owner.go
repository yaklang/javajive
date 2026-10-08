package class_context

import "strings"

// A field select reclassifies its leading name in the same way as a static
// method select. When a value obscures the type, an exact null primary keeps
// the type namespace without evaluating a receiver or adding a null check
// (JLS 15.11.1). Unlike static interface methods, interface fields allow this
// primary form too. An implicit own-field name still needs a free value name.
func (f *ClassContext) StaticFieldSelection(owner, member string) string {
	member = SafeIdentifier(member)
	if owner == f.ClassName && !f.QualifiedStaticFields && !f.localValueNameShadows(member) {
		return member
	}
	return f.staticOwner(owner, true) + "." + member
}

// StaticClassOwner retains type binding when a method select would otherwise
// reclassify the first identifier as a value name (JLS 6.5.2). A null primary of
// the exact class type contributes no effects and is discarded for invokestatic
// (JLS 15.12.4.1); it is never read as a receiver or null-checked. Interface static
// calls require a TypeName and must not use this class-only rendering path.
func (f *ClassContext) StaticClassOwner(name string) string {
	return f.staticOwner(name, true)
}

func (f *ClassContext) StaticInterfaceOwner(name string) string {
	return f.staticOwner(name, false)
}

func (f *ClassContext) staticOwner(name string, classOwner bool) string {
	bound, known := f.staticTypeOwner(name)
	if known {
		return bound
	}
	if classOwner {
		if !f.staticOwnerTypeSyntax(name, bound) {
			f.failStaticOwner(name, "", "", "class type is obscured in both simple and qualified source syntax")
			return bound
		}
		return "((" + bound + ")null)"
	}
	return bound
}

// A method select reclassifies its first identifier as a value. Type syntax
// alone therefore does not certify an interface's static invocation owner.
func (f *ClassContext) staticTypeOwner(name string) (string, bool) {
	bound := f.ShortTypeName(name)
	if f.staticOwnerTypeSyntax(name, bound) && !f.valueNameShadows(strings.SplitN(bound, ".", 2)[0]) {
		return bound, true
	}
	pkg, _ := SplitPackageClassName(name)
	if pkg != "" {
		qualified := bound
		if !strings.HasPrefix(bound, pkg+".") {
			qualified = pkg + "." + bound
		}
		root := strings.SplitN(qualified, ".", 2)[0]
		if f.staticOwnerTypeSyntax(name, qualified) && !f.valueNameShadows(root) {
			return qualified, true
		}
	}
	return bound, false
}

// A cast ignores value names, but cannot escape a type parameter or a type
// obscuring the package root. An owned Outer.Member spelling is a type path,
// not a package path; only the original owner's package prefix needs the
// package-root check.
func (f *ClassContext) staticOwnerTypeSyntax(owner, source string) bool {
	root := strings.SplitN(source, ".", 2)[0]
	if f.IsTypeParam(root) {
		return false
	}
	pkg, _ := SplitPackageClassName(owner)
	return pkg == "" || !strings.HasPrefix(source, pkg+".") || !f.typeNameShadowsPackageRoot(root)
}

func (f *ClassContext) typeNameShadowsPackageRoot(name string) bool {
	if f.LexicalTypeNames[name] || f.IsTypeParam(name) {
		return true
	}
	own := name
	if f.PackageName != "" {
		own = strings.ReplaceAll(f.PackageName, ".", "/") + "/" + name
	}
	if f.InvocationMetadata != nil {
		declaration, known := f.InvocationMetadata(own)
		return known && declaration.Name == own
	}
	_, current := SplitPackageClassName(f.ClassName)
	return current == name
}

func (f *ClassContext) valueNameShadows(name string) bool {
	if f.SourceValueNameShadow != nil && f.SourceValueNameShadow(name) {
		return true
	}
	return f.localValueNameShadows(name)
}

func (f *ClassContext) localValueNameShadows(name string) bool {
	for _, arg := range f.Arguments {
		if arg == name {
			return true
		}
	}
	for _, local := range f.LocalNames {
		if local == name {
			return true
		}
	}
	return false
}
