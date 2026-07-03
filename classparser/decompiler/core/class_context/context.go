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
	// InjectedTypeParamBounds maps an INJECTED (flattened inner-class) type variable name to its
	// recovered non-Object bound CLAUSE (e.g. `C` -> `Comparable<?>`), exactly the map the dumper uses to
	// render `<C extends Comparable<?>>` on the flat class header. A variable absent from this map (or a
	// nil map) has the canonical bare `<C>` (Object) bound AS RENDERED. Unlike a class's OWN formal type
	// parameters (whose bounds are parseable from ClassSig), an injected variable is NOT declared in
	// ClassSig, so its boundedness is otherwise unknowable to renderers. Consumed by parameterizedReturnCast
	// (which may only emit an unchecked `X<Object>` -> `X<E>` cast for an UNBOUNDED E, since a bounded one
	// makes that cast inconvertible). Nil for an ordinary (non-inner / own-formal) class.
	InjectedTypeParamBounds map[string]string
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
	// MethodSignaturesByDesc maps a same-class method's (name, EXACT JVM descriptor) key (see
	// MethodDescKey) to its raw generic method Signature string. It is the descriptor-keyed companion of
	// MethodSignatures (arity-keyed): because (name, descriptor) is unique in the JVM, it retains
	// overloaded methods that MethodSignatures had to drop as (name, arity)-ambiguous, so a same-class
	// call whose exact descriptor is known (FunctionCallExpression.Descriptor) can still recover its
	// erased `(K)`/`(E)` argument cast (guava Builder `putAll(K, Iterable)` vs varargs `putAll(K, V...)`;
	// `add(E)` vs `add(E...)`). Consumed by sameClassMethodParamType as a fallback after the arity path
	// declines. Kill-switch consumer: JDEC_SAMECLASS_DESC_SIG_OFF. Empty when the class has no such method.
	MethodSignaturesByDesc map[string]string
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
	// CurrentMethodSig is the raw generic method Signature string of the method CURRENTLY being rendered
	// (e.g. `<C:Ljava/util/Collection<-TE;>;>(TC;)TC;` for `<C extends Collection<? super E>> C copyInto
	// (C)`). Unlike MethodSignatures (which maps OTHER same-class methods for call-site resolution), this
	// is the enclosing method whose body is being emitted, set on entry and restored on exit. It lets a
	// renderer recover the PARAMETERIZED BOUND of a receiver whose static type is a bare method-scope type
	// variable (`C var1`): the value type is just `C`, but the bound `Collection<? super E>` carries the
	// element type args downstream receiver/param resolution needs (guava FluentIterable.copyInto
	// `var1.add(...)`, Multimaps.invertFrom `var1.put(...)`). Empty for a non-generic method. Stored as a
	// string (class_context must not import the types package; renderers parse it via
	// types.FormalTypeParamBounds). Kill-switch consumers: JDEC_TYPEVAR_BOUND_RECV_OFF.
	CurrentMethodSig string
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
	// StandaloneEraseTypeVars maps each undeclarable enclosing type-variable name (the same set as
	// RawEraseTypeVars) to the ERASURE it must render as when used as a STANDALONE type -- a field type,
	// method parameter/return, cast, or local declaration (`E nextEntry;`, `T output(K var0, V var1)`).
	// RawEraseTypeVars only strips a variable used as a type ARGUMENT (`Node<K,V>` -> `Node`); a variable
	// used as a standalone type has no `<...>` to strip, so it would render as a bare undeclared `K`/`E`
	// (javac "cannot find symbol: class K"; guava AbstractMapBasedMultimap$Itr `K key`, output(K,V) and
	// MapMakerInternalMap$HashIterator `E nextEntry`, advanceTo(E)). Rendering the JVM erasure instead --
	// the variable's first bound's raw class (`E extends InternalEntry<..>` -> InternalEntry), or
	// java.lang.Object for an unbounded variable -- is runtime-identical and compiles: member accesses on
	// the erased value resolve against the bound (`nextEntry.getNext()`), and every own-formal-declared
	// sibling override (`$1.output(K,V)` where K,V are its own injected params) erases to the same
	// signature, so the override relation is preserved. Nil/empty for every ordinary class. Populated only
	// for a flattened NON-STATIC inner class that has its OWN formal type parameters. See dumper
	// (JDEC_INNER_STANDALONE_ERASE_OFF) and types.JavaClass.String.
	StandaloneEraseTypeVars map[string]string
	// SuppressStandaloneErase temporarily disables StandaloneEraseTypeVars while rendering a position
	// where erasing to Object would HURT: an ABSTRACT method's parameter types. Erasing an abstract
	// method's `output(K,V)` to `output(Object,Object)` makes a no-own-formal sibling override that
	// declares its own K,V (`AbstractMapBasedMultimap$1.output(K,V)`) no longer override it ("same
	// erasure, yet neither overrides the other" + "abstract method not overridden"), turning one
	// undeclared-symbol error into two clash errors. Keeping the abstract parameter as the bare
	// (undeclared) variable is no worse than before this fix, while fields/locals/concrete-method
	// positions still benefit from the erasure. Set/cleared by the dumper around abstract param rendering.
	SuppressStandaloneErase bool
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
	// SiblingCtorSig resolves a jar-internal class's CONSTRUCTOR generic Signature by binary internal name
	// (slash-form) and DESCRIPTOR argument count. It returns the raw `<init>` Signature string (e.g.
	// `(Lcom/google/common/graph/BaseGraph<TN;>;TN;)V` for IncidentEdgeSet) or ok=false for JDK/external
	// classes, ambiguous overloads (same arity), or offset-mismatched inner-class ctors (a non-static
	// inner's ctor Signature omits the synthetic leading this$0, so its param count differs from the
	// descriptor -- skipped to avoid mis-indexing). SiblingClassSig deliberately SKIPS <init>; this fills
	// that gap so a `super(...)` call can recover which arguments feed a bare type-variable ctor parameter
	// and re-emit the erased `(N)`/`(C)` cast (guava graph IncidentEdgeSet subclasses' `super(g, node)`,
	// RegularContiguousSet anonymous iterators' `super(first)`). The dumper builds this closure (it owns the
	// byte resolver + parser); the type/class_context packages consume only strings. Nil on the single-class
	// path; set only on the jar / DecompileWithResolver path. Kill-switch consumer:
	// JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF.
	SiblingCtorSig func(internalName string, argc int) (sig string, ok bool)
	// SiblingFieldSig resolves a jar-internal class's FIELD generic Signature by binary internal name
	// (slash-form) and field name, or ok=false for JDK/external classes (bytes not in jar) or a field with
	// no generic Signature attribute. SiblingClassSig carries only the class + method Signatures; this fills
	// the field gap so an INHERITED parameterized field (whose Signature lives in a superclass, absent from
	// the current class's FieldSignatures) can be recovered by walking the hierarchy with type-argument
	// substitution (types.ResolveInstantiatedFieldType, consumed in receiverParamTypeArgs): guava
	// RegularContiguousSet<C>'s `this.domain` is declared `DiscreteDomain<C>` in ContiguousSet, so
	// `this.domain.distance(first(), var1)` recovers the erased `(C)` cast on var1. The dumper builds this
	// closure (it owns the byte resolver + parser); the type/class_context packages consume only strings.
	// The field type is intentionally an unnamed func type so its value is directly assignable to
	// types.FieldSigProvider (identical signature) without class_context importing types. Nil on the
	// single-class path; set only on the jar / DecompileWithResolver path. Kill-switch consumer:
	// JDEC_INHERITED_FIELD_SIG_OFF.
	SiblingFieldSig func(internalName, fieldName string) (sig string, ok bool)
	// SamePkgFQNames is the set of SafeIdentifier'd simple type names that live in THIS class's own
	// package but must nonetheless be rendered fully-qualified, because the class ALSO references a
	// DIFFERENT type with the same simple name from another package. A single-type-import of that
	// other type (`import a.b.FieldWriter;`) shadows the same-package one (JLS 6.4.1 / 7.5.1), so the
	// bare simple name would resolve to the WRONG type -- the canonical hit is fastjson2
	// ObjectWriterCreatorASM, which uses both `com.alibaba.fastjson2.writer.FieldWriter<T>` (same
	// package, generic) and `com.alibaba.fastjson2.internal.asm.FieldWriter` (imported, non-generic):
	// the import made `FieldWriter<T>` resolve to the non-generic ASM class -> "type FieldWriter does
	// not take parameters". The dumper computes this from the constant pool BEFORE rendering (so it is
	// render-order independent) and ShortTypeName consults it for the same-package branch. Nil/empty
	// for the overwhelming majority of classes (no cross-package simple-name clash), so a strict no-op
	// there. Kill-switch: JDEC_SAMEPKG_FQ_OFF=1 (dumper leaves this nil).
	SamePkgFQNames map[string]bool
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

// MethodSignatureByDesc returns the raw generic Signature string of a same-class method identified by
// name and its EXACT JVM descriptor, or "" when none. Unlike MethodSignature (arity-keyed, which drops
// same-arity overloads as ambiguous), a (name, descriptor) pair is UNIQUE in the JVM, so this resolves an
// overloaded same-class method precisely from the call's descriptor -- recovering the erased `(K)`/`(E)`
// argument cast for calls the arity path had to abandon (guava Builder `putAll(K, Iterable)` vs
// `putAll(K, V...)`; `add(E)` vs `add(E...)`). See MethodSignaturesByDesc.
func (f *ClassContext) MethodSignatureByDesc(name, descriptor string) string {
	if f == nil || name == "" || descriptor == "" || f.MethodSignaturesByDesc == nil {
		return ""
	}
	return f.MethodSignaturesByDesc[methodDescKey(name, descriptor)]
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

// methodDescKey builds the (name, descriptor) key used by MethodSignaturesByDesc.
func methodDescKey(name, descriptor string) string {
	return name + descriptor
}

// MethodDescKey is the exported builder for the MethodSignaturesByDesc key, so the dumper populates the
// map with exactly the key MethodSignatureByDesc looks up.
func MethodDescKey(name, descriptor string) string {
	return methodDescKey(name, descriptor)
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

// StandaloneEraseTypeVar returns the erasure spelling this class must render for a STANDALONE use of the
// bare enclosing type-variable `name` (a field/param/return/cast/local type), and ok=false when `name` is
// not an undeclarable enclosing variable (so the normal type-variable name is kept). See
// StandaloneEraseTypeVars.
func (f *ClassContext) StandaloneEraseTypeVar(name string) (string, bool) {
	if f == nil || name == "" || f.StandaloneEraseTypeVars == nil || f.SuppressStandaloneErase {
		return "", false
	}
	repl, ok := f.StandaloneEraseTypeVars[name]
	return repl, ok
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
				stdlibOrLegacy := f.nestedTypeShouldDot(pkg, className) || os.Getenv("JDEC_NESTED_FLAT_IMPORT_OFF") != ""
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
// nestedTypeShouldDot reports whether a `$`-nested binary type name (in package pkg) must be rendered
// with the DOTTED source spelling (Outer.Inner) rather than Yak's flat `Outer$Inner`. Two cases:
//
//   - stdlib packages (java.*/javax.*/kotlin.*/...): always dotted (isStdlibNestedDottedPackage).
//   - EXTERNAL third-party nested types whose OUTER class is NOT one of the jar's own decompiled units
//     (reactor.blockhound.BlockHound$Builder in spring-core): they only exist on the compile classpath
//     as a genuinely nested `Outer.Inner`, so the flat `Outer$Inner` is unresolvable ("cannot find
//     symbol: class BlockHound$Builder"). Yak NEVER emits a unit for a class outside the jar, so
//     dotting these is safe. The jar-membership test is SiblingSuperTypes (reads the always-present
//     super_class entry, so it resolves for EVERY jar-internal class, generic or not; ok=false means
//     the class is external). When no cross-class resolver is wired (single-class decompile,
//     SiblingSuperTypes==nil) the answer is "keep flat" -- unchanged legacy behavior.
//
// Kill-switch JDEC_EXTERNAL_NESTED_DOT_OFF disables ONLY the external-class extension (stdlib dotting
// is unaffected).
func (f *ClassContext) nestedTypeShouldDot(pkg, className string) bool {
	if isStdlibNestedDottedPackage(pkg) {
		return true
	}
	if f == nil || f.SiblingSuperTypes == nil || os.Getenv("JDEC_EXTERNAL_NESTED_DOT_OFF") != "" {
		return false
	}
	// Same-package nested types are (almost always) Yak's own flat units; never dot them.
	if pkg == f.PackageName {
		return false
	}
	outerBin := pkg + "." + className
	if i := strings.IndexByte(className, '$'); i >= 0 {
		outerBin = pkg + "." + className[:i]
	}
	outerBin = strings.ReplaceAll(outerBin, ".", "/")
	if _, ok := f.SiblingSuperTypes(outerBin); ok {
		return false // outer class IS in the jar -> Yak emits it flat -> keep flat
	}
	return true // outer class is external -> genuinely nested -> dot it
}

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
	// Kotlin runtime/reflect packages are never Yak-decompiled units (they ship as an external optional
	// dependency), so their nested types (e.g. kotlin.reflect.KParameter.Kind) are only resolvable on
	// the classpath under the dotted source name. Left flat, `KParameter$Kind.VALUE` makes javac read
	// `KParameter$Kind` as a package ("package KParameter$Kind does not exist"): spring-core's
	// KotlinReflectionParameterNameDiscoverer / MethodParameter$KotlinDelegate.
	case pkg == "kotlin" || strings.HasPrefix(pkg, "kotlin."):
		return true
	case pkg == "kotlinx" || strings.HasPrefix(pkg, "kotlinx."):
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
	if strings.Contains(className, "$") && os.Getenv("JDEC_STDLIB_NESTED_DOT_OFF") == "" && f.nestedTypeShouldDot(pkg, className) {
		if src, ok := binaryNestedNameToSource(className); ok {
			dotted = src
		}
	}
	if pkg == f.PackageName || pkg == "java.lang" {
		// A same-package (or java.lang) type is normally reachable by its bare simple name with no
		// import. But when this class ALSO references a DIFFERENT-package type of the same simple name,
		// that type gets a single-type-import which SHADOWS the same-package/java.lang one, so the bare
		// name would bind to the wrong type. In that case emit the fully-qualified name instead. The
		// clashing set is precomputed from the constant pool by the dumper (render-order independent).
		if pkg == f.PackageName && f.SamePkgFQNames != nil && f.SamePkgFQNames[className] {
			return pkg + "." + dotted
		}
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
