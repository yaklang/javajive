package class_context

import "strings"

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
	bound := f.ShortTypeName(name)
	if !f.valueNameShadows(strings.SplitN(bound, ".", 2)[0]) {
		return bound
	}
	pkg, _ := SplitPackageClassName(name)
	if pkg != "" {
		qualified := bound
		if !strings.HasPrefix(bound, pkg+".") {
			qualified = pkg + "." + bound
		}
		root := strings.SplitN(qualified, ".", 2)[0]
		if !f.valueNameShadows(root) && !f.typeNameShadowsPackageRoot(root) {
			return qualified
		}
	}
	if classOwner {
		return "((" + bound + ")null)"
	}
	// An interface static method cannot use a value primary. Keep that
	// restriction; never turn it into a null invocation on an interface.
	return bound
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
