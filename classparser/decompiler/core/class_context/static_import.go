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
	bound, known := f.staticTypeOwner(owner)
	if known {
		return bound + "."
	}
	if f.staticOwnerTypeSyntax(owner, bound) {
		return "((" + bound + ")null)."
	}
	return f.staticImportCallPrefix(owner, member, descriptor, bound, false)
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
		return fail("incomplete, mismatched or inaccessible original owner")
	}
	matches := 0
	for _, method := range declaration.Methods {
		if method.Name == member && method.Desc == descriptor && method.Public && method.Static && !method.Bridge {
			matches++
		}
	}
	if matches != 1 || !f.staticImportScopeClear(member) {
		return fail("method namespace or original static target unproved")
	}
	pkg, cls := SplitPackageClassName(owner)
	if pkg == "" || isAnonymousOrLocalBinaryName(cls) {
		return fail("owner has no importable qualified source name")
	}
	if f.nestedTypeShouldDot(pkg, cls) {
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
