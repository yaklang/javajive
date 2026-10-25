package class_context

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// StaticMethodImports is a compilation-unit transaction. Importing two owners
// of one method name would change every earlier unqualified call; retain the
// conflict and reject the whole unit instead of accepting a partial import set.
type StaticMethodImports struct {
	owners        map[string]string
	err           error
	failedMethod  string
	retainedBytes int64
}

func NewStaticMethodImports() *StaticMethodImports {
	return &StaticMethodImports{owners: map[string]string{}}
}

func (s *StaticMethodImports) Clone() *StaticMethodImports {
	if s == nil {
		return nil
	}
	c := NewStaticMethodImports()
	c.err = s.err
	c.failedMethod = s.failedMethod
	c.retainedBytes = s.retainedBytes
	for name, owner := range s.owners {
		c.owners[name] = owner
	}
	return c
}

func (s *StaticMethodImports) Error() error {
	if s == nil {
		return nil
	}
	return s.err
}

func (s *StaticMethodImports) FailedMethod() string {
	if s == nil {
		return ""
	}
	return s.failedMethod
}

func (s *StaticMethodImports) Imports() []string {
	if s == nil {
		return nil
	}
	imports := make([]string, 0, len(s.owners))
	for name, owner := range s.owners {
		imports = append(imports, owner+"."+name)
	}
	slices.Sort(imports)
	return imports
}

// StaticInterfaceCallPrefix returns either a proved TypeName plus '.', or an
// empty prefix backed by a single static import. Static-import lookup is a
// method-namespace operation: all lexical/ancestor declarations must be known
// and have no same-spelled method (even one of a different arity). Values and
// local/formal type names do not participate in import declarations.
func (f *ClassContext) StaticInterfaceCallPrefix(owner, member, descriptor string) string {
	if f.packageRootIsType(owner) && !f.staticImportScopeClear(member) {
		if spelling := f.importedTypeCallPrefix(owner, member, descriptor, true); spelling != "" {
			return spelling
		}
	}
	bound, known := f.staticTypeOwner(owner)
	if known {
		return bound + "."
	}
	return f.staticImportCallPrefix(owner, member, descriptor, bound, true)
}

// A class-only null primary resolves a value-name collision only if its cast
// type still denotes the original class. If type syntax is also obscured,
// use the same proved method-namespace import as an interface invocation.
func (f *ClassContext) StaticClassCallPrefix(owner, member, descriptor string) string {
	if f.packageRootIsType(owner) && !f.staticImportScopeClear(member) {
		if spelling := f.importedTypeCallPrefix(owner, member, descriptor, false); spelling != "" {
			return spelling
		}
	}
	bound, known := f.staticTypeOwner(owner)
	if known {
		return bound + "."
	}
	if f.staticOwnerTypeSyntax(owner, bound) && !f.packageRootIsType(owner) {
		if f.boundNamesOwner(owner, bound) || f.samePackageClassOwnerCast(owner, bound, member, descriptor) {
			return "((" + bound + ")null)."
		}
	}
	return f.staticImportCallPrefix(owner, member, descriptor, bound, false)
}

// retainedOwnerPrefix keeps the original package path. A keyword segment
// such as `if` is spelled `if_`, matching every other rendered type name.
func retainedOwnerPrefix(owner string) string {
	parts := strings.Split(owner, ".")
	for i, part := range parts {
		parts[i] = SafeIdentifier(part)
	}
	return strings.Join(parts, ".") + "."
}

func (f *ClassContext) failStaticOwner(owner, member, descriptor, reason string) {
	if f.StaticMethodImports == nil {
		f.StaticMethodImports = NewStaticMethodImports()
	}
	if f.StaticMethodImports.err == nil {
		f.StaticMethodImports.failedMethod = f.ClassName + "." + f.FunctionName + f.CurrentMethodDesc
		f.StaticMethodImports.err = fmt.Errorf("source method %s: static owner %s.%s%s cannot be bound: %s", f.StaticMethodImports.failedMethod, owner, member, descriptor, reason)
	}
}

func (f *ClassContext) staticImportCallPrefix(owner, member, descriptor, bound string, interfaceOwner bool) string {
	if f.StaticMethodImports == nil {
		f.StaticMethodImports = NewStaticMethodImports()
	}
	if f.StaticMethodImports.err != nil {
		return bound + "."
	}
	fail := func(reason string) string {
		f.failStaticOwner(owner, member, descriptor, reason)
		return bound + "."
	}
	if f.InvocationMetadata == nil || SafeIdentifier(member) != member || member == "" {
		return fail("missing original declaration or unnameable member")
	}
	// An owned lexical declaration may be private or require a source binder.
	// Its qualified importability needs a separate certificate.
	if f.DeclarationSourceName != nil {
		if _, owned := f.DeclarationSourceName(owner); owned {
			return fail("lexical declaration importability unknown")
		}
	}
	internal := strings.ReplaceAll(owner, ".", "/")
	declaration, exists := f.InvocationMetadata(internal)
	if !exists || declaration.Name != internal || declaration.IsInterface != interfaceOwner || !declaration.Public || !declaration.MembersComplete || !declaration.ParentsComplete {
		// The invoke already names this owner. A missing JDK table is not a
		// different target. Analysis output keeps the qualified call; the default
		// path still refuses an unproved owner.
		if f.RetainUnprovedSource && owner != "" && SafeIdentifier(member) == member {
			return retainedOwnerPrefix(owner)
		}
		return fail("incomplete, mismatched or inaccessible original owner")
	}
	matches := 0
	for _, method := range declaration.Methods {
		if method.Name == member && method.Desc == descriptor && method.Public && method.Static && !method.Bridge {
			matches++
		}
	}
	if matches != 1 || !f.staticImportScopeClear(member) {
		// The method name is already a member here, so a static import is
		// ambiguous (JLS 6.4.1 / 15.12). When the owner's package root is a type
		// in scope, the qualified name is also illegal (JLS 6.5.5.1). The single
		// type import of that unique owner is the remaining source spelling:
		// `Owner.member(...)`, which does not reclassify the method name.
		if spelling := f.importedTypeCallPrefix(owner, member, descriptor, interfaceOwner); spelling != "" {
			return spelling
		}
		// The invoke names one public static method, but neither a static import
		// nor a single-type import can spell it. Analysis output keeps that owner.
		// Keyword segments use the same SafeIdentifier spelling as other types.
		// The default path still refuses instead of choosing another method.
		if f.RetainUnprovedSource && matches == 1 {
			return retainedOwnerPrefix(owner)
		}
		return fail("method namespace or original static target unproved")
	}
	pkg, cls := SplitPackageClassName(owner)
	if pkg == "" || isAnonymousOrLocalBinaryName(cls) {
		return fail("owner has no importable qualified source name")
	}
	// The dotting policy describes how an existing binary nesting separator
	// is rendered. It also returns true for top-level external/platform types;
	// that answer must not turn a non-nested owner into an invalid conversion.
	if strings.Contains(cls, "$") && f.nestedTypeShouldDot(pkg, cls) {
		var valid bool
		cls, valid = binaryNestedNameToSource(cls)
		if !valid {
			return fail("nested source identity unknown")
		}
	}
	qualified := pkg + "." + cls
	for _, word := range strings.Split(qualified, ".") {
		if word == "" || SafeIdentifier(word) != word {
			return fail("owner source spelling changed")
		}
	}
	if prior := f.StaticMethodImports.owners[member]; prior != "" {
		if prior != qualified {
			return fail("different owners require the same imported method name")
		}
		return ""
	}
	if len(f.StaticMethodImports.owners) >= 512 && f.StaticMethodImports.owners[member] == "" {
		return fail("static import inventory limit")
	}
	retained := f.StaticMethodImports.retainedBytes + int64(len(member)+len(qualified)) + 256
	if f.Work != nil && f.Work.CheckAlloc(retained) != nil {
		return fail("static import budget")
	}
	f.StaticMethodImports.owners[member] = qualified
	f.StaticMethodImports.retainedBytes = retained
	return ""
}

// samePackageClassOwnerCast proves a cast primary for an exact same-package
// top-level class whose simple name is shadowed by a value. Cast type syntax
// is resolved in the type namespace. Require an exact original declaration,
// complete members, one matching static target, no private/source-inaccessible
// target, and no type parameter, lexical type, or imported type taking the
// simple name.
func (f *ClassContext) samePackageClassOwnerCast(owner, bound, member, descriptor string) bool {
	if f == nil || owner == "" || member == "" || bound == "" {
		return false
	}
	pkg, cls := SplitPackageClassName(owner)
	internal := strings.ReplaceAll(owner, ".", "/")
	if pkg != f.PackageName || cls == "" || cls != bound || f.IsTypeParam(bound) || f.LexicalTypeNames[bound] || f.KeySet != nil && f.KeySet.Has(bound) {
		return false
	}
	if f.InvocationMetadata == nil {
		return false
	}
	declaration, ok := f.InvocationMetadata(internal)
	if !ok || declaration.Name != internal || declaration.IsInterface || !declaration.MembersComplete {
		return false
	}
	matches := 0
	for _, target := range declaration.Methods {
		if target.Name != member || target.Desc != descriptor || !target.Static || target.Bridge {
			continue
		}
		if target.SourceInaccessible || target.AccessFlagsKnown && target.AccessFlags&0x0002 != 0 {
			return false
		}
		matches++
	}
	return matches == 1
}

// boundNamesOwner reports that a cast type still denotes this owner. A
// simple name does so only after that owner took the single-type import;
// otherwise the cast names whatever else is in scope, or nothing.
func (f *ClassContext) packageRootIsType(owner string) bool {
	pkg, _ := SplitPackageClassName(owner)
	if pkg == "" {
		return false
	}
	root := strings.SplitN(pkg, ".", 2)[0]
	return f.LexicalTypeNames[root] || f.IsTypeParam(root) || f.typeNameShadowsPackageRoot(root)
}

func (f *ClassContext) boundNamesOwner(owner, bound string) bool {
	if strings.Contains(bound, ".") {
		return true
	}
	_, cls := SplitPackageClassName(owner)
	return cls == bound && f.KeySet != nil && f.KeySet.Has(cls)
}

func (f *ClassContext) importedTypeCallPrefix(owner, member, descriptor string, interfaceOwner bool) string {
	if f == nil || f.staticImportScopeClear(member) {
		return ""
	}
	pkg, cls := SplitPackageClassName(owner)
	if pkg == "" || cls == "" || isAnonymousOrLocalBinaryName(cls) || strings.Contains(cls, "$") {
		return ""
	}
	// A value obscuring the package root still leaves the qualified type
	// name legal, so a conflicting method keeps the static-import refusal.
	// A type in that position (lexical, a type parameter, or a same-package
	// class such as a.cn) removes the qualified spelling too.
	if !f.packageRootIsType(owner) {
		return ""
	}
	if f.typeImportOwner(cls) != "" && f.typeImportOwner(cls) != owner {
		return ""
	}
	internal := strings.ReplaceAll(owner, ".", "/")
	declaration, exists := f.InvocationMetadata(internal)
	if !exists || declaration.Name != internal || declaration.IsInterface != interfaceOwner || !declaration.Public || !declaration.MembersComplete || !declaration.ParentsComplete {
		return ""
	}
	matches := 0
	for _, method := range declaration.Methods {
		if method.Name == member && method.Desc == descriptor && method.Public && method.Static && !method.Bridge {
			matches++
		}
	}
	if matches != 1 || SafeIdentifier(cls) != cls {
		return ""
	}
	before := f.KeySet != nil && f.KeySet.Has(cls)
	f.Import(owner)
	if f.KeySet == nil || !f.KeySet.Has(cls) {
		return ""
	}
	// The simple name was already taken. Reuse it only when this package is
	// the one that registered it; another type of the same simple name must
	// not receive its calls.
	if before && !f.importedFrom(pkg, cls) {
		return ""
	}
	// ShortTypeName records a qualified-name refusal before this import
	// exists. That refusal names the caller method, not the owner, and it is
	// the only error this prefix computation can have stored so far.
	if f.StaticMethodImports != nil && f.StaticMethodImports.err != nil && f.StaticMethodImports.failedMethod == f.ClassName+"."+f.FunctionName+f.CurrentMethodDesc {
		f.StaticMethodImports.err = nil
		f.StaticMethodImports.failedMethod = ""
	}
	return cls + "."
}

func (f *ClassContext) importedFrom(pkg, cls string) bool {
	if f == nil || f.BuildInLibsMap == nil {
		return false
	}
	libs := f.BuildInLibsMap.GetMust(pkg)
	return slices.Contains(libs, cls) || slices.Contains(libs, "*")
}

func (f *ClassContext) staticImportScopeClear(member string) bool {
	if answer, found := f.staticImportScopeMemo[member]; found {
		return answer
	}
	clear := true
	lexical := map[*ClassContext]bool{}
	for scope := f; scope != nil && clear; scope = scope.SourceLexicalParent {
		if lexical[scope] || len(lexical) >= 256 || scope.InvocationMetadata == nil || scope.ClassName == "" {
			clear = false
			break
		}
		lexical[scope] = true
		active, done := map[string]bool{}, map[string]bool{}
		var walk func(string) bool
		walk = func(owner string) bool {
			if active[owner] {
				return false
			}
			if done[owner] {
				return true
			}
			if len(done)+len(active) >= 256 {
				return false
			}
			declaration, known := scope.InvocationMetadata(owner)
			if !known || declaration.Name != owner || !declaration.MembersComplete || !declaration.ParentsComplete {
				return false
			}
			if f.Work != nil {
				if f.Work.CheckAlloc(int64(len(done)+len(active)+len(lexical)+len(declaration.Methods)+len(declaration.Parents)+1)*128) != nil || f.Work.Charge(workbudget.CounterGraphScans, int64(1+len(declaration.Methods)+len(declaration.Parents))) != nil {
					return false
				}
			}
			for _, method := range declaration.Methods {
				if SafeIdentifier(method.Name) == member {
					return false
				}
			}
			active[owner] = true
			for _, parent := range declaration.Parents {
				if !walk(parent) {
					return false
				}
			}
			delete(active, owner)
			done[owner] = true
			return true
		}
		clear = walk(strings.ReplaceAll(scope.ClassName, ".", "/"))
	}
	if f.staticImportScopeMemo == nil {
		f.staticImportScopeMemo = map[string]bool{}
	}
	f.staticImportScopeMemo[member] = clear
	return clear
}
