package class_context

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/internal/funk"
	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/omap"
	"github.com/yaklang/javajive/internal/utils"
)

type ClassContext struct {
	ClassName       string
	FunctionName    string
	SupperClassName string
	FunctionType    any
	PackageName     string
	BuildInLibsMap  *omap.OrderedMap[string, []string]
	KeySet          *utils.Set[string]
	Arguments       []string
	IsStatic        bool
	IsVarArgs       bool
	// TypeParams holds the bare names of the type variables in scope for the class being
	// rendered (its formal type parameters, plus any free variables injected on a flattened
	// inner class). It lets renderers tell a type-variable reference (e.g. `T`/`K`/`V`) apart
	// from an ordinary bare-named class so they can, for instance, emit an unchecked cast when
	// a value erased to a bound is returned from a now-type-variable-typed method.
	TypeParams []string
	// ClassTypeParams holds ONLY the class-scope type variables (the class's formal type parameters
	// plus any free variables injected on a flattened inner class) -- it is the CLASS-only subset of
	// TypeParams and, unlike TypeParams, is NEVER extended with a method's own `<T>` parameters while
	// that method is rendered. It lets a renderer recover `this`'s actual parameterization
	// (`C<ClassTypeParams>`) so it can tell the `cast()` reparameterization idiom
	// (`<N1 extends N> C<N1> cast() { return this; }`, where `this` is `C<N>` not `C<N1>`) apart from
	// an identity `return this` (`C<K,V> in class C<K,V>`).
	ClassTypeParams []string
	// FieldTypeVars maps a (same-class) field's safe identifier to the bare class-scope type
	// variable it is declared as (e.g. `key` -> `K` for `private final K key;`). It is recovered
	// from each field's generic Signature (`TK;`) once per class. A store into such a field whose
	// right-hand side erased to Object/the bound needs an explicit unchecked `(K)` cast to
	// recompile (bytecode erases the field type to its bound, so `this.key = objExpr` otherwise
	// fails "Object cannot be converted to K"). Empty when the class has no type variables.
	FieldTypeVars map[string]string
	// FieldSignatures maps a (same-class) field's safe identifier to the RAW generic Signature
	// string of a PARAMETERIZED field type (e.g. `function` ->
	// `Ljava/util/function/BiConsumer<TT;TV;>;`). A field access value (getfield) only carries the
	// ERASED descriptor type (raw `BiConsumer`), so an argument flowing into a JDK generic method on
	// that receiver (`this.function.accept(x, y)`) loses the source's `(T)`/`(V)` cast and javac
	// rejects it. Renderers parse this on demand (types.ParseSignature) to recover the receiver's
	// type args. Stored as a string because class_context must not import the types package (cycle).
	// Only parameterized signatures (containing `<`) are recorded; empty when none.
	FieldSignatures map[string]string
	// MethodSignatures maps a same-class method's (name, arity) key (see MethodSigKey) to its raw
	// generic method Signature string (e.g. `tailSet` -> `(TE;)Ljava/util/SortedSet<TE;>;`). A call
	// `this.tailSet(objVal)` on a same-class generic method loses the source's `(E)` argument cast
	// because the descriptor erases the parameter to its bound (Object); javac then rejects
	// "Object cannot be converted to E". Renderers parse this on demand (types.ParseMethodSignatureFull)
	// to recover the formal parameter type and re-emit the cast -- but only for CLASS-scope type
	// variables, never a method-scope `<T>` (not in scope at the call site). Overloads that collide on
	// (name, arity) are dropped to avoid resolving to the wrong signature. Stored as a string because
	// class_context must not import the types package (cycle). Empty when the class has no such method.
	MethodSignatures map[string]string
	// ConstructorSignatures maps a same-class constructor's argument count to its raw generic Signature
	// string (e.g. 1 -> `(Ljava/util/Comparator<-TK;>;)V`). A `this(...)` self-call loses the source's
	// unchecked wildcard cast on an argument whose parameter is a wildcard parameterization mentioning a
	// class type variable (gson LinkedTreeMap `this((Comparator<? super K>) NATURAL_ORDER)`): the bytecode
	// erases both to raw `Comparator`, emits no checkcast, and javac then rejects
	// "Comparator<Comparable> cannot be converted to Comparator<? super K>". It is keyed by ARGUMENT COUNT
	// (not name) and is recorded ONLY when the Signature's parameter count equals the descriptor's -- a
	// non-static inner class's constructor Signature OMITS the synthetic leading this$0/outer-capture
	// parameters, so a count mismatch (the offset case) is skipped to avoid mis-indexing. Overloads
	// colliding on arity are dropped. Stored as a string (class_context must not import the types package).
	// Empty when the class has no such constructor.
	ConstructorSignatures map[int]string
	// ClassSig is the raw generic class Signature string of the class currently being rendered (e.g.
	// `<K:Ljava/lang/Object;V:Ljava/lang/Object;>Lcom/foo/Base<TK;TV;>;`). It seeds the unified
	// cross-class generic resolver (types.ResolveInstantiatedParamType) for a `this` receiver: the walk
	// reads this class's parameterized supertypes from it. Empty when the class is non-generic / has no
	// signature. Stored as a string (class_context must not import the types package).
	ClassSig string
	// RawEraseTypeVars is the set of bare type-variable names that this class REFERENCES but does NOT
	// declare, and which CANNOT be injected onto its declaration. It is populated only for a flattened
	// NON-STATIC inner class that has its OWN formal type parameters (e.g. `Iterator<T>`): such a class
	// also references the enclosing class's variables (`Node<K,V>` fields) but cannot re-declare them
	// without breaking the arity of its `<ownParam>` reference sites (`Iterator<ElementType>`). Rendering
	// `Node<K,V>` verbatim would emit undeclared `K`/`V` (javac "cannot find symbol: class K"); instead
	// any parameterization whose DIRECT argument is one of these names is rendered RAW (`Node<K,V>` ->
	// `Node`) -- legal, runtime-identical, and matching the local already emitted raw. Nil/empty for every
	// ordinary class, so it is a strict no-op there. See dumper (JDEC_INNER_RAW_ERASE_OFF) and
	// types.JavaParameterizedType.String.
	RawEraseTypeVars map[string]bool
	// SiblingClassSig resolves a jar-internal class's generic signature info by binary internal name
	// (slash-separated). It returns the class's raw class Signature and a (name,arity)->method Signature
	// map, or ok=false for JDK/external classes whose bytes are not in the jar. The dumper builds this
	// closure (it owns the byte resolver + parser); the renderer/types packages consume only strings, so
	// the type/class_context packages never import the parser. The field type is intentionally an unnamed
	// func type so its value is directly assignable to types.ClassSigProvider (identical signature)
	// without class_context importing types. Nil when no cross-class resolver is available (single-class
	// decompile); set only on the jar / DecompileWithResolver path.
	SiblingClassSig func(internalName string) (classSig string, methodSigs map[string]string, ok bool)
	// SiblingSuperTypes resolves a jar-internal class's RAW direct supertypes by binary internal name
	// (slash-separated): its super_class internal name followed by its direct interface internal names
	// (each slash-form, "" entries omitted). Unlike SiblingClassSig (which reads the generic Signature
	// attribute and is empty for non-generic classes), this reads the always-present super_class /
	// Interfaces constant-pool entries, so it works for plain classes too (e.g. `Any extends JSONSchema`).
	// It returns ok=false for JDK/external classes whose bytes are not in the jar. Used by the cross-class
	// subtype/LUB resolver (types.CrossClassDirectLUB) to WIDEN a declaration to a jar-internal supertype.
	// The dumper builds this closure (it owns the byte resolver + parser); the type/class_context packages
	// consume only strings. The field type is intentionally an unnamed func type so its value is directly
	// assignable to types.SuperTypeProvider (identical signature) without class_context importing types.
	// Nil when no cross-class resolver is available (single-class decompile); set only on the jar /
	// DecompileWithResolver path.
	SiblingSuperTypes func(internalName string) (supers []string, ok bool)
}

// FieldSignature returns the raw generic Signature string of a same-class parameterized field, or ""
// when name is not a recorded parameterized field (see FieldSignatures).
func (f *ClassContext) FieldSignature(name string) string {
	if f == nil || name == "" || f.FieldSignatures == nil {
		return ""
	}
	return f.FieldSignatures[name]
}

// MethodSignature returns the raw generic Signature string of a same-class method identified by name
// and arity (number of descriptor parameters == number of call-site arguments), or "" when there is
// no unique such method (see MethodSignatures). Overloads colliding on (name, arity) are deliberately
// dropped at population time so a call site never resolves to the wrong generic signature.
func (f *ClassContext) MethodSignature(name string, argc int) string {
	if f == nil || name == "" || f.MethodSignatures == nil {
		return ""
	}
	return f.MethodSignatures[methodSigKey(name, argc)]
}

// ConstructorSignature returns the raw generic Signature string of a same-class constructor with the
// given argument count, or "" when there is no unique offset-safe such constructor (see
// ConstructorSignatures).
func (f *ClassContext) ConstructorSignature(argc int) string {
	if f == nil || f.ConstructorSignatures == nil {
		return ""
	}
	return f.ConstructorSignatures[argc]
}

// methodSigKey builds the (name, arity) key used by MethodSignatures.
func methodSigKey(name string, argc int) string {
	return name + "/" + strconv.Itoa(argc)
}

// MethodSigKey is the exported builder for the MethodSignatures key, so the dumper populates the map
// with exactly the key MethodSignature looks up.
func MethodSigKey(name string, argc int) string {
	return methodSigKey(name, argc)
}

// FieldTypeVar reports the bare class-scope type variable a same-class field is declared as, or ""
// when name is not a recorded type-variable field (see FieldTypeVars).
func (f *ClassContext) FieldTypeVar(name string) string {
	if f == nil || name == "" || f.FieldTypeVars == nil {
		return ""
	}
	return f.FieldTypeVars[name]
}

// HasRawEraseTypeVars reports whether any undeclared-enclosing type variable must be raw-erased from
// parameterized type-argument render sites in the class currently being rendered (see RawEraseTypeVars).
func (f *ClassContext) HasRawEraseTypeVars() bool {
	return f != nil && len(f.RawEraseTypeVars) > 0
}

// RawEraseTypeVar reports whether the bare type-variable name is one this class references but cannot
// declare, so a parameterization using it as a direct argument must be rendered raw (see RawEraseTypeVars).
func (f *ClassContext) RawEraseTypeVar(name string) bool {
	if f == nil || name == "" || f.RawEraseTypeVars == nil {
		return false
	}
	return f.RawEraseTypeVars[name]
}

// IsTypeParam reports whether name is one of the class-scope type variables (see TypeParams).
func (f *ClassContext) IsTypeParam(name string) bool {
	if f == nil || name == "" {
		return false
	}
	for _, p := range f.TypeParams {
		if p == name {
			return true
		}
	}
	return false
}

var javaKeywords = map[string]struct{}{
	"abstract": {}, "assert": {}, "boolean": {}, "break": {}, "byte": {}, "case": {}, "catch": {},
	"char": {}, "class": {}, "const": {}, "continue": {}, "default": {}, "do": {}, "double": {},
	"else": {}, "enum": {}, "extends": {}, "final": {}, "finally": {}, "float": {}, "for": {},
	"goto": {}, "if": {}, "implements": {}, "import": {}, "instanceof": {}, "int": {}, "interface": {},
	"long": {}, "native": {}, "new": {}, "package": {}, "private": {}, "protected": {}, "public": {},
	"return": {}, "short": {}, "static": {}, "strictfp": {}, "super": {}, "switch": {}, "synchronized": {},
	"this": {}, "throw": {}, "throws": {}, "transient": {}, "try": {}, "void": {}, "volatile": {}, "while": {},
	"true": {}, "false": {}, "null": {}, "_": {},
}

func SafeIdentifier(name string) string {
	if name == "" {
		return "_"
	}
	var b strings.Builder
	for i, r := range name {
		valid := r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			continue
		}
		if i == 0 && r >= '0' && r <= '9' {
			b.WriteByte('_')
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	name = b.String()
	if _, ok := javaKeywords[name]; ok {
		return name + "_"
	}
	return name
}

func (f *ClassContext) GetAllImported() []string {
	imports := []string{}
	seen := map[string]struct{}{}
	f.BuildInLibsMap.ForEach(func(pkg string, classes []string) bool {
		if pkg == f.PackageName || pkg == "java.lang" {
			return true
		}
		for _, className := range classes {
			// A nested type may have been registered under its binary name (Outer$Inner). How to import
			// it depends on how its REFERENCE site is spelled (see ShortTypeName):
			//   - EXTERNAL stdlib nested type (java.*/javax.*/...): the reference uses the dotted source
			//     spelling (Map.Entry), so import the OUTER class (java.util.Map).
			//   - SAME-JAR Yak-emitted nested type: it is a STANDALONE flat top-level unit literally named
			//     `Outer$Inner` and the reference uses that exact flat name, so the import MUST carry the
			//     flat `$` name too (`import pkg.Outer$Inner;` is legal Java - '$' is an identifier char).
			//     Importing only the OUTER class (legacy behaviour, on the false premise that imports cannot
			//     contain '$') left `Outer$Inner.X` unresolved at every cross-package use site -> javac read
			//     it as `package Outer$Inner` (the single biggest fastjson2 cross-package recompile blocker:
			//     JSONReader$Feature / JSONWriter$Feature). Kill-switch: JDEC_NESTED_FLAT_IMPORT_OFF=1.
			// Anonymous/local segments ($1) are never importable, so drop them in either branch.
			if strings.Contains(className, "$") {
				// Only a digit-leading '$'-segment (Outer$1 / Outer$1Helper) marks an anonymous/local
				// class, which has no source name and can never be imported. A name that merely STARTS
				// WITH '$' or contains '$$' (an empty split segment) is NOT anonymous: '$' is a legal Java
				// identifier char, so it is a real, importable flat unit. The old gate used
				// binaryNestedNameToSource's ok-flag, which also rejects empty segments, so it wrongly
				// DROPPED the import for gson's top-level `$Gson$Preconditions` / `$Gson$Types` family at
				// every cross-package use site (`cannot find symbol`, gson's largest cluster). Kill-switch
				// JDEC_DOLLAR_FLAT_IMPORT_OFF restores the legacy drop-on-empty-segment behaviour.
				if isAnonymousOrLocalBinaryName(className) {
					continue
				}
				src, dotOK := binaryNestedNameToSource(className)
				if !dotOK && os.Getenv("JDEC_DOLLAR_FLAT_IMPORT_OFF") != "" {
					continue
				}
				stdlibOrLegacy := isStdlibNestedDottedPackage(pkg) || os.Getenv("JDEC_NESTED_FLAT_IMPORT_OFF") != ""
				// stdlib nested types import the OUTER class (the reference uses the dotted Outer.Inner
				// spelling); this only applies when the name is dot-splittable (dotOK). A '$'-leading flat
				// unit (dotOK==false) keeps its flat name so the import matches the flat reference.
				if stdlibOrLegacy && dotOK {
					outer := src
					if i := strings.IndexByte(src, '.'); i >= 0 {
						outer = src[:i]
					}
					className = outer
				}
				// else: keep the flat `Outer$Inner` / `$Gson$Preconditions` name verbatim.
			}
			imp := pkg + "." + className
			if _, dup := seen[imp]; dup {
				continue
			}
			seen[imp] = struct{}{}
			imports = append(imports, imp)
		}
		return true
	})
	// Sort imports for determinism: they are registered during dumping in traversal order, and some
	// paths (e.g. the enum DumpWithResolver) register types via Go-map iteration, so the registration
	// order varies run to run. Import order is semantically irrelevant to javac, so a stable lexical
	// sort removes that nondeterminism (and yields tidy alphabetical imports) without any delta impact.
	slices.Sort(imports)
	return imports
}
func (f *ClassContext) Import(name string) {
	if f.KeySet == nil {
		f.KeySet = utils.NewSet[string]()
	}
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	pkg, className := SplitPackageClassName(name)
	if pkg == "" || pkg == "java.lang" {
		return
	}
	if className != "*" {
		className = SafeIdentifier(className)
	}
	if f.KeySet.Has(className) {
		return
	}
	key, ok := f.BuildInLibsMap.Get(pkg)
	if ok {
		if slices.Contains(key, className) || slices.Contains(key, "*") {
			return
		}
	}
	f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
	f.KeySet.Add(className)
}

// stdlibNestedDottedPackages enumerates the package prefixes whose nested types are guaranteed to be
// JDK / standard-library types (never a Yak-emitted flat unit, and always present on the compile
// classpath as genuinely nested Outer.Inner). For these a nested-type REFERENCE must use the dotted
// Java source spelling (Map.Entry), not the binary flat name (Map$Entry) Yak uses for its own units.
func isStdlibNestedDottedPackage(pkg string) bool {
	switch {
	case pkg == "java" || strings.HasPrefix(pkg, "java."):
		return true
	case pkg == "javax" || strings.HasPrefix(pkg, "javax."):
		return true
	case pkg == "jdk" || strings.HasPrefix(pkg, "jdk."):
		return true
	case pkg == "sun" || strings.HasPrefix(pkg, "sun."):
		return true
	case strings.HasPrefix(pkg, "com.sun."):
		return true
	case strings.HasPrefix(pkg, "org.w3c."):
		return true
	case strings.HasPrefix(pkg, "org.xml."):
		return true
	case strings.HasPrefix(pkg, "org.ietf."):
		return true
	case strings.HasPrefix(pkg, "org.omg."):
		return true
	}
	return false
}

func (f *ClassContext) ShortTypeName(name string) string {
	pkg, className := SplitPackageClassName(name)
	className = SafeIdentifier(className)
	if pkg == "" {
		return className
	}
	// A reference to an EXTERNAL standard-library nested type must use the dotted Java source spelling
	// (java.util.Map.Entry -> Map.Entry), never the binary flat name (Map$Entry). Yak emits its OWN
	// nested classes as standalone flat `Outer$Inner` units and references them by that same flat name
	// so the whole decompiled set recompiles together; but a JDK/stdlib nested type is only present on
	// the compile classpath as a genuinely nested Outer.Inner and is unresolvable as `Outer$Inner` in
	// source (this was the single largest guava/spring recompile blocker - hundreds of `Map$Entry`
	// "cannot find symbol"). java.*/javax.*/... can never be a Yak unit, so the conversion is always
	// safe. The import statement still carries the OUTER class (see GetAllImported). Kill-switch:
	// JDEC_STDLIB_NESTED_DOT_OFF=1 restores the legacy flat spelling.
	dotted := className
	if strings.Contains(className, "$") && os.Getenv("JDEC_STDLIB_NESTED_DOT_OFF") == "" && isStdlibNestedDottedPackage(pkg) {
		if src, ok := binaryNestedNameToSource(className); ok {
			dotted = src
		}
	}
	if pkg == f.PackageName || pkg == "java.lang" {
		return dotted
	}
	f.Import(name)
	if f.BuildInLibsMap == nil {
		f.BuildInLibsMap = omap.NewEmptyOrderedMap[string, []string]()
	}
	libs := f.BuildInLibsMap.GetMust(pkg)
	if len(libs) > 0 && (funk.Contains(libs, className) || libs[0] == "*") {
		return dotted
	}
	//f.BuildInLibsMap.Set(pkg, append(f.BuildInLibsMap.GetMust(pkg), className))
	return pkg + "." + dotted
}

// binaryNestedNameToSource converts a binary nested class simple name (Outer$Inner$Deeper) into its
// Java source spelling (Outer.Inner.Deeper). It returns ok=false when the name is not nested or when
// any segment is anonymous/local (a segment that is empty or begins with a digit, e.g. Outer$1).
// NOTE: Yak emits each nested class as a STANDALONE top-level unit literally named `Outer$Inner`
// ('$' is a legal Java identifier char) and references it by that same flat name, which is internally
// consistent and recompiles when the whole decompiled source set is compiled together (the standard
// decompiler round-trip). This helper is therefore only used to keep import statements legal (an
// import line cannot contain '$'); type references stay flat to match the flat declarations.
// isAnonymousOrLocalBinaryName reports whether a binary nested class simple name (assumed to contain
// '$') denotes an anonymous (Outer$1) or local (Outer$1Helper) class -- i.e. some '$'-separated segment
// BEGINS WITH A DIGIT. Such classes have no usable source name and can never appear in an import. A
// merely empty segment (the name starts with '$', ends with '$', or contains '$$', e.g. gson's
// top-level `$Gson$Preconditions`) is NOT anonymous: '$' is a legal Java identifier char, so the class
// is a real importable flat unit. This is intentionally narrower than binaryNestedNameToSource's
// ok-flag (which also rejects empty segments) because importability and dot-splittability differ.
func isAnonymousOrLocalBinaryName(className string) bool {
	for _, p := range strings.Split(className, "$") {
		if p != "" && p[0] >= '0' && p[0] <= '9' {
			return true
		}
	}
	return false
}

func binaryNestedNameToSource(className string) (string, bool) {
	if !strings.Contains(className, "$") {
		return className, false
	}
	parts := strings.Split(className, "$")
	for _, p := range parts {
		if p == "" || (p[0] >= '0' && p[0] <= '9') {
			return className, false
		}
	}
	return strings.Join(parts, "."), true
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}
