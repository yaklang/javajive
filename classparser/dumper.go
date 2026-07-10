package javaclassparser

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	utils2 "github.com/yaklang/javajive/classparser/decompiler/core/utils"

	"github.com/samber/lo"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/utils"
)

type ClassObjectDumper struct {
	obj           *ClassObject
	FuncCtx       *class_context.ClassContext
	ClassName     string
	PackageName   string
	CurrentMethod *MemberInfo
	ConstantPool  []ConstantInfo
	deepStack     *utils.Stack[int]
	MethodType    *types.JavaFuncType
	lambdaMethods map[string][]string
	// lambdaCaptureCount records, per synthetic lambda impl method (keyed by name+descriptor),
	// how many leading parameters are captured variables that javac prepended to the impl
	// signature. They are not lambda parameters: DumpMethodWithInitialId drops them from the arrow
	// parameter list and renames them to capture placeholders that the invokedynamic call site
	// resolves to the actual captured values.
	lambdaCaptureCount map[string]int
	// lambdaLocalSeq hands each inlined lambda body a unique id so its own locals can be renamed
	// into a private `lv<seq>_<n>` namespace. A lambda arrow body is spliced INLINE into the
	// enclosing method, and Java forbids a local declared in the lambda body from shadowing a
	// local/parameter of the enclosing scope (or a captured variable, which resolves to an
	// enclosing `varN`). The fresh-root id namespace gives lambda locals var0,var1,... that collide
	// with the enclosing method's var0,var1,..., producing "variable varN is already defined".
	// Renaming them per-lambda eliminates the collision; nested lambdas are dumped first and already
	// carry their own `lv<innerseq>` names, so the outer rename (matching only `varN`) never touches
	// them. See renameLambdaBodyLocals.
	lambdaLocalSeq int
	// lambdaDepth is the current lexical nesting depth of lambda bodies being dumped. A nested lambda
	// body is rendered EAGERLY while its enclosing lambda's bytecode is still being parsed (the
	// invokedynamic that materialises the inner arrow runs inside the outer's ParseBytesCode), so the
	// enclosing lambda's own parameters are not yet named when the inner picks its parameter names.
	// javac forbids a lambda parameter from shadowing an enclosing lambda parameter ("variable l0 is
	// already defined"), so a flat `l0,l1,...` scheme collides for nested lambdas (spring-core
	// MergedAnnotationPredicates.typeIn, DataBufferUtils.readAsynchronousFileChannel). We namespace
	// the parameters of a lambda at depth>=2 by depth (`l<depth>_<i>`), which is collision-free:
	// nesting strictly increases depth, and two lambdas at the SAME depth are always siblings in
	// DISJOINT scopes (safe to share names). Top-level lambdas (depth 1) keep `l<i>`, so the common
	// case is byte-for-byte unchanged. See DumpMethodWithInitialId. Kill-switch:
	// JDEC_LAMBDA_PARAM_SCOPE_OFF=1 restores the flat `l<i>` naming.
	lambdaDepth       int
	fieldDefaultValue map[string]string
	dumpedMethodsSet  map[string]*dumpedMethods
	// aggressive marks that the CURRENT method dump is a second attempt for a method whose
	// conservative decompilation already failed. While set, the decompiler enables higher-risk
	// reconstruction paths (relaxed structuring, node-duplication, synthetic rebuilds). It is
	// toggled per-method by aggressiveRedumpMethod and is otherwise always false, so methods that
	// decompile cleanly on the first pass are never affected (zero regression by construction).
	aggressive bool
	// aggressiveRetried records methods (name+desc) already attempted in aggressive mode, so a
	// method that reaches both degradation points (DumpMethods and degradeInvalidMethods) is
	// re-decompiled at most once; the aggressive path is deterministic, so repeating is pointless.
	aggressiveRetried map[string]bool
	// fieldStoreTotals counts, per field name, how many putfield/putstatic targets it has across
	// ALL of this class's <init> and <clinit> bodies. It is computed lazily (and cached) by a
	// read-only opcode pre-scan and is used to suppress field-initializer hoisting for blank-final
	// fields that are assigned in more than one place (multiple constructors or multiple branches),
	// which would otherwise emit an illegal double assignment to a final field. A nil map means the
	// pre-scan has not run yet; an entry of 0 means "not seen", so callers treat <=1 as hoistable.
	fieldStoreTotals map[string]int
	// methodReturnTypes maps a same-class method name to its rendered return type, used by the
	// generated-local safety net to recover the type of a reference local that only receives its
	// value through an embedded assignment `(v = m(...)) != null`. Names that are overloaded with
	// DIFFERENT return types are omitted (ambiguous). Built lazily and cached.
	methodReturnTypes map[string]string
	// foldSiblingResolver, when non-nil, resolves a sibling class's raw bytes by its binary internal
	// name (slash form, e.g. "ev/EnumBody$1"). It is the hook that enables enum constant-body
	// CROSS-CLASS folding: a constant-specific class body is compiled by javac into a synthetic
	// `Outer$N` subclass, and the only legal Java is to inline that subclass's members back into the
	// enum constant (`CONST { ...body... }`). The standalone single-class entry (Decompile) leaves
	// this nil, so folding is OFF and per-class output is byte-for-byte unchanged (zero regression by
	// construction); only the multi-class entry (DecompileWithResolver / jar path) sets it.
	foldSiblingResolver func(internalName string) ([]byte, bool)
}

func (c *ClassObjectDumper) GetConstructorMethodName() string {
	if c.PackageName == "" {
		return c.ClassName
	}
	after, ok := strings.CutPrefix(c.ClassName, c.PackageName+".")
	if ok {
		return after
	}
	log.Error("GetConstructorMethodName failed")
	return ""
}
func NewClassObjectDumper(obj *ClassObject) *ClassObjectDumper {
	return &ClassObjectDumper{
		obj:                obj,
		ConstantPool:       obj.ConstantPool,
		deepStack:          utils.NewStack[int](),
		lambdaMethods:      map[string][]string{},
		lambdaCaptureCount: map[string]int{},
		fieldDefaultValue:  map[string]string{},
		dumpedMethodsSet:   map[string]*dumpedMethods{},
		aggressiveRetried:  map[string]bool{},
	}
}
func (c *ClassObjectDumper) TabNumber() int {
	return c.deepStack.Peek()
}
func (c *ClassObjectDumper) GetTabString() string {
	return strings.Repeat("\t", c.deepStack.Peek())
}
func (c *ClassObjectDumper) Tab() {
	pre := c.deepStack.Peek()
	if pre == 0 {
		c.deepStack.Push(1)
	} else {
		c.deepStack.Push(pre + 1)
	}
}
func (c *ClassObjectDumper) UnTab() {
	c.deepStack.Pop()
}

// selfInnerClassAccessFlags returns the inner_class_access_flags this class carries in its own
// InnerClasses entry (the entry whose inner_class_info refers to this very class), with ok=false when
// no such entry exists. For a nested type the top-level ClassFile access_flags omit its real
// visibility (a public nested type's own access_flags lack ACC_PUBLIC); the authoritative visibility
// is in the InnerClasses attribute. javap reads exactly this to print `public` for a nested type.
func (c *ClassObjectDumper) selfInnerClassAccessFlags() (uint16, bool) {
	return innerSelfAccessFlags(c.obj)
}

// superIsOwnFormalFlattenedSibling reports whether this class's direct superclass is a flattened
// `$`-named SIBLING (same top-level nest) that declares its OWN formal type parameters. That is exactly
// the shape of ConcurrentReferenceHashMap$1..$5 extends ConcurrentReferenceHashMap$Task<T>: the super
// (Task<T>) went through the own-formal raw-erase path (case a) and rendered its method params raw
// (`execute(Reference, Entry)`), so a subclass override rendered with the injected `<K,V>` generics
// (`execute(Reference<K,V>, Entry<K,V>)`) would clash ("same erasure, yet neither overrides"). Erasing
// the subclass's override params to match restores the override. Requires foldSiblingResolver (a
// single-class decompile cannot see the sibling, and there the injection+clash do not both arise the
// same way). A super whose formal-param set is EMPTY (no-own-formal, or a raw JDK class) is rejected:
// it declares+renders those vars generically, so erasing here would instead CREATE a clash.
func (c *ClassObjectDumper) superIsOwnFormalFlattenedSibling() bool {
	if c.foldSiblingResolver == nil {
		return false
	}
	super := c.obj.GetSupperClassName()
	if super == "" || !strings.Contains(super, "$") {
		return false
	}
	self := strings.ReplaceAll(c.obj.GetClassName(), ".", "/")
	// Same top-level nest: both share the substring up to the first '$'.
	topOf := func(n string) string {
		if i := strings.IndexByte(n, '$'); i >= 0 {
			return n[:i]
		}
		return n
	}
	if topOf(self) != topOf(super) {
		return false
	}
	data, ok := c.foldSiblingResolver(super)
	if !ok || len(data) == 0 {
		return false
	}
	sObj, err := Parse(data)
	if err != nil {
		return false
	}
	if flags, ok := innerSelfAccessFlags(sObj); ok && flags&StaticFlag != 0 {
		return false
	}
	for _, attr := range sObj.Attributes {
		if sa, ok := attr.(*SignatureAttribute); ok {
			if sig, err := sObj.getUtf8(sa.SignatureIndex); err == nil && sig != "" {
				return len(types.ClassFormalTypeParamNames(sig)) > 0
			}
			break
		}
	}
	return false
}

// innerSelfAccessFlags returns the inner_class_access_flags obj carries in the InnerClasses entry that
// refers to obj itself, with ok=false when none exists. Free-function form of selfInnerClassAccessFlags
// so it can also be applied to a sibling class parsed on the fly.
func innerSelfAccessFlags(obj *ClassObject) (uint16, bool) {
	self := obj.GetClassName()
	if self == "" {
		return 0, false
	}
	for _, attr := range obj.Attributes {
		ic, ok := attr.(*InnerClassesAttribute)
		if !ok {
			continue
		}
		for _, e := range ic.Classes {
			if e == nil || e.InnerClassInfoIndex == 0 {
				continue
			}
			name, err := obj.getUtf8(e.InnerClassInfoIndex)
			if err != nil {
				continue
			}
			if name == self {
				return e.InnerClassAccessFlags, true
			}
		}
	}
	return 0, false
}

// computeSamePkgFQNames scans the constant pool for CONSTANT_Class entries and returns the set of
// SafeIdentifier'd simple type names that (a) occur in THIS class's own package AND (b) also occur in
// some OTHER package. Such a same-package type is shadowed by the single-type-import the other-package
// type gets (JLS 7.5.1), so its bare simple name must be rendered fully-qualified. Being constant-pool
// based, the result is independent of body render order (the streaming importer could otherwise decide
// too late). Returns nil (strict no-op) when the kill-switch is set, the pool is unavailable, or no
// cross-package simple-name clash exists -- the common case. Kill-switch: JDEC_SAMEPKG_FQ_OFF=1.
// extractDescriptorClassNames pulls every reference-type internal name out of a JVM descriptor or
// generic Signature string. A reference type is spelled `L<binary/name>;` in a descriptor and
// `L<binary/name>...` (terminated by `;`, `<`, or `.`) inside a generic Signature. Type variables
// (`T...;`), primitives, and array markers are naturally skipped because they do not start with `L`.
// The returned names are slash-separated with no leading `L` / trailing terminator. Over-matching a
// non-descriptor Utf8 (e.g. a string constant that happens to contain `Lfoo/Bar;`) at worst adds a
// harmless extra type to the clash scan.
func extractDescriptorClassNames(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != 'L' {
			continue
		}
		j := i + 1
		for j < len(s) {
			ch := s[j]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
				ch == '_' || ch == '/' || ch == '$' {
				j++
				continue
			}
			break
		}
		// A valid reference type must be terminated by ';', '<' (generic args), or '.' (inner type in
		// a signature) and must contain a package separator to matter for a cross-package clash.
		if j < len(s) && (s[j] == ';' || s[j] == '<' || s[j] == '.') {
			name := s[i+1 : j]
			if strings.Contains(name, "/") {
				out = append(out, name)
			}
		}
		i = j
	}
	return out
}

func (c *ClassObjectDumper) computeSamePkgFQNames() map[string]bool {
	if os.Getenv("JDEC_SAMEPKG_FQ_OFF") != "" {
		return nil
	}
	if c.obj == nil || c.obj.ConstantPoolManager == nil {
		return nil
	}
	pkgsBySimple := map[string]map[string]struct{}{}
	record := func(binary string) {
		// binary is a slash-separated internal name (no leading L / trailing ;), possibly nested
		// (Outer$Inner). It must carry a package ('/') for a cross-package clash to be possible.
		if binary == "" || !strings.Contains(binary, "/") {
			return
		}
		dotted := strings.ReplaceAll(binary, "/", ".")
		pkg, simple := class_context.SplitPackageClassName(dotted)
		if simple == "" {
			return
		}
		simple = class_context.SafeIdentifier(simple)
		m := pkgsBySimple[simple]
		if m == nil {
			m = map[string]struct{}{}
			pkgsBySimple[simple] = m
		}
		m[pkg] = struct{}{}
	}
	// (1) CONSTANT_Class entries (new/checkcast/instanceof/method+field owners), stripping any array
	// descriptor prefix. (2) EVERY Utf8 entry: method/field descriptors and generic Signatures embed
	// referenced types as `L<binary-name>;` (or `L<binary-name><...>` in a signature), and a
	// same-package type used ONLY as a method return/parameter (e.g. the return type of an inherited
	// or cross-class method) appears there but NOT as a CONSTANT_Class -- the fastjson2 FieldWriter
	// clash is exactly this shape. Scanning both is a superset; over-inclusion only ever FQ-qualifies a
	// same-package name that also genuinely occurs in another package, which is always legal Java.
	for _, ci := range c.obj.ConstantPool {
		switch e := ci.(type) {
		case *ConstantClassInfo:
			u := c.obj.ConstantPoolManager.GetUtf8(int(e.NameIndex))
			if u == nil {
				continue
			}
			bin := u.Value
			for strings.HasPrefix(bin, "[") {
				bin = bin[1:]
			}
			if strings.HasPrefix(bin, "L") && strings.HasSuffix(bin, ";") {
				bin = bin[1 : len(bin)-1]
			}
			if strings.ContainsAny(bin, "[;<") {
				continue
			}
			record(bin)
		case *ConstantUtf8Info:
			for _, name := range extractDescriptorClassNames(e.Value) {
				record(name)
			}
		}
	}
	out := map[string]bool{}
	for simple, pkgs := range pkgsBySimple {
		if _, hasOwn := pkgs[c.PackageName]; !hasOwn {
			continue
		}
		for p := range pkgs {
			if p != c.PackageName && p != "" {
				out[simple] = true
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *ClassObjectDumper) DumpClass() (string, error) {
	// accessFlagsVerbose := c.obj.AccessFlagsVerbose
	accessFlagsToCode := c.obj.AccessFlagsToCode

	nonClassKeyword := false
	isInterface := false
	isEnum := false
	isAnnotation := false
	syntheticEnumSubclass := false
	superRawName := strings.Replace(c.obj.GetSupperClassName(), "/", ".", -1)
	for _, k := range c.obj.AccessFlagsVerbose {
		if k == "interface" || k == "enum" || k == "annotation" {
			if k == "interface" {
				isInterface = true
			} else if k == "annotation" {
				isAnnotation = true
			} else if k == "enum" {
				// A genuine enum extends java.lang.Enum directly. Synthetic enum-constant
				// subclasses (e.g. Foo$1) carry ACC_ENUM but extend the enum type itself and
				// cannot be declared with the `enum` keyword; render them as ordinary classes.
				if superRawName != "java.lang.Enum" {
					syntheticEnumSubclass = true
					break
				}
				isEnum = true
			}

			nonClassKeyword = true
			break
		}
	}

	//if len(accessFlagsVerbose) < 1 {
	//	return "", utils.Error("accessFlagsVerbose is empty")
	//}
	accessFlags := accessFlagsToCode
	if syntheticEnumSubclass {
		// Drop the `enum` keyword so the synthetic subclass renders as a normal class.
		accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "enum", ""))
	}
	name := c.obj.GetClassName()
	splits := strings.Split(name, "/")
	packageName := strings.Join(splits[:len(splits)-1], ".")
	c.PackageName = packageName
	rawClassName := splits[len(splits)-1]
	className := class_context.SafeIdentifier(rawClassName)
	// Nested/local/anonymous classes carry a '$' in their binary name (Outer$Inner). Yak emits each
	// such class as a STANDALONE top-level unit literally named `Outer$Inner` and writes it to
	// `Outer$Inner.java` ('$' is a legal Java identifier char), so a `public` modifier IS legal here
	// (Java only requires that a file's public top-level class match the file name). `protected` is
	// illegal at top level and is always dropped (demoted to package-private).
	//
	// Crucially, a nested type's REAL visibility lives in the InnerClasses attribute's
	// inner_class_access_flags, NOT in the (visibility-less) top-level ClassFile access_flags: a public
	// nested type's own access_flags lack ACC_PUBLIC, so the legacy unconditional public-stripping left
	// every public nested type package-private. Cross-package use sites then failed to recompile with
	// `... is defined in an inaccessible class or interface` and `package Outer$Inner does not exist`
	// (the single biggest fastjson2 blocker: JSONReader$Feature / JSONWriter$Feature / JSONReader$Context).
	// Recover ACC_PUBLIC from InnerClasses (this is exactly what javap consults) and keep `public` for a
	// genuinely public nested type. Kill-switch: JDEC_NESTED_PUBLIC_OFF=1 restores legacy stripping.
	if strings.Contains(rawClassName, "$") {
		// `protected` is illegal at top level (a flattened nested unit is emitted as a top-level class),
		// so always drop it.
		accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "protected", ""))
		innerFlags, isNested := c.selfInnerClassAccessFlags()
		switch {
		case os.Getenv("JDEC_NESTED_PUBLIC_OFF") != "":
			// Legacy: strip `public` from every '$'-named class (kept for A/B comparison).
			accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "public", ""))
		case !isNested:
			// A GENUINE top-level class whose name merely contains '$' (gson's `$Gson$Preconditions` /
			// `$Gson$Types`, deliberately '$'-named to avoid collisions): it appears in NO InnerClasses
			// attribute, so its top-level ClassFile access_flags already carry the correct visibility
			// (ACC_PUBLIC for a public class). Keep them verbatim. The old code assumed every '$' name was
			// nested, failed the InnerClasses lookup, and wrongly stripped `public`, making the class
			// inaccessible across packages ("$Gson$Preconditions is not public in com.google.gson.internal").
		case innerFlags&0x0001 == 0x0001:
			// Genuinely nested AND public per InnerClasses: a public nested type's top-level access_flags
			// omit ACC_PUBLIC (real visibility lives in InnerClasses, which is what javap consults), so
			// add `public` (fastjson2 JSONReader$Feature / JSONWriter$Feature family).
			if !strings.Contains(accessFlags, "public") {
				accessFlags = strings.TrimSpace("public " + accessFlags)
			}
		case innerFlags&0x0004 == 0x0004 && os.Getenv("JDEC_NESTED_PROTECTED_PUBLIC_OFF") == "":
			// Genuinely nested AND `protected` per InnerClasses. A protected nested type is reachable
			// from subclasses in OTHER packages via inheritance (e.g. cglib's
			// AbstractClassGenerator.Source / .ClassLoaderData, used by subclasses in the beans/proxy/
			// reflect/util packages). Once flattened to a standalone top-level unit that inheritance
			// relationship is gone, so leaving it package-private makes it unreachable cross-package
			// ("AbstractClassGenerator$Source is not public in ...; cannot be accessed from outside
			// package" -- the single biggest spring-core cglib blocker, 44 error lines across ~15
			// classes). `protected` is illegal at top level, so the only faithful-enough, recompile-safe
			// top-level encoding is to WIDEN it to `public` (widening visibility never breaks a compile).
			// Kill-switch: JDEC_NESTED_PROTECTED_PUBLIC_OFF=1 restores the legacy package-private stripping.
			if !strings.Contains(accessFlags, "public") {
				accessFlags = strings.TrimSpace("public " + accessFlags)
			}
		default:
			// Genuinely nested, non-public: strip any spurious `public`.
			accessFlags = strings.TrimSpace(strings.ReplaceAll(accessFlags, "public", ""))
		}
	}
	// module-info / package-info are synthetic descriptor pseudo-classes; their internal
	// name ("module-info" / "package-info") is not a legal Java identifier, so emitting
	// `class module-info {}` yields un-parseable source. Render a valid minimal compilation
	// unit instead. (Full JPMS module / package-info annotation reconstruction is a
	// separate feature.)
	if rawClassName == "module-info" || rawClassName == "package-info" {
		var sb strings.Builder
		if rawClassName == "package-info" && packageName != "" {
			sb.WriteString(fmt.Sprintf("package %s;\n\n", packageName))
		}
		sb.WriteString(fmt.Sprintf("// decompiled from a synthetic %s descriptor\n", rawClassName))
		return sb.String(), nil
	}
	supperClassName := c.obj.GetSupperClassName()
	supperClassName = strings.Replace(supperClassName, "/", ".", -1)
	if packageName == "" {
		c.ClassName = className
	} else {
		c.ClassName = packageName + "." + className
	}
	funcCtx := &class_context.ClassContext{
		ClassName:       c.ClassName,
		SupperClassName: supperClassName,
		PackageName:     c.PackageName,
	}
	c.FuncCtx = funcCtx
	// Precompute the same-package simple names that must be rendered fully-qualified because the class
	// also references a different-package type of the same simple name (whose import would shadow the
	// same-package one). Constant-pool based, so it is independent of body render order. See
	// ClassContext.SamePkgFQNames. Kill-switch: JDEC_SAMEPKG_FQ_OFF=1.
	funcCtx.SamePkgFQNames = c.computeSamePkgFQNames()
	buildInLib := []string{
		//c.PackageName + ".*",
		c.ClassName,
		"java.lang.*",
		//"java.io.*",
	}
	for _, s := range buildInLib {
		funcCtx.Import(s)
	}
	// Recover generic supertypes from the class Signature attribute: the raw super_class and
	// Interfaces constant-pool entries are erased, so a class like `Ints$IntConverter extends
	// Converter<Integer, Integer>` or `enum LexicographicalComparator implements Comparator<int[]>`
	// would otherwise render with raw supertypes and fail to override the erased generic methods.
	// Keyed by the raw dotted class name so it can be matched against each erased supertype below.
	// Kill-switch: JDEC_GENERIC_SUPERS_OFF=1 restores the erased supertypes.
	genericSuperByRaw := map[string]string{}
	if os.Getenv("JDEC_GENERIC_SUPERS_OFF") == "" {
		for _, attr := range c.obj.Attributes {
			sigAttr, ok := attr.(*SignatureAttribute)
			if !ok {
				continue
			}
			sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
			if err != nil || sigStr == "" {
				break
			}
			sup, sigIfaces := types.ParseClassSignatureSupers(sigStr)
			recordGeneric := func(t types.JavaType) {
				raw := genericSupertypeRawName(t)
				if raw == "" {
					return
				}
				rendered := t.String(funcCtx)
				// Only override when the recovered type actually carries type arguments; a raw
				// supertype in the signature adds nothing and must not shadow the erased name.
				if strings.Contains(rendered, "<") {
					genericSuperByRaw[raw] = rendered
				}
			}
			if sup != nil {
				recordGeneric(sup)
			}
			for _, it := range sigIfaces {
				recordGeneric(it)
			}
			break
		}
	}

	superStr := ""
	ifaces := c.obj.Interfaces
	interfaceLists := make([]string, 0, len(ifaces)+1)
	if supperClassName != "java.lang.Object" {
		if isEnum && (supperClassName == "java.lang.Enum" || supperClassName == "Enum") {
			supperClassName = ""
			superStr = ""
		} else {
			funcCtx.Import(supperClassName)
			rawSuper := supperClassName
			supperClassName = funcCtx.ShortTypeName(supperClassName)
			if generic, ok := genericSuperByRaw[rawSuper]; ok {
				supperClassName = generic
			}
			if supperClassName != "" {
				if !isEnum {
					superStr += fmt.Sprintf(" extends %s", supperClassName)
				} else {
					interfaceLists = append(interfaceLists, supperClassName)
				}
			}
		}
	}

	for _, u := range ifaces {
		info, err := c.obj.getConstantInfo(u)
		if err != nil {
			continue
		}
		classInfo := info.(*ConstantClassInfo)
		name, err := c.obj.getUtf8(classInfo.NameIndex)
		if err != nil {
			continue
		}
		rawIfaceName := strings.Replace(name, "/", ".", -1)
		// An annotation type implicitly extends java.lang.annotation.Annotation; emitting it
		// explicitly ("@interface M extends Annotation") is illegal Java, so drop it.
		if isAnnotation && rawIfaceName == "java.lang.annotation.Annotation" {
			continue
		}
		name = funcCtx.ShortTypeName(rawIfaceName)
		if generic, ok := genericSuperByRaw[rawIfaceName]; ok {
			name = generic
		}
		if name != "" {
			interfaceLists = append(interfaceLists, name)

		}
	}
	if len(interfaceLists) > 0 {
		if isInterface {
			superStr += fmt.Sprintf(" extends %s", strings.Join(interfaceLists, ", "))
		} else {
			superStr += fmt.Sprintf(" implements %s", strings.Join(interfaceLists, ", "))
		}
	}

	if packageName == "" {
		packageName = "defaultpackagename"
	}
	// Extract class-level type parameters from the Signature attribute so that
	// fields/methods referencing type variables (e.g. `T value`) compile. A class
	// without generic parameters or without a Signature attribute yields "".
	classTypeParams := ""
	classSigStr := ""
	for _, attr := range c.obj.Attributes {
		if sigAttr, ok := attr.(*SignatureAttribute); ok {
			if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
				classSigStr = sigStr
				// Render the type-variable bounds against the REAL funcCtx so a bound type in a
				// non-java.lang package (java.lang.annotation.Annotation, java.lang.reflect.Type) gets
				// its import registered; a throwaway context loses it and the class header recompiles as
				// "cannot find symbol" (spring MergedAnnotationSelector / FirstRunOfPredicate). Kill-switch
				// JDEC_TYPEPARAM_BOUND_IMPORT_OFF restores the (import-less) throwaway rendering.
				boundCtx := funcCtx
				if os.Getenv("JDEC_TYPEPARAM_BOUND_IMPORT_OFF") != "" {
					boundCtx = nil
				}
				if tp := types.ParseClassSignature(sigStr, boundCtx); tp != "" {
					classTypeParams = tp
				}
			}
			break
		}
	}
	// A non-static inner / local / anonymous class inherits type variables from its enclosing scope.
	// When Yak flattens it to a top-level `Outer$Inner` unit, those variables lose their declaration:
	// `class AbstractMapBasedMultimap$WrappedList extends AbstractMapBasedMultimap$WrappedCollection<K, V>
	// implements List<V>` references K, V that nothing declares -> javac "cannot find symbol: class K".
	// This was the single largest remaining guava recompile blocker (~2000 undeclared type-variable
	// errors across the Multimap/Table/cache inner-class families). Recover the variables this unit
	// actually USES by scanning its own supertype + field signatures for TypeVariableSignature references
	// and declaring them on the flattened class. Those positions can only reference class- or
	// enclosing-class-level variables (never method-level ones), so this never clashes with a method's
	// own `<T>`. Bounds default to Object, matching the common unbounded enclosing variable; a bounded
	// enclosing variable used in a bound-requiring position is a known residual. Kill-switch:
	// JDEC_INNER_TYPEVAR_OFF=1.
	//
	// RESTRICTED to classes that declare NO formal type parameters of their own. For such a
	// pure-inherited inner class the flattened reference sites still carry the enclosing type arguments
	// (parseSigClassType keeps the outer args of `LOuter<..>.Inner;`), so injecting the matching free
	// variables is arity-consistent. An inner class that ALSO has its own parameters (e.g.
	// MapMakerInternalMap$HashIterator<T>) renders references with only its own-param arity, so injecting
	// the enclosing variables would make declaration and reference arities disagree ("wrong number of
	// type arguments"); those are left to the future cross-class integral rebuild. A self-contained
	// top-level class references no free variables, so this is a strict no-op for it.
	// classTypeParamNames tracks the bare names of the type variables in scope for this class
	// (its own formal parameters, or the free variables injected on a flattened inner class).
	// It is propagated to the render context so statement renderers can recognize type-variable
	// references (e.g. to emit an unchecked `(T)` cast on an erased return value).
	classTypeParamNames := types.ClassFormalTypeParamNames(classSigStr)
	if os.Getenv("JDEC_INNER_TYPEVAR_OFF") == "" && len(classTypeParamNames) == 0 {
		seen := map[string]bool{}
		var free []string
		addRef := func(n string) {
			if n == "" || seen[n] {
				return
			}
			seen[n] = true
			free = append(free, n)
		}
		if classSigStr != "" {
			for _, n := range types.FreeTypeVarRefsInClassSig(classSigStr) {
				addRef(n)
			}
		}
		for _, field := range c.obj.Fields {
			for _, fattr := range field.Attributes {
				if sa, ok := fattr.(*SignatureAttribute); ok {
					if fs, err := c.obj.getUtf8(sa.SignatureIndex); err == nil && fs != "" {
						for _, n := range types.TypeVarRefsInFieldSig(fs) {
							addRef(n)
						}
					}
					break
				}
			}
		}
		// A flattened class can capture an enclosing type variable it references ONLY in a METHOD
		// PARAMETER -- never in its supertype or a field. This happens for an anonymous class created
		// inside a GENERIC METHOD: javac emits the class Signature WITHOUT a formal `<...>` section (it
		// carries only the free-var refs, e.g. guava `Futures$2` has `Ljava/lang/Object;LFuture<TO;>;` --
		// no `<O:...>` prefix), so O is recovered above as free from the supertype, but the free `I` in
		// `private O applyTransformation(I var1)` appears in no supertype/field and stays undeclared
		// ("cannot find symbol: class I"). Scan method-parameter signatures too so such a var is DECLARED
		// as a formal (`Futures$2<O, I>`), symmetric with how O is recovered. The raw `new Futures$2(...)`
		// call site is a raw instantiation and unaffected. Kill-switch JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF.
		if os.Getenv("JDEC_INNER_METHODPARAM_TYPEVAR_INJECT_OFF") == "" {
			for _, method := range c.obj.Methods {
				for _, mattr := range method.Attributes {
					if sa, ok := mattr.(*SignatureAttribute); ok {
						if ms, err := c.obj.getUtf8(sa.SignatureIndex); err == nil && ms != "" {
							for _, n := range types.TypeVarRefsInMethodParams(ms) {
								addRef(n)
							}
						}
						break
					}
				}
			}
		}
		// Variables are emitted in first-seen (supertype-then-field) order. The canonical enclosing order
		// is NOT recoverable from single-class bytecode (the synthetic this$0 field is erased to the raw
		// enclosing type with no Signature, and InnerClasses carries only names), so a sibling override
		// chain can occasionally bind a variable to a swapped position; that residual is left to the
		// future cross-class integral rebuild.
		//
		// ENCLOSING-ARITY RECONCILIATION: a flattened NON-STATIC inner class's reference sites always
		// carry the FULL enclosing formal-parameter set (javac encodes it as `LOuter<TK;TV;>.Inner;`,
		// which parseSigClassType carries onto the flattened name), but the usage scan above only recovers
		// the SUBSET the inner body actually mentions. So a class that uses K but not V is declared
		// `Inner<K>` yet referenced `Inner<K, V>` (gson LinkedTreeMap$KeySet -> javac "wrong number of
		// type arguments; required 1"), and one that uses NEITHER is declared bare yet referenced
		// `Inner<T>` (gson TreeTypeAdapter$GsonContextImpl -> "type Inner does not take parameters").
		// When the used set is a SUBSET of the NEAREST generic enclosing class's formal parameters (which
		// is exactly what a singly-nested reference carries), adopt that full ordered set so declaration
		// and reference arities AND order agree. The `free ⊆ encl` guard keeps deeper-nesting and
		// method-level residuals on the existing usage-based path. Kill-switch JDEC_INNER_ENCLOSING_ARITY_OFF.
		if encl := c.enclosingFormalTypeParamsForArity(); len(encl) > 0 && typeNamesSubset(free, encl) {
			free = encl
		}
		if len(free) > 0 {
			// Recover each injected variable's BOUND from its enclosing class so a flattened inner class
			// renders `<C extends Comparable<?>>` instead of the bare `<C>` -- otherwise a `Range<C>` use
			// (Range requires `C extends Comparable`) fails javac with "type argument C is not within
			// bounds of type-variable C". Bounds default to bare names when no enclosing bound is found
			// (single-class decompile, Object bound, or a bound referencing an out-of-scope variable).
			// Kill-switch JDEC_INNER_TYPEVAR_BOUND_OFF restores the bare-name behavior.
			bounds := c.enclosingTypeParamBounds(free)
			decls := make([]string, len(free))
			for i, n := range free {
				if clause, ok := bounds[n]; ok && clause != "" {
					decls[i] = n + " extends " + clause
				} else {
					decls[i] = n
				}
			}
			classTypeParams = "<" + strings.Join(decls, ", ") + ">"
			classTypeParamNames = free
			// Expose the injected vars' recovered bounds so renderers can tell an unbounded injected `<E>`
			// (safe for an unchecked `X<Object>` -> `X<E>` return cast) from a bounded `<C extends Comparable>`
			// (for which that cast is inconvertible). Only non-Object bounds are present, mirroring the header.
			if c.FuncCtx != nil {
				injected := map[string]string{}
				for _, n := range free {
					if clause, ok := bounds[n]; ok && clause != "" {
						injected[n] = clause
					}
				}
				if len(injected) > 0 {
					c.FuncCtx.InjectedTypeParamBounds = injected
				}
			}
		}
	}
	// RAW-ERASE SET (mirror image of the enclosing-arity injection above, for the case it explicitly
	// leaves out): a flattened NON-STATIC inner class that has its OWN formal type parameters (e.g.
	// `LinkedTreeMap$LinkedTreeMapIterator<T>`) cannot ALSO declare the enclosing class's variables --
	// that would change the arity of its `<ownParam>` reference sites ("wrong number of type arguments").
	// Yet its field/return signatures still mention those enclosing variables (`Node<K,V>`), which would
	// render as undeclared `K`/`V` (javac "cannot find symbol: class K"). Collect exactly those
	// referenced-but-undeclarable variables (every type-variable ref in the class's own supertype + field
	// signatures that is NOT one of its own formal parameters) and mark them for raw-erasure at
	// parameterized type-argument render sites (`Node<K,V>` -> `Node`): legal, runtime-identical, and
	// matching the local var already emitted raw. Derived from THIS class's own bytecode only (no sibling
	// resolver), so it works under single-class decompile too. Kill-switch: JDEC_INNER_RAW_ERASE_OFF.
	var rawEraseTypeVars map[string]bool
	if ownFormalNames := types.ClassFormalTypeParamNames(classSigStr); os.Getenv("JDEC_INNER_RAW_ERASE_OFF") == "" {
		if flags, ok := c.selfInnerClassAccessFlags(); ok && flags&StaticFlag == 0 {
			own := make(map[string]bool, len(ownFormalNames))
			for _, n := range ownFormalNames {
				own[n] = true
			}
			erase := map[string]bool{}
			addErase := func(n string) {
				if n != "" && !own[n] {
					erase[n] = true
				}
			}
			// A flattened non-static inner class that has its OWN formal type parameters cannot ALSO
			// declare the enclosing class's variables (arity of its `<ownParam>` reference sites), so
			// enclosing variables in its supertype + field signatures are raw-erased at render
			// (`Node<K,V>` -> `Node`). Restricted to own-formal classes: a no-own-formal inner class
			// instead gets those variables DECLARED via the enclosing-arity injection above, so erasing
			// them here would clobber legitimate generics.
			if len(ownFormalNames) > 0 {
				if classSigStr != "" {
					for _, n := range types.FreeTypeVarRefsInClassSig(classSigStr) {
						addErase(n)
					}
				}
				for _, field := range c.obj.Fields {
					for _, fattr := range field.Attributes {
						if sa, ok := fattr.(*SignatureAttribute); ok {
							if fs, err := c.obj.getUtf8(sa.SignatureIndex); err == nil && fs != "" {
								for _, n := range types.TypeVarRefsInFieldSig(fs) {
									addErase(n)
								}
							}
							break
						}
					}
				}
			}
			// A flattened inner class can reference enclosing type variables in its METHOD PARAMETER
			// positions -- e.g. spring-core ConcurrentReferenceHashMap$Task<T>.execute(Reference<K,V>,
			// Entry<K,V>, Entries<V>): K/V come from the enclosing map, not from Task's own `<T>`, so they
			// render undeclared ("cannot find symbol: class K"). Raw-erase the PARAMETER positions (return
			// type kept: it does not participate in erasure).
			//
			// Applies to two shapes:
			//   (a) OWN-FORMAL inner classes (Task<T>): K/V are genuinely undeclarable (arity of its
			//       `<T>` reference sites), so erasing fixes "cannot find symbol".
			//   (b) NO-own-formal inner classes whose SUPERCLASS is a flattened `$`-named SIBLING that
			//       itself erased those params (Task's anonymous subclasses $1..$5, extends
			//       ConcurrentReferenceHashMap$Task): they DECLARE K/V via enclosing-arity injection, but
			//       their execute OVERRIDES Task's now-erased execute, so they must erase the same
			//       parameter positions or javac reports "name clash ... same erasure, yet neither
			//       overrides the other".
			// A no-own-formal inner class extending a JDK/library class (e.g.
			// LinkedCaseInsensitiveMap$EntrySet extends AbstractSet, overriding Iterable.forEach(
			// Consumer<? super Entry>)) is EXCLUDED (its super has no `$`), so a genuine JDK-generic
			// override keeps its parameterized parameter and is not broken. Kill-switch
			// JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF.
			eraseMethodParams := len(ownFormalNames) > 0
			// Case (b): a NO-own-formal flattened class whose SUPERCLASS is an own-formal `$`-named
			// SIBLING (same top-level nest) that itself raw-erased its method params via case (a). The
			// subclass DECLARES the enclosing vars (K/V) via enclosing-arity injection and renders its
			// override with parameterized params (`Reference<K,V>`), while the erased base renders raw
			// (`Reference`) -- same erasure, neither overrides -> javac "name clash". Erasing the
			// subclass's override params to match the base restores the override relation. Gated on the
			// super ACTUALLY being own-formal (so it went through case (a)); a no-own-formal `$` super
			// declares+renders those vars generically, so erasing here would instead CREATE a clash.
			if !eraseMethodParams && os.Getenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF") == "" {
				if c.superIsOwnFormalFlattenedSibling() {
					eraseMethodParams = true
				}
			}
			if eraseMethodParams && os.Getenv("JDEC_INNER_RAW_ERASE_METHOD_PARAM_OFF") == "" {
				for _, method := range c.obj.Methods {
					for _, mattr := range method.Attributes {
						if sa, ok := mattr.(*SignatureAttribute); ok {
							if ms, err := c.obj.getUtf8(sa.SignatureIndex); err == nil && ms != "" {
								for _, n := range types.TypeVarRefsInMethodParams(ms) {
									addErase(n)
								}
							}
							break
						}
					}
				}
			}
			if len(erase) > 0 {
				rawEraseTypeVars = erase
			}
		}
	}
	// STANDALONE-ERASE companion of the raw-erase set: the same undeclarable enclosing variables, used
	// as a STANDALONE type (`E nextEntry;`, `T output(K var0, V var1)`, `advanceTo(E var1)`), have no
	// `<...>` to strip, so raw-erase cannot help and they render as undeclared bare names (javac
	// "cannot find symbol: class K"; guava AbstractMapBasedMultimap$Itr, MapMakerInternalMap$HashIterator).
	// Render the variable's JVM ERASURE instead: the raw class of its first bound recovered from the
	// enclosing chain (`E extends InternalEntry<..>` -> InternalEntry, so `nextEntry.getNext()` still
	// resolves), defaulting to java.lang.Object for an unbounded variable (or with no resolver, where a
	// bare undeclared name failed anyway). Runtime-identical, and sibling overrides erase to the same
	// signature so the override relation is preserved. Kill-switch: JDEC_INNER_STANDALONE_ERASE_OFF.
	var standaloneEraseTypeVars map[string]string
	if len(rawEraseTypeVars) > 0 && os.Getenv("JDEC_INNER_STANDALONE_ERASE_OFF") == "" {
		// A variable this class actually DECLARES (own formal params, or enclosing vars injected onto a
		// flattened no-own-formal inner class) is in scope and must NOT be standalone-erased: doing so
		// turns a legitimate `V execute(...)` return into `Object`, breaking the override against the
		// base's `T`(=V) return ("cannot override"). Only genuinely undeclarable enclosing vars (case a,
		// own-formal Task) need standalone erasure. Matters for the case-(b) override-param erasure,
		// whose vars ARE declared here (they are only erased in the override PARAMETER positions).
		declared := map[string]bool{}
		for _, n := range classTypeParamNames {
			declared[n] = true
		}
		bounds := c.enclosingTypeParamErasures(rawEraseTypeVars)
		standaloneEraseTypeVars = make(map[string]string, len(rawEraseTypeVars))
		for name := range rawEraseTypeVars {
			if declared[name] {
				continue
			}
			if e, ok := bounds[name]; ok && e != "" {
				standaloneEraseTypeVars[name] = e
			} else {
				standaloneEraseTypeVars[name] = "java.lang.Object"
			}
		}
	}
	if c.FuncCtx != nil {
		c.FuncCtx.TypeParams = classTypeParamNames
		c.FuncCtx.RawEraseTypeVars = rawEraseTypeVars
		c.FuncCtx.StandaloneEraseTypeVars = standaloneEraseTypeVars
		// ClassTypeParams is the CLASS-only snapshot (never extended with a method's own `<T>` while
		// that method renders); it lets typeVarReturnCast recover `this`'s real parameterization.
		c.FuncCtx.ClassTypeParams = classTypeParamNames
		// Record which same-class fields are declared as a bare class-scope type variable (e.g.
		// `private final K key;`). A store into such a field whose RHS erased to Object/the bound
		// needs an unchecked `(K)` cast to recompile (see AssignStatement.typeVarFieldStoreCast).
		// A type-variable field signature is exactly `T<name>;` (JVMS 4.7.9.1) - matched textually
		// so this never renders a type or touches the import set (no import-order side effects).
		// In the same pass, record PARAMETERIZED field signatures (`Ljava/util/...<...>;`) so a JDK
		// generic method call on a field receiver can recover the receiver's type args
		// (FunctionCallExpression.instantiatedParamType; see FieldSignatures). The signature string
		// is stored verbatim and parsed on demand, so this never renders a type here.
		fieldTypeVars := map[string]string{}
		fieldSignatures := map[string]string{}
		for _, field := range c.obj.Fields {
			name, err := c.obj.getUtf8(field.NameIndex)
			if err != nil || name == "" {
				continue
			}
			for _, attr := range field.Attributes {
				sigAttr, ok := attr.(*SignatureAttribute)
				if !ok {
					continue
				}
				sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
				if err != nil || len(sigStr) < 3 {
					continue
				}
				// Parameterized object field signature (`L...<...>;`): keep verbatim for type-arg
				// recovery at JDK-generic-method call sites on this field receiver.
				if sigStr[0] == 'L' && strings.ContainsRune(sigStr, '<') {
					fieldSignatures[class_context.SafeIdentifier(name)] = sigStr
				}
				if len(classTypeParamNames) == 0 {
					continue
				}
				// Accept `TK;` (bare type var) and `[TK;`/`[[TK;` (array of type var). The stored
				// value is the rendered field type (`K` / `K[]` / `K[][]`) used as the cast target.
				sig := sigStr
				arrayDepth := 0
				for len(sig) > 0 && sig[0] == '[' {
					arrayDepth++
					sig = sig[1:]
				}
				if len(sig) < 3 || sig[0] != 'T' || sig[len(sig)-1] != ';' || strings.ContainsAny(sig, "<>[/") {
					continue
				}
				tv := sig[1 : len(sig)-1]
				if c.FuncCtx.IsTypeParam(tv) {
					fieldTypeVars[class_context.SafeIdentifier(name)] = tv + strings.Repeat("[]", arrayDepth)
				}
			}
		}
		if len(fieldTypeVars) > 0 {
			c.FuncCtx.FieldTypeVars = fieldTypeVars
		}
		if len(fieldSignatures) > 0 {
			c.FuncCtx.FieldSignatures = fieldSignatures
		}
		// Record same-class methods carrying a generic Signature, keyed by (name, arity). A call
		// `this.tailSet(objVal)` on such a method must re-emit the source's `(E)` argument cast that
		// the descriptor erased away (FunctionCallExpression.sameClassMethodParamType). Overloads that
		// collide on (name, arity) are dropped (set to "") so a call never picks the wrong signature.
		methodSignatures := map[string]string{}
		methodSigSeen := map[string]bool{}
		// Descriptor-keyed companion: (name, EXACT descriptor) is unique in the JVM, so unlike the
		// arity-keyed map it keeps same-arity overloads that the arity path must drop as ambiguous, letting
		// a same-class call recover its erased argument cast via its exact descriptor. Kill-switch
		// JDEC_SAMECLASS_DESC_SIG_OFF disables population (so the consumer degrades to the arity path).
		methodSignaturesByDesc := map[string]string{}
		recordByDesc := os.Getenv("JDEC_SAMECLASS_DESC_SIG_OFF") == ""
		for _, m := range c.obj.Methods {
			name, err := c.obj.getUtf8(m.NameIndex)
			// <init>/<clinit> are NOT recorded: a non-static inner class's constructor Signature OMITS the
			// synthetic leading `this$0` (and outer-capture) parameters, so signature-param indices are
			// OFFSET from the descriptor/argument indices -- recovering a `this(...)` self-call's param type
			// by raw index would mis-cast the synthetic enclosing argument (guava TreeBasedTable$TreeRow
			// `this((R) this$0, ...)`). <clinit> has no call site. Both stay skipped.
			if err != nil || name == "" || name == "<init>" || name == "<clinit>" {
				continue
			}
			descriptor, err := c.obj.getUtf8(m.DescriptorIndex)
			if err != nil || descriptor == "" {
				continue
			}
			for _, attr := range m.Attributes {
				sigAttr, ok := attr.(*SignatureAttribute)
				if !ok {
					continue
				}
				sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
				if err != nil || sigStr == "" || !strings.HasPrefix(sigStr, "(") && !strings.HasPrefix(sigStr, "<") {
					continue
				}
				if recordByDesc {
					// (name, descriptor) is unique -- no collision handling needed.
					methodSignaturesByDesc[class_context.MethodDescKey(name, descriptor)] = sigStr
				}
				key := class_context.MethodSigKey(name, len(methodParamFieldDescriptors(descriptor)))
				if methodSigSeen[key] {
					// (name, arity) collision: ambiguous, drop so no call resolves it.
					delete(methodSignatures, key)
					continue
				}
				methodSigSeen[key] = true
				methodSignatures[key] = sigStr
			}
		}
		if len(methodSignaturesByDesc) > 0 {
			c.FuncCtx.MethodSignaturesByDesc = methodSignaturesByDesc
		}
		// Augment with DIRECT-supertype (inherited) generic method signatures so a `this.m(objVal)`
		// call to an inherited generic method recovers its erased `(K)` argument cast too. Same-class
		// signatures (already in methodSignatures/methodSigSeen) always win. Kill-switch
		// JDEC_GENERIC_SUPER_METHOD_OFF.
		if os.Getenv("JDEC_GENERIC_SUPER_METHOD_OFF") == "" {
			c.collectInheritedThisMethodSignatures(classSigStr, classTypeParamNames, methodSignatures, methodSigSeen)
		}
		if len(methodSignatures) > 0 {
			c.FuncCtx.MethodSignatures = methodSignatures
		}
		// Record same-class CONSTRUCTOR generic signatures keyed by argument count, for the `this(...)`
		// self-call wildcard argument cast (FunctionCallExpression.ctorWildcardArgCast). Kept SEPARATE
		// from methodSignatures (which deliberately skips <init>) so the general same-class method path is
		// untouched. OFFSET GUARD: a non-static inner class's constructor Signature omits the synthetic
		// leading this$0/outer-capture parameters, so its signature-param count is LESS than the descriptor
		// (= call argument) count; recording it would mis-index arguments (guava TreeBasedTable$TreeRow).
		// Only record when the two counts MATCH (top-level / static-nested constructors). Overloads
		// colliding on arity are dropped. Kill-switch JDEC_CTOR_WILDCARD_CAST_OFF.
		if os.Getenv("JDEC_CTOR_WILDCARD_CAST_OFF") == "" {
			ctorSignatures := map[int]string{}
			ctorSeen := map[int]bool{}
			for _, m := range c.obj.Methods {
				name, err := c.obj.getUtf8(m.NameIndex)
				if err != nil || name != "<init>" {
					continue
				}
				descriptor, err := c.obj.getUtf8(m.DescriptorIndex)
				if err != nil || descriptor == "" {
					continue
				}
				descArgc := len(methodParamFieldDescriptors(descriptor))
				for _, attr := range m.Attributes {
					sigAttr, ok := attr.(*SignatureAttribute)
					if !ok {
						continue
					}
					sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
					if err != nil || sigStr == "" || (!strings.HasPrefix(sigStr, "(") && !strings.HasPrefix(sigStr, "<")) {
						continue
					}
					_, sigParams, _ := types.ParseMethodSignatureFull(sigStr, c.FuncCtx)
					if len(sigParams) != descArgc {
						// Synthetic-parameter offset (non-static inner class) -> cannot align by index.
						continue
					}
					if ctorSeen[descArgc] {
						delete(ctorSignatures, descArgc)
						continue
					}
					ctorSeen[descArgc] = true
					ctorSignatures[descArgc] = sigStr
				}
			}
			if len(ctorSignatures) > 0 {
				c.FuncCtx.ConstructorSignatures = ctorSignatures
			}
		}
		// Seed the unified cross-class generic resolver (types.ResolveInstantiatedParamType): record this
		// class's own class Signature and install a lazy sibling-signature provider so a call on ANY
		// jar-internal receiver (this / local / field of a parameterized type) can recover its erased
		// argument cast by walking the receiver's generic supertype hierarchy with proper type-argument
		// substitution -- the root-cause generalization of the same-class / identity-one-level special
		// cases. Only wired on the jar / DecompileWithResolver path (foldSiblingResolver != nil).
		c.FuncCtx.ClassSig = classSigStr
		c.FuncCtx.SiblingClassSig = c.buildSiblingClassSig()
		c.FuncCtx.SiblingSuperTypes = c.buildSiblingSuperTypes()
		c.FuncCtx.SiblingCtorSig = c.buildSiblingCtorSig()
		c.FuncCtx.SiblingFieldSig = c.buildSiblingFieldSig()
	}
	packageSource := fmt.Sprintf("package %s;\n\n", packageName)
	if className == "" {
		return "", utils.Error("className is empty")
	}

	annoStrs := []string{}
	for _, info := range lo.Filter(c.obj.Attributes, func(item AttributeInfo, index int) bool {
		_, ok := item.(*RuntimeVisibleAnnotationsAttribute)
		return ok
	}) {
		for _, annotation := range info.(*RuntimeVisibleAnnotationsAttribute).Annotations {
			res, err := c.DumpAnnotation(annotation)
			if err != nil {
				return "", utils.Wrap(err, "DumpAnnotation failed")
			}
			annoStrs = append(annoStrs, res)
		}
	}
	methods, err := c.DumpMethods()
	if err != nil {
		return "", utils.Wrap(err, "DumpMethods failed")
	}
	fields, err := c.DumpFields()
	if err != nil {
		return "", utils.Wrap(err, "DumpFields failed")
	}
	// Enum constant-body cross-class folding: when a multi-class resolver is available, recover each
	// constant's synthetic `Outer$N` subclass body and inline it as `CONST { ...body... }`. Computed
	// once (before assemble, which may run twice on the degradation path) so required imports are
	// merged into funcCtx ahead of the import-assembly step below. Empty (nil) on the single-class
	// path, leaving the constant render hook untouched.
	enumConstantBodies := c.foldEnumConstantBodies(isEnum)
	var classKeyword string
	if !nonClassKeyword {
		classKeyword = " class"
	}
	// assemble renders the full compilation unit from the current methods/fields. It is a
	// closure so the syntax safety net can re-render after degrading malformed members.
	assemble := func() string {
		// strings.Builder instead of `attrs += ...`: a class with many methods otherwise
		// triggers O(n^2) string concatenation (each += re-copies the whole accumulated
		// body), which profiling flagged as a top dumper allocator. The builder produces
		// the exact same bytes in O(n).
		var attrsB strings.Builder
		if len(fields) > 0 {
			attrsB.WriteString("\n\t// Fields\n")
			enumFields := make([]dumpedFields, 0, len(fields))
			ordinaryFields := make([]string, 0, len(fields))
			for _, field := range fields {
				if isEnum && field.typeName == className && (field.modifier == "public static final enum" || field.modifier == "public static final") {
					enumFields = append(enumFields, field)
					continue
				}
				ordinaryFields = append(ordinaryFields, field.code)
			}
			for idx, enumSimple := range enumFields {
				constStr := enumSimple.fieldName
				if args := c.enumConstantArgs(enumSimple.fieldName); args != "" {
					constStr += "(" + args + ")"
				}
				attrsB.WriteString("\t")
				attrsB.WriteString(constStr)
				if body := enumConstantBodies[enumSimple.fieldName]; body != "" {
					attrsB.WriteString(body)
				}
				if idx == len(enumFields)-1 {
					attrsB.WriteString(";\n")
				} else {
					attrsB.WriteString(",\n")
				}
			}
			if isEnum && len(enumFields) == 0 && (len(ordinaryFields) > 0 || len(methods) > 0) {
				// Java requires a separator before enum body declarations when the constant list is empty:
				// `enum E { ; int x; }`.
				attrsB.WriteString("\t;\n")
			}
			for _, ordinaryField := range ordinaryFields {
				attrsB.WriteString("\t")
				attrsB.WriteString(ordinaryField)
				attrsB.WriteString("\n")
			}
		}
		if isEnum && len(fields) == 0 && len(methods) > 0 {
			attrsB.WriteString("\n\t;\n")
		}
		if len(methods) > 0 {
			attrsB.WriteString("\n")
			for _, method := range methods {
				attrsB.WriteString("\t")
				attrsB.WriteString(method.code)
				attrsB.WriteString("\n")
			}
		}
		attrs := attrsB.String()
		result := fmt.Sprintf("%s%s %s%s%s {%s}", accessFlags, classKeyword, className, classTypeParams, superStr, attrs)
		if len(annoStrs) > 0 {
			result = fmt.Sprintf("%s\n%s", strings.Join(annoStrs, "\n"), result)
		}
		importsStr := ""
		for _, s := range funcCtx.GetAllImported() {
			if utils.StringSliceContain(buildInLib, s) {
				continue
			}
			// Import spelling is already normalized by GetAllImported per type kind:
			//   - EXTERNAL stdlib nested type -> reduced to the OUTER class (java.util.Map), no '$';
			//     the body renders the dotted Outer.Inner source spelling against that import.
			//   - SAME-JAR Yak flat unit -> kept as the flat `pkg.Outer$Inner` name; the body renders
			//     the matching flat `Outer$Inner` reference and the import resolves to the sibling
			//     flat unit `Outer$Inner.java` ('$' is a legal identifier char, so `import a.b.C$D;`
			//     is valid Java - verified). Rewriting '$'->'.' here (legacy behaviour, on the false
			//     premise that imports cannot carry '$') turned it into `import pkg.Outer.Inner;`, which
			//     does NOT resolve because Yak's flat `Outer.java` has no nested `Inner` (it is a
			//     separate flat unit) - the second-largest fastjson2 cross-package recompile blocker.
			// So emit the (already correct) import string verbatim.
			importsStr += fmt.Sprintf("import %s;\n", s)
		}
		if len(importsStr) > 0 {
			importsStr += "\n"
		}
		return packageSource + importsStr + result
	}

	full := assemble()
	if EnableDecompileSyntaxValidation && len(full) < 50000 {
		if err := validateJavaSyntax(full); err != nil {
			// The assembled class is not valid Java. Degrade malformed members (using the real
			// class header so interface/enum/constructor context is honored) and re-render, so a
			// single broken method/field cannot make the whole class un-parseable.
			header := fmt.Sprintf("%s%s %s%s", accessFlags, classKeyword, className, superStr)
			methods = c.degradeInvalidMethods(header, methods)
			fields = c.degradeInvalidFields(header, className, isEnum, fields)
			full = assemble()
			if err := validateJavaSyntax(full); err != nil && !isDollarIdentifierValidatorGap(full, err) {
				log.Warnf("decompiled class %s still has syntax errors after degradation: %v", c.ClassName, err)
			}
		}
	}
	// Enum-switch ($SwitchMap) cross-class fold (Bug V): rewrite `switch(Outer$N.$SwitchMap$E[sel.
	// ordinal()])` back to the idiomatic `switch(sel){ case CONST: ... }`. No-op without a resolver
	// or when JDEC_NO_ENUM_SWITCH_FOLD is set; produces valid Java, so it runs after assembly.
	full = c.foldEnumSwitchMaps(full)
	// Run the split-slot definite-assignment init on the FULL class text (all methods merged).
	// This overcomes the chunked-sourceCode limitation where the per-method init couldn't see
	// assignments in other method blocks. Kill-switch: JDEC_INIT_PROX_SPLIT_OFF=1.
	full = initProximateSplitSlotDecl(full)
	// Fix try/catch structuring: move exception-throwing calls that are rendered outside a
	// try block INTO the nearest inner try body. Kill-switch: JDEC_FIX_TRYCATCH_OFF=1.
	full = fixTryCatchExceptionPlacement(full)
	return full, nil
}

type dumpedFields struct {
	code      string
	fieldName string
	modifier  string
	typeName  string
}

func (c *ClassObjectDumper) DumpFields() ([]dumpedFields, error) {
	genuineEnum := c.isGenuineEnum()
	fields := make([]dumpedFields, 0, len(c.obj.Fields))
	for _, field := range c.obj.Fields {
		accessFlagsVerbose, accessCode := getFieldAccessFlagsVerbose(field.AccessFlags)
		//if len(accessFlagsVerbose) < 1 {
		//	return nil, utils.Error("fields accessFlagsVerbose is empty")
		//}
		_ = accessFlagsVerbose
		accessFlags := accessCode
		name, err := c.obj.getUtf8(field.NameIndex)
		if err != nil {
			return nil, err
		}
		renderName := class_context.SafeIdentifier(name)
		// $VALUES is the synthetic array backing values(); javac re-synthesizes it.
		if genuineEnum && name == "$VALUES" {
			continue
		}
		descriptor, err := c.obj.getUtf8(field.DescriptorIndex)
		if err != nil {
			return nil, err
		}
		fieldType, err := types.ParseDescriptor(descriptor)
		if err != nil {
			return nil, err
		}

		// Scan the field attributes once to find the Signature (generic) attribute before
		// rendering the type. When a parseable generic signature is present, it overrides the
		// descriptor-derived type to recover erased generics (e.g. List<String> instead of List).
		for _, attr := range field.Attributes {
			if sigAttr, ok := attr.(*SignatureAttribute); ok {
				if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
					if sigType := types.ParseSignature(sigStr); sigType != nil {
						fieldType = sigType
					}
				}
			}
		}

		// fieldType.String already registers the needed imports and shortens (or
		// FQN-disambiguates) every class component via ShortTypeName, so the rendered
		// string is the final field type. Re-running Import/ShortTypeName on that whole
		// string corrupts parameterized, array, or primitive-array types.
		lastPacket := fieldType.String(c.FuncCtx)
		valueLiteral := ""
		for _, attr := range field.Attributes {
			switch ret := attr.(type) {
			case *ConstantValueAttribute:
				value, err := c.obj.getConstantInfo(ret.ConstantValueIndex)
				if err != nil {
					log.Errorf("getConstantInfo(%d) failed", ret.ConstantValueIndex)
					continue
				}
				switch constVal := value.(type) {
				case *ConstantStringInfo:
					constStr, _ := c.obj.getUtf8(constVal.StringIndex)
					valueLiteral = values.JavaStringToLiteral(constStr)
				case *ConstantIntegerInfo:
					// boolean/char are stored as int constants in the pool; render them
					// in their declared type so the field initializer type-checks
					// (e.g. `boolean B = true` instead of the illegal `boolean B = 1`).
					switch fieldType.String(c.FuncCtx) {
					case types.NewJavaPrimer(types.JavaBoolean).String(c.FuncCtx):
						if constVal.Value == 0 {
							valueLiteral = "false"
						} else {
							valueLiteral = "true"
						}
					default:
						valueLiteral = strconv.Itoa(int(constVal.Value))
					}
				case *ConstantLongInfo:
					valueLiteral = strconv.Itoa(int(constVal.Value))
					if !strings.HasSuffix(valueLiteral, "L") {
						valueLiteral += "L"
					}
				case *ConstantFloatInfo:
					valueLiteral = javaFloatLiteral(constVal.Value)
				case *ConstantDoubleInfo:
					valueLiteral = javaDoubleLiteral(constVal.Value)
				default:
					log.Errorf("when handling for fields unknown constant type: %T", constVal)
				}
			case *SyntheticAttribute:
			// synthetic (compiler-generated) field marker; no diagnostic needed
			case *DeprecatedAttribute:
			// log.Infof("field %s is deprecated", name)
			case *SignatureAttribute:
			case *UnparsedAttribute:
				// Silently ignore unrecognized attributes (RuntimeInvisibleTypeAnnotations,
				// PermittedSubclasses, Record, NestMembers, etc.) rather than flooding logs.
			case *RuntimeVisibleAnnotationsAttribute:

			default:
				// Silently ignore unknown attribute types on fields.
			}
		}

		if valueLiteral != "" {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, valueLiteral),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		} else if slices.Contains(accessFlagsVerbose, "final") && c.fieldDefaultValue[name] != "" {
			// A final field with a captured, hoistable initializer (constant-folded value
			// or a parameter-independent <init>/<clinit> assignment). Emit it inline.
			dumped := dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, c.fieldDefaultValue[name]),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			}

			fields = append(fields, dumped)
		} else if c.isInterfaceLike() && slices.Contains(accessFlagsVerbose, "static") && slices.Contains(accessFlagsVerbose, "final") {
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s = %s;", accessFlags, lastPacket, renderName, defaultInitializerForFieldType(lastPacket)),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		} else {
			// No initializer to emit (incl. blank finals assigned in the constructor /
			// static block). A bogus `= 0` here would be illegal for reference types and is
			// unnecessary: definite assignment in <init>/<clinit> keeps blank finals valid.
			fields = append(fields, dumpedFields{
				code:      fmt.Sprintf("%s %s %s;", accessFlags, lastPacket, renderName),
				fieldName: renderName,
				modifier:  accessFlags,
				typeName:  lastPacket,
			})
		}
	}
	return fields, nil
}

func defaultInitializerForFieldType(typeName string) string {
	switch strings.TrimSpace(typeName) {
	case "boolean":
		return "false"
	case "byte":
		return "(byte)0"
	case "char":
		return "(char)0"
	case "short":
		return "(short)0"
	case "int":
		return "0"
	case "long":
		return "0L"
	case "float":
		return "0.0F"
	case "double":
		return "0.0D"
	default:
		return "null"
	}
}

// enclosingTypeParamBounds recovers, for each requested injected free type-variable name, the bound
// declared on the NEAREST enclosing class. A non-static inner class inherits its enclosing scope's type
// variables; when Yak flattens it to a top-level `Outer$Inner` unit those variables are re-declared on
// the flat class (see the JDEC_INNER_TYPEVAR injection) but, until now, with a bare `Object` bound. That
// drops the real bound, so a use like `Range<C>` (Range requires `C extends Comparable`) fails javac
// with "type argument C is not within bounds of type-variable C". This walks the binary-name `$` chain
// (Outer$Mid$Inner -> Outer$Mid -> Outer) via foldSiblingResolver, parses each enclosing class's formal
// type-parameter bounds, and returns name -> rendered bound clause.
//
// SAFETY: a bound is returned ONLY when (a) the enclosing declaration has a real (non-Object) bound and
// (b) every type variable the bound references is itself in the injected free set, so the reconstructed
// `<C extends Comparable<?>>` can never reference an undeclared variable. The nearest enclosing scope
// wins (Java type-variable shadowing). Returns an empty map when no cross-class resolver is available
// (single-class decompile) or under kill-switch JDEC_INNER_TYPEVAR_BOUND_OFF, so the bare-name behavior
// is unchanged there. The arity at reference sites is untouched (only the declaration gains `extends`),
// so this never introduces "wrong number of type arguments".
func (c *ClassObjectDumper) enclosingTypeParamBounds(free []string) map[string]string {
	res := map[string]string{}
	if c.foldSiblingResolver == nil || os.Getenv("JDEC_INNER_TYPEVAR_BOUND_OFF") != "" {
		return res
	}
	freeSet := map[string]bool{}
	for _, n := range free {
		freeSet[n] = true
	}
	binName := strings.ReplaceAll(c.obj.GetClassName(), ".", "/")
	for {
		idx := strings.LastIndexByte(binName, '$')
		if idx < 0 {
			break
		}
		binName = binName[:idx]
		data, ok := c.foldSiblingResolver(binName)
		if !ok || len(data) == 0 {
			continue
		}
		sObj, err := Parse(data)
		if err != nil {
			continue
		}
		sig := ""
		for _, attr := range sObj.Attributes {
			if sa, ok := attr.(*SignatureAttribute); ok {
				if s, err := sObj.getUtf8(sa.SignatureIndex); err == nil {
					sig = s
				}
				break
			}
		}
		if sig == "" {
			continue
		}
		for name, b := range types.ClassFormalTypeParamBounds(sig, c.FuncCtx) {
			if !freeSet[name] {
				continue
			}
			if _, done := res[name]; done {
				continue // a nearer enclosing scope already bound it (shadowing)
			}
			if b.Clause == "" {
				continue
			}
			inScope := true
			for _, r := range b.Refs {
				if !freeSet[r] {
					inScope = false
					break
				}
			}
			if inScope {
				res[name] = b.Clause
			}
		}
	}
	return res
}

// enclosingTypeParamErasures recovers, for each requested undeclarable enclosing type-variable name, the
// ERASURE of its bound declared on the NEAREST enclosing class that declares it: the raw dotted class of
// its first class/interface bound (`E extends InternalEntry<..>` -> "com.google.common.collect...
// InternalEntry"). It is the standalone-position companion of enclosingTypeParamBounds (which recovers
// the source bound CLAUSE for a re-DECLARED inner parameter); here the erasure head is what a flattened
// inner class must render for the variable used as a STANDALONE type (`E nextEntry;`, `advanceTo(E)`),
// since it cannot declare the variable. Walks the binary-name `$` chain via foldSiblingResolver; the
// nearest enclosing scope that binds a name wins (type-variable shadowing). Returns an empty map with no
// resolver (single-class decompile), so the caller defaults every requested name to java.lang.Object.
func (c *ClassObjectDumper) enclosingTypeParamErasures(vars map[string]bool) map[string]string {
	res := map[string]string{}
	if c.foldSiblingResolver == nil || len(vars) == 0 {
		return res
	}
	seen := map[string]bool{}
	binName := strings.ReplaceAll(c.obj.GetClassName(), ".", "/")
	for {
		idx := strings.LastIndexByte(binName, '$')
		if idx < 0 {
			break
		}
		binName = binName[:idx]
		data, ok := c.foldSiblingResolver(binName)
		if !ok || len(data) == 0 {
			continue
		}
		sObj, err := Parse(data)
		if err != nil {
			continue
		}
		sig := ""
		for _, attr := range sObj.Attributes {
			if sa, ok := attr.(*SignatureAttribute); ok {
				if s, err := sObj.getUtf8(sa.SignatureIndex); err == nil {
					sig = s
				}
				break
			}
		}
		if sig == "" {
			continue
		}
		// Shadowing: the NEAREST enclosing scope that DECLARES a name binds it, even when that
		// declaration is unbounded (erasure Object, i.e. no res entry) -- a farther scope's bound
		// must not leak through. Track declared-and-resolved names separately from res.
		erasures := types.ClassFormalTypeParamErasures(sig)
		for _, name := range types.ClassFormalTypeParamNames(sig) {
			if !vars[name] || seen[name] {
				continue
			}
			seen[name] = true
			if e, ok := erasures[name]; ok {
				res[name] = e
			}
		}
	}
	return res
}

// enclosingFormalTypeParamsForArity recovers the formal type-parameter NAMES of the NEAREST generic
// enclosing class, for a flattened NON-STATIC inner class that itself declares and uses NO type
// variables. Such a class injects nothing via the usage scan, so its flattened declaration has zero
// parameters; yet its reference sites carry the enclosing class's type arguments (a non-static inner's
// generic signatures encode `LOuter<TT;>.Inner;`, which parseSigClassType keeps on the flattened name).
// Re-declaring the enclosing formal parameters makes declaration and reference arities agree (gson
// TreeTypeAdapter$GsonContextImpl: declared `class ...GsonContextImpl`, referenced `...GsonContextImpl<T>`).
//
// SAFETY: returns nil unless (a) a cross-class resolver is available, (b) this class is a NON-STATIC
// inner per its own InnerClasses access flags (only those capture enclosing type variables at reference
// sites; static nested classes do not), and (c) some enclosing class on the binary-name `$` chain
// declares at least one formal type parameter. The NEAREST such enclosing class wins, matching
// parseSigClassType (which keeps the innermost non-empty argument list of `LOuter<..>.Mid<..>.Inner;`).
// The injected names may be unused, which is legal; raw reference sites stay valid; an injected arity
// always equals what reference signatures carry, so this cannot create "wrong number of type arguments".
// Kill-switch: JDEC_INNER_ENCLOSING_ARITY_OFF. No-op under single-class decompile (no resolver).
// typeNamesSubset reports whether every name in sub appears in super_ (set containment). Used to gate
// the enclosing-arity reconciliation: only when the inner class's used type-variable set is a subset of
// the nearest enclosing class's formal parameters is it safe to adopt that full ordered set (a used
// variable from a deeper enclosing scope or a method would NOT be covered, so those stay on the
// usage-based path). An empty sub is trivially a subset.
func typeNamesSubset(sub, super_ []string) bool {
	set := make(map[string]bool, len(super_))
	for _, n := range super_ {
		set[n] = true
	}
	for _, n := range sub {
		if !set[n] {
			return false
		}
	}
	return true
}

func (c *ClassObjectDumper) enclosingFormalTypeParamsForArity() []string {
	if c.foldSiblingResolver == nil || os.Getenv("JDEC_INNER_ENCLOSING_ARITY_OFF") != "" {
		return nil
	}
	flags, ok := c.selfInnerClassAccessFlags()
	if !ok || flags&StaticFlag != 0 {
		return nil
	}
	binName := strings.ReplaceAll(c.obj.GetClassName(), ".", "/")
	for {
		idx := strings.LastIndexByte(binName, '$')
		if idx < 0 {
			break
		}
		binName = binName[:idx]
		data, ok := c.foldSiblingResolver(binName)
		if !ok || len(data) == 0 {
			continue
		}
		sObj, err := Parse(data)
		if err != nil {
			continue
		}
		sig := ""
		for _, attr := range sObj.Attributes {
			if sa, ok := attr.(*SignatureAttribute); ok {
				if s, err := sObj.getUtf8(sa.SignatureIndex); err == nil {
					sig = s
				}
				break
			}
		}
		if sig == "" {
			continue
		}
		if names := types.ClassFormalTypeParamNames(sig); len(names) > 0 {
			return names
		}
	}
	return nil
}

// genericSupertypeRawName returns the raw dotted class name of a (possibly generic) supertype type
// recovered from a class Signature, so it can be matched against the erased super_class / Interfaces
// names. Returns "" for type variables or anything that is not a class/parameterized type.
func genericSupertypeRawName(t types.JavaType) string {
	if t == nil {
		return ""
	}
	switch r := t.RawType().(type) {
	case *types.JavaParameterizedType:
		return r.RawClassName
	case *types.JavaClass:
		return r.Name
	}
	return ""
}

// collectInheritedThisMethodSignatures augments the same-class MethodSignatures table with the generic
// method signatures of a class's DIRECT supertypes (its superclass + directly-implemented interfaces),
// so a `this.get(objVal)` call to a method DECLARED on a supertype (not the current class) can still
// recover the source's `(K)` argument cast that the descriptor erased away (guava AbstractLoadingCache
// `this.get(k)` from interface LoadingCache<K,V>; the `Object cannot be converted to K` family).
//
// SAFETY (identity-mapping only): a supertype's method signature is copied VERBATIM into the current
// class's table only when the current class instantiates that supertype with the IDENTITY type-argument
// mapping -- i.e. `Sub<K,V> implements Super<K,V>` where each argument is a bare type variable whose
// name equals the supertype's corresponding formal parameter name AND is itself a current-class type
// variable. Under identity the supertype method's `(TK;)` uses the very same name K that is in scope on
// the current class, so the verbatim copy is sound and sameClassMethodParamType's `IsTypeParam` check
// remains the cast gate. A renamed/reordered/concrete instantiation (`Sub<X> implements Super<X,String>`)
// is NOT identity and is skipped -- copying verbatim there could cast to the wrong type variable. JDK /
// external supertypes are not in the jar (foldSiblingResolver misses) and are handled by the
// InstantiateJDKMethodParam path instead, so this only ever augments jar-internal supertypes. Only
// DIRECT supertypes are walked (one level); deeper chains are a documented residual. Collisions on
// (name, arity) across supertypes are dropped (set unresolvable) so no call binds the wrong signature.
// Kill-switch JDEC_GENERIC_SUPER_METHOD_OFF.
func (c *ClassObjectDumper) collectInheritedThisMethodSignatures(classSigStr string, classTypeParamNames []string, out map[string]string, seen map[string]bool) {
	if c.foldSiblingResolver == nil || classSigStr == "" || len(classTypeParamNames) == 0 {
		return
	}
	isCurrentTypeVar := func(name string) bool {
		for _, p := range classTypeParamNames {
			if p == name {
				return true
			}
		}
		return false
	}
	sup, ifaces := types.ParseClassSignatureSupers(classSigStr)
	supertypes := make([]types.JavaType, 0, len(ifaces)+1)
	if sup != nil {
		supertypes = append(supertypes, sup)
	}
	supertypes = append(supertypes, ifaces...)
	// Collect inherited signatures into a local map first so the same-class entries in `out`/`seen`
	// are never clobbered (same-class always wins). Keys colliding across multiple supertypes are
	// dropped as ambiguous.
	inherited := map[string]string{}
	dropped := map[string]bool{}
	for _, st := range supertypes {
		// Only a parameterized supertype carries the type arguments needed for the identity check; a
		// raw supertype (no `<...>`) provides no mapping and is skipped.
		pt, ok := st.RawType().(*types.JavaParameterizedType)
		if !ok || len(pt.TypeArgs) == 0 {
			continue
		}
		rawName := pt.RawClassName
		if rawName == "" {
			continue
		}
		data, ok := c.foldSiblingResolver(strings.ReplaceAll(rawName, ".", "/"))
		if !ok || len(data) == 0 {
			continue // JDK / external supertype not in this jar (covered by InstantiateJDKMethodParam)
		}
		sObj, err := Parse(data)
		if err != nil {
			continue
		}
		supSig := ""
		for _, attr := range sObj.Attributes {
			if sa, ok := attr.(*SignatureAttribute); ok {
				if s, err := sObj.getUtf8(sa.SignatureIndex); err == nil {
					supSig = s
				}
				break
			}
		}
		formalParams := types.ClassFormalTypeParamNames(supSig)
		if len(formalParams) == 0 || len(formalParams) != len(pt.TypeArgs) {
			continue
		}
		// Identity check: every type argument must be a bare type variable whose name equals the
		// supertype's corresponding formal parameter AND is a current-class type variable.
		identity := true
		for i, ta := range pt.TypeArgs {
			jc, ok := ta.RawType().(*types.JavaClass)
			if !ok || jc.Name != formalParams[i] || !isCurrentTypeVar(jc.Name) {
				identity = false
				break
			}
		}
		if !identity {
			continue
		}
		for _, m := range sObj.Methods {
			name, err := sObj.getUtf8(m.NameIndex)
			if err != nil || name == "" || name == "<init>" || name == "<clinit>" {
				continue
			}
			descriptor, err := sObj.getUtf8(m.DescriptorIndex)
			if err != nil || descriptor == "" {
				continue
			}
			for _, attr := range m.Attributes {
				sigAttr, ok := attr.(*SignatureAttribute)
				if !ok {
					continue
				}
				sigStr, err := sObj.getUtf8(sigAttr.SignatureIndex)
				if err != nil || sigStr == "" || (!strings.HasPrefix(sigStr, "(") && !strings.HasPrefix(sigStr, "<")) {
					continue
				}
				key := class_context.MethodSigKey(name, len(methodParamFieldDescriptors(descriptor)))
				if seen[key] {
					continue // declared on the current class: same-class signature wins
				}
				if dropped[key] {
					continue
				}
				if _, exists := inherited[key]; exists {
					// Same (name, arity) on two supertypes: ambiguous, drop so no call binds it.
					delete(inherited, key)
					dropped[key] = true
					continue
				}
				inherited[key] = sigStr
			}
		}
	}
	for k, v := range inherited {
		out[k] = v
		seen[k] = true
	}
}

// buildSiblingClassSig returns a lazy, cached provider of a jar-internal class's generic signature info
// (class Signature + (name,arity)->method Signature map) keyed by binary internal name, for the unified
// cross-class generic resolver (types.ResolveInstantiatedParamType, consumed in
// FunctionCallExpression.resolvedParamType). It reuses the same byte resolver + parse + Signature
// extraction that collectInheritedThisMethodSignatures uses, but exposes ARBITRARY classes (not just
// identity-mapped direct supertypes), so the resolver can walk a full hierarchy with real type-argument
// substitution. Returns nil when no cross-class resolver is available (single-class decompile), which
// disables the resolver walk (callers fall back to the JDK table / same-class paths). Caching keeps the
// per-class-dump cost bounded and the result deterministic (a nil cache entry records a confirmed miss).
func (c *ClassObjectDumper) buildSiblingClassSig() func(internalName string) (string, map[string]string, bool) {
	if c.foldSiblingResolver == nil {
		return nil
	}
	type entry struct {
		classSig   string
		methodSigs map[string]string
		ok         bool
	}
	cache := map[string]*entry{}
	resolver := c.foldSiblingResolver
	return func(internal string) (string, map[string]string, bool) {
		if e, hit := cache[internal]; hit {
			return e.classSig, e.methodSigs, e.ok
		}
		e := &entry{}
		cache[internal] = e
		data, ok := resolver(internal)
		if !ok || len(data) == 0 {
			return "", nil, false // JDK / external: not in jar
		}
		sObj, err := Parse(data)
		if err != nil {
			return "", nil, false
		}
		for _, attr := range sObj.Attributes {
			if sa, ok := attr.(*SignatureAttribute); ok {
				if s, err := sObj.getUtf8(sa.SignatureIndex); err == nil {
					e.classSig = s
				}
				break
			}
		}
		methodSigs := map[string]string{}
		methodSeen := map[string]bool{}
		for _, m := range sObj.Methods {
			name, err := sObj.getUtf8(m.NameIndex)
			if err != nil || name == "" || name == "<init>" || name == "<clinit>" {
				continue
			}
			descriptor, err := sObj.getUtf8(m.DescriptorIndex)
			if err != nil || descriptor == "" {
				continue
			}
			for _, attr := range m.Attributes {
				sigAttr, ok := attr.(*SignatureAttribute)
				if !ok {
					continue
				}
				sigStr, err := sObj.getUtf8(sigAttr.SignatureIndex)
				if err != nil || sigStr == "" || (!strings.HasPrefix(sigStr, "(") && !strings.HasPrefix(sigStr, "<")) {
					continue
				}
				// Descriptor-keyed entry: UNIQUE per overload, so it survives the arity-key
				// collision drop below and lets an exact-overload lookup (by the call's descriptor)
				// recover the right Signature. Canonical need: SortedLists.binarySearch has two
				// 5-arg overloads (`(List,E,KPB,KAB)` erasure-collides with `(List,Function,K,KPB,KAB)`)
				// whose shared arity key is dropped, leaving calleeParamIsErasedTypeVar unable to see
				// that the 3rd formal is the type variable K -> the spurious `(Comparable)` cast that
				// breaks binarySearch's K inference (guava ImmutableRangeMap/ImmutableRangeSet).
				methodSigs[class_context.MethodDescKey(name, descriptor)] = sigStr
				key := class_context.MethodSigKey(name, len(methodParamFieldDescriptors(descriptor)))
				if methodSeen[key] {
					// (name, arity) collision (overload): ambiguous, drop so no arity-keyed call binds it.
					delete(methodSigs, key)
					continue
				}
				methodSeen[key] = true
				methodSigs[key] = sigStr
			}
		}
		e.methodSigs = methodSigs
		e.ok = true
		return e.classSig, e.methodSigs, true
	}
}

// buildSiblingCtorSig returns a lazy, cached provider of a jar-internal class's CONSTRUCTOR generic
// Signature keyed by DESCRIPTOR argument count. It complements buildSiblingClassSig (which deliberately
// skips <init>): a `super(...)` call needs the superclass ctor's parameter types WITH type variables
// (e.g. `(BaseGraph<TN;>;TN;)V`) to know which arguments feed a bare type-variable parameter, so the
// erased `(N)` cast can be re-emitted. Only ctors whose Signature parameter count EQUALS the descriptor
// parameter count are recorded (a non-static inner class's ctor Signature omits the synthetic leading
// this$0, so a count mismatch would mis-index; skip it). Overloads colliding on argument count are
// dropped. Returns nil when no cross-class resolver is available (single-class decompile).
func (c *ClassObjectDumper) buildSiblingCtorSig() func(internalName string, argc int) (string, bool) {
	if c.foldSiblingResolver == nil {
		return nil
	}
	type entry struct {
		ctorSigs map[int]string
		ok       bool
	}
	cache := map[string]*entry{}
	resolver := c.foldSiblingResolver
	return func(internal string, argc int) (string, bool) {
		e, hit := cache[internal]
		if !hit {
			e = &entry{}
			cache[internal] = e
			data, ok := resolver(internal)
			if ok && len(data) > 0 {
				if sObj, err := Parse(data); err == nil {
					ctorSigs := map[int]string{}
					seen := map[int]bool{}
					for _, m := range sObj.Methods {
						name, err := sObj.getUtf8(m.NameIndex)
						if err != nil || name != "<init>" {
							continue
						}
						descriptor, err := sObj.getUtf8(m.DescriptorIndex)
						if err != nil || descriptor == "" {
							continue
						}
						descArgc := len(methodParamFieldDescriptors(descriptor))
						for _, attr := range m.Attributes {
							sigAttr, ok := attr.(*SignatureAttribute)
							if !ok {
								continue
							}
							sigStr, err := sObj.getUtf8(sigAttr.SignatureIndex)
							if err != nil || sigStr == "" || !strings.HasPrefix(sigStr, "(") {
								continue
							}
							// Offset safety: a non-static inner class's ctor Signature omits the synthetic
							// leading this$0/outer-capture params, so its param count is smaller than the
							// descriptor's. Only record when they match, so the index used by the caller
							// (which indexes descriptor arguments) lines up with the Signature params.
							sigParams, _ := types.ParseMethodSignature(sigStr)
							if len(sigParams) != descArgc {
								continue
							}
							if seen[descArgc] {
								delete(ctorSigs, descArgc)
								continue
							}
							seen[descArgc] = true
							ctorSigs[descArgc] = sigStr
						}
					}
					e.ctorSigs = ctorSigs
					e.ok = true
				}
			}
		}
		if !e.ok || e.ctorSigs == nil {
			return "", false
		}
		sig, ok := e.ctorSigs[argc]
		return sig, ok
	}
}

// buildSiblingFieldSig returns a lazy, cached provider of a jar-internal class's FIELD generic Signature
// keyed by field name. It complements buildSiblingClassSig (which carries only class + method
// Signatures): an INHERITED parameterized field's Signature lives in a superclass, absent from the
// current class's FieldSignatures, so `this.<inheritedField>.m(...)` loses the receiver's type
// arguments. Exposing per-class field Signatures lets types.ResolveInstantiatedFieldType walk the
// hierarchy and recover the instantiated field type (guava RegularContiguousSet `this.domain` ->
// `DiscreteDomain<C>`). Only fields carrying a Signature attribute (generic fields) are recorded; a
// plain-descriptor field yields ok=false (nothing to recover). Returns nil when no cross-class resolver
// is available (single-class decompile). A nil/empty cache entry records a confirmed miss.
func (c *ClassObjectDumper) buildSiblingFieldSig() func(internalName, fieldName string) (string, bool) {
	if c.foldSiblingResolver == nil {
		return nil
	}
	type entry struct {
		fieldSigs map[string]string
		ok        bool
	}
	cache := map[string]*entry{}
	resolver := c.foldSiblingResolver
	return func(internal, fieldName string) (string, bool) {
		e, hit := cache[internal]
		if !hit {
			e = &entry{}
			cache[internal] = e
			data, ok := resolver(internal)
			if ok && len(data) > 0 {
				if sObj, err := Parse(data); err == nil {
					fieldSigs := map[string]string{}
					for _, fld := range sObj.Fields {
						name, err := sObj.getUtf8(fld.NameIndex)
						if err != nil || name == "" {
							continue
						}
						for _, attr := range fld.Attributes {
							sigAttr, ok := attr.(*SignatureAttribute)
							if !ok {
								continue
							}
							sigStr, err := sObj.getUtf8(sigAttr.SignatureIndex)
							if err != nil || sigStr == "" {
								continue
							}
							fieldSigs[name] = sigStr
							break
						}
					}
					e.fieldSigs = fieldSigs
					e.ok = true
				}
			}
		}
		if !e.ok || e.fieldSigs == nil {
			return "", false
		}
		sig, ok := e.fieldSigs[fieldName]
		return sig, ok
	}
}

// buildSiblingSuperTypes returns a lazy, cached provider of a jar-internal class's RAW direct supertype
// internal names (slash-form): its super_class followed by its direct interfaces. Unlike
// buildSiblingClassSig (which reads the generic Signature attribute and is empty for non-generic
// classes), it reads the always-present super_class / Interfaces constant-pool entries, so it resolves
// plain class hierarchies too (e.g. fastjson2 `Any extends JSONSchema`). It powers the cross-class
// subtype/LUB widening (types.CrossClassDirectLUB). Returns nil when no cross-class resolver is
// available (single-class decompile), which disables the widening. A nil/empty cache entry records a
// confirmed miss (JDK/external class not in the jar) so the result is deterministic and bounded.
func (c *ClassObjectDumper) buildSiblingSuperTypes() func(internalName string) ([]string, bool) {
	if c.foldSiblingResolver == nil {
		return nil
	}
	type entry struct {
		supers []string
		ok     bool
	}
	cache := map[string]*entry{}
	resolver := c.foldSiblingResolver
	return func(internal string) ([]string, bool) {
		if e, hit := cache[internal]; hit {
			return e.supers, e.ok
		}
		e := &entry{}
		cache[internal] = e
		data, ok := resolver(internal)
		if !ok || len(data) == 0 {
			return nil, false // JDK / external: not in jar
		}
		sObj, err := Parse(data)
		if err != nil {
			return nil, false
		}
		var supers []string
		if sup := sObj.GetSupperClassName(); sup != "" {
			supers = append(supers, sup)
		}
		for _, iface := range sObj.GetInterfacesName() {
			if iface != "" {
				supers = append(supers, iface)
			}
		}
		e.supers = supers
		e.ok = true
		return e.supers, true
	}
}

// javaCharLiteralFromCode renders a char annotation value (stored as an int code point) as a valid
// Java char literal: printable ASCII becomes 'x' (with the four chars that need escaping handled),
// everything else becomes a '\uXXXX' escape so the result always compiles.
func javaCharLiteralFromCode(code int) string {
	switch code {
	case '\'':
		return "'\\''"
	case '\\':
		return "'\\\\'"
	case '\n':
		return "'\\n'"
	case '\r':
		return "'\\r'"
	case '\t':
		return "'\\t'"
	}
	if code >= 0x20 && code <= 0x7e {
		return fmt.Sprintf("'%c'", rune(code))
	}
	return fmt.Sprintf("'\\u%04x'", code&0xffff)
}

// externalNestedEnumSourceName converts an enum-constant annotation value's binary type name
// `pkg/Outer$Inner` to its Java SOURCE spelling when the enum type is a NESTED type that is NOT a
// decompiled sibling unit -- i.e. an external dependency / JDK nested enum. It returns the dotted
// simple reference (`Outer.Inner`), the outer-class import (`pkg.Outer`), and ok=true.
//
// Rationale: Yak emits its OWN nested classes as standalone flat `Outer$Inner.java` units and refers to
// them by that same flat name, so a decompiled sibling stays flat. But an EXTERNAL nested enum (guava's
// `@ReflectionSupport(value=ReflectionSupport$Level.FULL)`, j2objc's `ReflectionSupport.Level`) is only
// on the compile classpath as a genuinely nested `Outer.Inner`; the flat `Outer$Inner` is unresolvable
// in source and javac rejects the annotation value ("an enum annotation value must be an enum
// constant"). foldSiblingResolver positively resolving the binary name proves it IS a decompiled
// sibling (keep flat, ok=false); a miss proves it is external (rewrite to dotted). With no resolver
// (single-class mode) we keep the legacy flat behavior. Kill-switch: JDEC_ANNO_ENUM_NESTED_DOT_OFF=1.
func (c *ClassObjectDumper) externalNestedEnumSourceName(internal string) (dottedSimple, outerImport string, ok bool) {
	if os.Getenv("JDEC_ANNO_ENUM_NESTED_DOT_OFF") != "" {
		return "", "", false
	}
	if !strings.Contains(internal, "$") || c.foldSiblingResolver == nil {
		return "", "", false
	}
	if _, isSibling := c.foldSiblingResolver(internal); isSibling {
		return "", "", false // decompiled sibling -> keep the flat `Outer$Inner` name
	}
	fqcn := strings.ReplaceAll(internal, "/", ".") // pkg.Outer$Inner
	pkg, simple := class_context.SplitPackageClassName(fqcn)
	// Guard anonymous/local segments (Outer$1); never an enum, but stay safe.
	for _, seg := range strings.Split(simple, "$") {
		if seg == "" || (seg[0] >= '0' && seg[0] <= '9') {
			return "", "", false
		}
	}
	dottedSimple = strings.ReplaceAll(simple, "$", ".") // Outer.Inner
	if pkg != "" {
		outerImport = pkg + "." + strings.SplitN(simple, "$", 2)[0] // pkg.Outer
	}
	return dottedSimple, outerImport, true
}

// formatAnnotationElementValue renders a single annotation element_value (the right-hand side of an
// element-value pair, or an AnnotationDefault's default value) into its Java source form. Extracted
// from DumpAnnotation so the annotation-default renderer (`@interface` element `default <value>`)
// can reuse the exact same value formatting.
func (c *ClassObjectDumper) formatAnnotationElementValue(element *ElementValuePairAttribute) (string, error) {
	valStr := ""
	switch element.Tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		constant := element.Value.(ConstantInfo)
		switch ret := constant.(type) {
		case *ConstantStringInfo:
			s, err := c.obj.getUtf8(ret.StringIndex)
			if err != nil {
				return "", err
			}
			valStr = values.JavaStringToLiteral(s)
		case *ConstantLongInfo:
			valStr = fmt.Sprintf("%dL", ret.Value)
		case *ConstantIntegerInfo:
			if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'Z' {
				if ret.Value == 0 {
					valStr = "false"
				} else {
					valStr = "true"
				}
			} else if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'C' {
				valStr = javaCharLiteralFromCode(int(ret.Value))
			} else {
				valStr = fmt.Sprintf("%d", ret.Value)
			}
		case *ConstantDoubleInfo:
			valStr = fmt.Sprintf("%f", ret.Value)
		case *ConstantFloatInfo:
			valStr = fmt.Sprintf("%f", ret.Value)
		default:
			return "", errors.New("parse annotation error, unknown constant type")
		}
	case 's':
		valStr = values.JavaStringToLiteral(element.Value)
	case 'c':
		descStr, _ := element.Value.(string)
		classTyp, perr := types.ParseDescriptor(descStr)
		if perr != nil || classTyp == nil {
			fallback := strings.TrimSuffix(strings.TrimPrefix(descStr, "L"), ";")
			valStr = strings.Replace(fallback, "/", ".", -1) + ".class"
		} else {
			typeStr := classTyp.String(c.FuncCtx)
			// A PRIMITIVE class literal (void.class / int.class / boolean.class ...) must render the
			// raw keyword: it is never imported and must NOT pass through ShortTypeName -> SafeIdentifier,
			// which appends '_' to every Java keyword ("void" -> "void_"), emitting the uncompilable
			// `void_.class` (real: fastjson2 @JSONType `builder() default void.class`). Only reference
			// (class) types get imported/short-named. Array types already bypass this branch below.
			// Kill-switch: JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF restores the old (broken) path.
			_, isPrimitive := classTyp.RawType().(*types.JavaPrimer)
			primitiveClassLit := isPrimitive && os.Getenv("JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF") == ""
			if !classTyp.IsArray() && !primitiveClassLit {
				c.FuncCtx.Import(typeStr)
				typeStr = c.FuncCtx.ShortTypeName(typeStr)
			}
			valStr = typeStr + ".class"
		}
	case '@':
		annotation := element.Value.(*AnnotationAttribute)
		res, err := c.DumpAnnotation(annotation)
		if err != nil {
			return "", err
		}
		valStr = res
	case '[':
		l := element.Value.([]*ElementValuePairAttribute)
		eleList := []string{}
		for _, e := range l {
			res, err := c.formatAnnotationElementValue(e)
			if err != nil {
				return "", err
			}
			eleList = append(eleList, res)
		}
		valStr = fmt.Sprintf("{%s}", strings.Join(eleList, ", "))
	case 'e':
		switch ret := element.Value.(type) {
		case *EnumConstValue:
			if len(ret.TypeName) <= 2 {
				return "", fmt.Errorf("parse annotation error, invalid enum type name: %s", ret.TypeName)
			}
			internal := ret.TypeName[1 : len(ret.TypeName)-1]
			// An EXTERNAL nested enum referenced by its flat binary name (Outer$Inner) is unresolvable
			// in source; rewrite to the dotted `Outer.Inner` and import the outer class instead.
			if dotted, outerImport, ok := c.externalNestedEnumSourceName(internal); ok {
				if outerImport != "" {
					c.FuncCtx.Import(outerImport)
				}
				return dotted + "." + ret.ConstName, nil
			}
			fullqualifiedName := strings.Replace(internal, "/", ".", -1)
			c.FuncCtx.Import(fullqualifiedName)
			last := strings.LastIndex(fullqualifiedName, ".")
			if last == -1 {
				return fullqualifiedName + "." + ret.ConstName, nil
			}
			return fullqualifiedName[last+1:] + "." + ret.ConstName, nil
		default:
			return "", fmt.Errorf("parse annotation error, unknown tag: %c, ret: %T", element.Tag, ret)
		}
	default:
		return "", fmt.Errorf("parse annotation error, unknown tag: %c", element.Tag)
	}
	return valStr, nil
}

// annotationElementDefaultClause returns the ` default <value>` suffix for an annotation element
// method (an abstract method of an @interface) when it carries an AnnotationDefault attribute, or
// "" otherwise. Without it javac rejects any use site that omits the element. Kill-switch:
// JDEC_ANNO_DEFAULT_OFF=1.
func (c *ClassObjectDumper) annotationElementDefaultClause(method *MemberInfo) string {
	if method == nil || os.Getenv("JDEC_ANNO_DEFAULT_OFF") != "" {
		return ""
	}
	for _, attr := range method.Attributes {
		ad, ok := attr.(*AnnotationDefaultAttribute)
		if !ok || ad.DefaultValue == nil {
			continue
		}
		val, err := c.formatAnnotationElementValue(ad.DefaultValue)
		if err != nil || val == "" {
			return ""
		}
		return " default " + val
	}
	return ""
}

func (c *ClassObjectDumper) DumpAnnotation(anno *AnnotationAttribute) (string, error) {
	result := ""

	annoName := anno.TypeName
	typ, err := types.ParseDescriptor(annoName)
	if err != nil {
		return "", fmt.Errorf("parse annotation error, %w", err)
	}
	classIns, ok := typ.RawType().(*types.JavaClass)
	if !ok {
		return "", errors.New("invalid annotation type")
	}
	annoName = c.FuncCtx.ShortTypeName(classIns.Name)
	var parseElement func(element *ElementValuePairAttribute) (string, error)
	parseElement = func(element *ElementValuePairAttribute) (string, error) {
		valStr := ""
		switch element.Tag {
		case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
			constant := element.Value.(ConstantInfo)
			switch ret := constant.(type) {
			case *ConstantStringInfo:
				s, err := c.obj.getUtf8(ret.StringIndex)
				if err != nil {
					return "", err
				}
				valStr = values.JavaStringToLiteral(s)
			case *ConstantLongInfo:
				valStr = fmt.Sprintf("%dL", ret.Value)
			case *ConstantIntegerInfo:
				// boolean/char/byte/short/int annotation members ALL store a CONSTANT_Integer in the
				// pool; the element TAG carries the real Java type. Without dispatching on the tag a
				// boolean member emits `1`/`0` (javac: "int cannot be converted to boolean") and a char
				// member emits its code point (`59` instead of `';'`), so the decompiled annotation does
				// not compile. Render by tag. Kill-switch: JDEC_ANNO_LITERAL_OFF=1 restores raw ints.
				if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'Z' {
					if ret.Value == 0 {
						valStr = "false"
					} else {
						valStr = "true"
					}
				} else if os.Getenv("JDEC_ANNO_LITERAL_OFF") == "" && element.Tag == 'C' {
					valStr = javaCharLiteralFromCode(int(ret.Value))
				} else {
					valStr = fmt.Sprintf("%d", ret.Value)
				}
			case *ConstantDoubleInfo:
				valStr = fmt.Sprintf("%f", ret.Value)
			case *ConstantFloatInfo:
				valStr = fmt.Sprintf("%f", ret.Value)
			default:
				return "", errors.New("parse annotation error, unknown constant type")
			}
		case 's':
			valStr = values.JavaStringToLiteral(element.Value) // fmt.Sprintf("\"%s\"", element.Value.(string))
		case 'c':
			// class element value: the raw value is a field descriptor like
			// "Lcom/example/Foo;" or "[I"; render it as a Java class literal "Foo.class".
			descStr, _ := element.Value.(string)
			classTyp, perr := types.ParseDescriptor(descStr)
			if perr != nil || classTyp == nil {
				fallback := strings.TrimSuffix(strings.TrimPrefix(descStr, "L"), ";")
				valStr = strings.Replace(fallback, "/", ".", -1) + ".class"
			} else {
				typeStr := classTyp.String(c.FuncCtx)
				// A PRIMITIVE class literal (void.class / int.class ...) must render the raw keyword:
				// it is never imported and must NOT go through ShortTypeName -> SafeIdentifier, which
				// mangles keywords ("void" -> "void_") into the uncompilable `void_.class`. See the
				// twin branch in formatAnnotationElementValue. Kill-switch: JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF.
				_, isPrimitive := classTyp.RawType().(*types.JavaPrimer)
				primitiveClassLit := isPrimitive && os.Getenv("JDEC_ANNO_PRIMITIVE_CLASSLIT_OFF") == ""
				if !classTyp.IsArray() && !primitiveClassLit {
					c.FuncCtx.Import(typeStr)
					typeStr = c.FuncCtx.ShortTypeName(typeStr)
				}
				valStr = typeStr + ".class"
			}
		case '@':
			//ele.Value = ParseAnnotation(cp)
			annotation := element.Value.(*AnnotationAttribute)
			res, err := c.DumpAnnotation(annotation)
			if err != nil {
				return "", err
			}
			valStr = res
		case '[':
			//length := reader.readUint16()
			//l := []any{}
			//for k := 0; k < int(length); k++ {
			//	val := ParseAnnotationElementValue(cp)
			//	l = append(l, val)
			//}
			//ele.Value = l
			l := element.Value.([]*ElementValuePairAttribute)
			eleList := []string{}
			for _, e := range l {
				res, err := parseElement(e)
				if err != nil {
					return "", err
				}
				eleList = append(eleList, res)
			}
			valStr = fmt.Sprintf("{%s}", strings.Join(eleList, ", "))
		case 'e':
			// fullname
			switch ret := element.Value.(type) {
			case *EnumConstValue:
				if len(ret.TypeName) <= 2 {
					return "", fmt.Errorf("parse annotation error, invalid enum type name: %s", ret.TypeName)
				}
				internal := ret.TypeName[1 : len(ret.TypeName)-1]
				// An EXTERNAL nested enum referenced by its flat binary name (Outer$Inner) is
				// unresolvable in source; rewrite to dotted `Outer.Inner` and import the outer class.
				if dotted, outerImport, ok := c.externalNestedEnumSourceName(internal); ok {
					if outerImport != "" {
						c.FuncCtx.Import(outerImport)
					}
					return dotted + "." + ret.ConstName, nil
				}
				fullqualifiedName := strings.Replace(internal, "/", ".", -1)
				c.FuncCtx.Import(fullqualifiedName)
				last := strings.LastIndex(fullqualifiedName, ".")
				if last == -1 {
					return fullqualifiedName + "." + ret.ConstName, nil
				}
				return fullqualifiedName[last+1:] + "." + ret.ConstName, nil
			default:
				return "", fmt.Errorf("parse annotation error, unknown tag: %c, ret: %T", element.Tag, ret)
			}
		default:
			return "", fmt.Errorf("parse annotation error, unknown tag: %c", element.Tag)
		}
		return valStr, nil
	}
	elementStrList := []string{}
	for _, element := range anno.ElementValuePairs {
		str, err := parseElement(element)
		if err != nil {
			return "", err
		}
		elementStrList = append(elementStrList, fmt.Sprintf("%s=%s", element.Name, str))
	}
	result = fmt.Sprintf("@%s(%s)", annoName, strings.Join(elementStrList, ", "))
	return result, nil
}

// normalizeCatchClauseType keeps a catch clause's declared type a legal reference type. A catch
// type must be a subtype of Throwable; when upstream type inference degrades the exception
// variable to a primitive (e.g. "boolean" from a reused slot) or an array, fall back to Throwable
// so the emitted Java stays syntactically valid.
func normalizeCatchClauseType(excType string) string {
	if strings.HasSuffix(excType, "[]") {
		return "Throwable"
	}
	switch excType {
	case "boolean", "byte", "char", "short", "int", "long", "float", "double", "void":
		return "Throwable"
	}
	return excType
}

// mergeNestedSameTypeCatches collapses the decompiler-synthesized "two catch clauses of the same
// type" shape. Java forbids a try from declaring two handlers of the same exception type, but it is
// exactly what javac's try-with-resources / try-catch-finally desugaring produces: an inner handler
// (e.g. the try-with-resources `catch (Throwable t) { primaryExc = t; throw t; }` that records the
// primary exception) whose protected region is itself covered by an outer Throwable cleanup ("any")
// handler. At runtime the inner handler runs first and unconditionally rethrows its caught
// exception, which the outer handler then catches; so the two handlers are sequential, not
// alternative. Reconstruct that ordering by concatenating the first handler's body (minus its
// trailing `throw e`) with the second handler's body, under the first handler's catch variable.
//
// The merge only fires on ADJACENT handlers of the same (normalized) type whose first member ends
// by unconditionally rethrowing its own caught variable. That signature is unique to the synthesized
// illegal shape, so the pass never reorders or merges distinct user-written handlers (which can
// never share a type anyway). Chains of three or more (nested try-with-resources) collapse by
// repeatedly merging the leading pair.
func mergeNestedSameTypeCatches(funcCtx *class_context.ClassContext, exceptions []*values.JavaRef, bodies [][]statements.Statement) ([]*values.JavaRef, [][]statements.Statement) {
	if len(bodies) < 2 || len(exceptions) != len(bodies) {
		return exceptions, bodies
	}
	catchTypeKey := func(ref *values.JavaRef) string {
		if ref == nil {
			return ""
		}
		t := ref.Type()
		if t == nil {
			return "Throwable"
		}
		return normalizeCatchClauseType(t.String(funcCtx))
	}
	// lastMeaningfulStmt returns the rendered text and index of the body's last statement that the
	// dumper would actually emit (MiddleStatement / StackAssignStatement are dropped at render time).
	lastMeaningfulStmt := func(body []statements.Statement) (string, int) {
		for i := len(body) - 1; i >= 0; i-- {
			switch body[i].(type) {
			case *statements.MiddleStatement, *statements.StackAssignStatement:
				continue
			}
			return strings.TrimSpace(body[i].String(funcCtx)), i
		}
		return "", -1
	}
	exc := append([]*values.JavaRef{}, exceptions...)
	bod := append([][]statements.Statement{}, bodies...)
	i := 0
	for i+1 < len(bod) {
		sameType := catchTypeKey(exc[i]) != "" && catchTypeKey(exc[i]) == catchTypeKey(exc[i+1])
		rethrows := false
		var throwIdx int
		if sameType && exc[i] != nil {
			varName := strings.TrimSpace(exc[i].String(funcCtx))
			lastStr, lastIdx := lastMeaningfulStmt(bod[i])
			if lastIdx >= 0 && varName != "" && lastStr == "throw "+varName {
				rethrows = true
				throwIdx = lastIdx
			}
		}
		if sameType && rethrows {
			merged := append([]statements.Statement{}, bod[i][:throwIdx]...)
			merged = append(merged, bod[i+1]...)
			bod[i] = merged
			exc = append(exc[:i+1], exc[i+2:]...)
			bod = append(bod[:i+1], bod[i+2:]...)
			// Do not advance: the merged handler may chain into a further same-type handler.
			continue
		}
		i++
	}
	return exc, bod
}

// isUnconditionalTerminalStatement reports whether st unconditionally transfers control out of
// the current block: return / throw / break / continue (with or without a label). In valid Java
// any sibling statement that follows such a statement at the same nesting level is unreachable and
// is rejected by javac as a compile error. The decompiler occasionally synthesizes a structural
// jump (e.g. a `break;` to leave a loop) right after a real `return`/`throw`; emitting it would
// make the output uncompilable, so callers stop rendering a statement list once this returns true.
func isUnconditionalTerminalStatement(st statements.Statement, funcCtx *class_context.ClassContext) bool {
	switch s := st.(type) {
	case *statements.ReturnStatement:
		return true
	case *statements.CustomStatement:
		t := strings.TrimSpace(s.String(funcCtx))
		switch {
		case t == "break", t == "continue", t == "return":
			return true
		case strings.HasPrefix(t, "break "), strings.HasPrefix(t, "continue "), strings.HasPrefix(t, "throw "):
			return true
		}
	case *statements.DoWhileStatement:
		// An infinite loop (condition is the constant true) that never breaks back to its own
		// successor transfers control away forever, so any sibling after it is unreachable.
		// This is common after CFG structuring: a nested loop's exit is wired straight to the
		// outer loop's `continue LABEL`, leaving the inner do-while(true) with no break and a
		// dangling `continue;` behind it that javac rejects as an unreachable statement.
		if loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
			!loopBodyHasEscapingBreak(s.Body, s.Label, true, funcCtx) {
			return true
		}
	case *statements.WhileStatement:
		if loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
			!loopBodyHasEscapingBreak(s.Body, "", true, funcCtx) {
			return true
		}
	}
	return false
}

func needsTrailingIncompleteControlFlowThrow(statementList []statements.Statement, returnType types.JavaType, funcCtx *class_context.ClassContext) bool {
	if returnType == nil || returnType.String(funcCtx) == "void" {
		return false
	}
	for i := len(statementList) - 1; i >= 0; i-- {
		st := statementList[i]
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		}
		switch s := st.(type) {
		case *statements.DoWhileStatement:
			return loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
				loopBodyHasEscapingBreak(s.Body, s.Label, true, funcCtx)
		case *statements.WhileStatement:
			return loopConditionIsConstTrue(s.ConditionValue, funcCtx) &&
				loopBodyHasEscapingBreak(s.Body, "", true, funcCtx)
		default:
			return false
		}
	}
	return false
}

// loopConditionIsConstTrue reports whether a loop condition is the literal true (an infinite loop).
func loopConditionIsConstTrue(cond values.JavaValue, funcCtx *class_context.ClassContext) bool {
	return cond != nil && strings.TrimSpace(cond.String(funcCtx)) == "true"
}

// loopBodyHasEscapingBreak reports whether body (the body of a loop whose label is loopLabel)
// contains a break that hands control to the statement following THAT loop: an unlabeled `break`
// that is not nested inside a deeper loop or switch, or a `break <loopLabel>` at any depth. continue
// statements and breaks targeting other constructs do not return control to this loop's successor,
// so they are not counted. directlyInLoop becomes false once the walk descends into a nested loop or
// switch, where an unlabeled break belongs to that inner construct instead of to our loop. The
// walker covers every statement kind that can hold a nested break; leaf statements without nested
// bodies cannot contain one.
func loopBodyHasEscapingBreak(body []statements.Statement, loopLabel string, directlyInLoop bool, funcCtx *class_context.ClassContext) bool {
	for _, st := range body {
		switch s := st.(type) {
		case *statements.CustomStatement:
			t := strings.TrimSpace(s.String(funcCtx))
			if directlyInLoop && t == "break" {
				return true
			}
			if loopLabel != "" && t == "break "+loopLabel {
				return true
			}
		case *statements.IfStatement:
			if loopBodyHasEscapingBreak(s.IfBody, loopLabel, directlyInLoop, funcCtx) ||
				loopBodyHasEscapingBreak(s.ElseBody, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
		case *statements.TryCatchStatement:
			if loopBodyHasEscapingBreak(s.TryBody, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
			for _, cb := range s.CatchBodies {
				if loopBodyHasEscapingBreak(cb, loopLabel, directlyInLoop, funcCtx) {
					return true
				}
			}
		case *statements.SynchronizedStatement:
			if loopBodyHasEscapingBreak(s.Body, loopLabel, directlyInLoop, funcCtx) {
				return true
			}
		case *statements.DoWhileStatement:
			// Nested loop: an unlabeled break is its own; only `break <loopLabel>` escapes to us.
			if loopBodyHasEscapingBreak(s.Body, loopLabel, false, funcCtx) {
				return true
			}
		case *statements.WhileStatement:
			if loopBodyHasEscapingBreak(s.Body, loopLabel, false, funcCtx) {
				return true
			}
		case *statements.SwitchStatement:
			for _, c := range s.Cases {
				if loopBodyHasEscapingBreak(c.Body, loopLabel, false, funcCtx) {
					return true
				}
			}
		}
	}
	return false
}

func (c *ClassObjectDumper) DumpMethod(methodName, desc string) (*dumpedMethods, error) {
	return c.DumpMethodWithInitialId(methodName, desc, utils2.NewRootVariableId())
}

func (c *ClassObjectDumper) DumpMethodWithInitialId(methodName, desc string, id *utils2.VariableId) (*dumpedMethods, error) {
	traitId := fmt.Sprintf("name:%s,desc:%s", methodName, desc)
	if v, ok := c.dumpedMethodsSet[traitId]; ok {
		return v, nil
	}
	var method *MemberInfo
	var name, descriptor string
	var err error
	var dumped = &dumpedMethods{}

	debugMode := false
	defer func() {
		if debugMode && method != nil {
			log.Info("DumpMethodWithInitialId done")
			log.Info("\n" + dumped.code)
		}
	}()

	c.dumpedMethodsSet[traitId] = dumped
	for _, info := range c.obj.Methods {
		name, err = c.obj.getUtf8(info.NameIndex)
		if err != nil {
			return dumped, utils.Wrapf(err, "getUtf8(%v) failed", info.NameIndex)
		}
		descriptor, err = c.obj.getUtf8(info.DescriptorIndex)
		if err != nil {
			return dumped, utils.Wrapf(err, "getUtf8(%v) failed", info.DescriptorIndex)
		}
		if name == methodName && descriptor == desc {
			method = info
			break
		}
	}
	if method == nil {
		return dumped, fmt.Errorf("method %s not found", methodName)
	}

	var isLambda bool
	if v := c.lambdaMethods[name]; slices.Contains(v, descriptor) {
		isLambda = true
	}
	// Track lexical lambda nesting so nested-lambda parameters get collision-free names (see
	// c.lambdaDepth). The increment must bracket ParseBytesCode below, because an inner lambda's arrow
	// is materialised eagerly during the outer lambda's bytecode parse.
	if isLambda {
		c.lambdaDepth++
		defer func() { c.lambdaDepth-- }()
	}

	c.FuncCtx.IsStatic = method.AccessFlags&StaticFlag == StaticFlag
	accessFlagsVerbose, accessFlagCode := getMethodAccessFlagsVerbose(method.AccessFlags)

	var isVarArgs bool
	var abstractMethod bool
	accessFlagsVerbose = lo.Filter(accessFlagsVerbose, func(item string, index int) bool {
		if item == "varargs" {
			isVarArgs = true
			return false
		}
		if item == "abstract" {
			abstractMethod = true
		}
		return true
	})
	_ = abstractMethod

	accessFlags := accessFlagCode
	methodType, err := types.ParseMethodDescriptor(descriptor)
	if err != nil {
		return dumped, utils.Wrapf(err, "ParseMethodDescriptor(%v) failed", descriptor)
	}
	descriptorParamTypes := slices.Clone(methodType.FunctionType().ParamTypes)
	descriptorParamCount := len(descriptorParamTypes)
	// Override the descriptor-derived method type with generic information from the
	// Signature attribute, if present and parseable. This recovers erased generics on
	// method parameters and return types (e.g. BiFunction<Integer,Integer,Integer> vs raw
	// BiFunction). Falls back silently to the descriptor if the signature cannot be parsed.
	//
	// methodTypeParams is the method's own formal type-parameter section ("<T>", "<K, V>"), rendered
	// from a signature that BEGINS with "<...>" (a generic method like `<T> T checkNotNull(T)`).
	// ParseMethodSignatureFull (unlike the old ParseMethodSignature) does not bail on that leading
	// section, so such a method's params/return are now recovered as the type variables (T) instead of
	// staying erased to Object - the dominant guava `base` recompile blocker after the synchronized
	// scope fix (Preconditions.checkNotNull rendered `Object checkNotNull(Object)`, so every
	// `predicate.apply(checkNotNull(x))` failed "Object cannot be converted to CAP#1"). The header
	// then emits the `<T>` declaration before the return type. Kill-switch: JDEC_METHOD_TYPEPARAMS_OFF.
	methodTypeParams := ""
	var methodTypeParamNames []string
	var methodSigStr string
	for _, attr := range method.Attributes {
		if sigAttr, ok := attr.(*SignatureAttribute); ok {
			if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && sigStr != "" {
				methodSigStr = sigStr
				tps, sigParams, sigRet := types.ParseMethodSignatureFull(sigStr, c.FuncCtx)
				// Gate on sigRet (not sigParams): a zero-arg generic method like
				// `()TK;` (Map.Entry.getKey) parses to (nil params, K return). The old
				// `sigParams != nil` guard skipped exactly these, leaving the return type
				// erased to Object so the override of an interface method failed to compile
				// ("return type Object is not compatible with V"). sigRet==nil still means a
				// genuine parse failure, so we fall back to the descriptor as before. Kill-switch
				// JDEC_METHOD_SIG_RET_OFF restores the legacy sigParams!=nil gate.
				applicable := sigRet != nil
				if os.Getenv("JDEC_METHOD_SIG_RET_OFF") != "" {
					applicable = sigParams != nil
				}
				if applicable {
					mt := methodType.FunctionType()
					// Only override when the param count matches (signature may include
					// formal type parameters that shift the count; skip those for safety).
					if len(sigParams) == len(mt.ParamTypes) {
						if sigParams != nil {
							mt.ParamTypes = sigParams
						}
						mt.ReturnType = sigRet
					} else if name == "<init>" && os.Getenv("JDEC_INNER_CTOR_SIG_ALIGN_OFF") == "" &&
						len(sigParams) == len(mt.ParamTypes)-1 && len(mt.ParamTypes) >= 1 &&
						c.hasOuterThisField() {
						// Non-static inner class constructor: javac OMITS the synthetic leading this$0
						// parameter from the ctor Signature, so its param count is exactly one less than
						// the descriptor's. Align the Signature params to the TRAILING descriptor params,
						// keeping the erased outer-instance param[0], and lift the generic types onto
						// params[1:]. Recovers e.g. guava TreeTraverser$PreOrderIterator(TreeTraverser, T)
						// whose real param was erased to Object, so `singletonIterator(checkNotNull(v))`
						// now infers `Iterator<T>` for the `Deque<Iterator<T>>` field. Behaviour-preserving
						// (descriptor unchanged). Kill-switch JDEC_INNER_CTOR_SIG_ALIGN_OFF.
						aligned := slices.Clone(mt.ParamTypes)
						copy(aligned[1:], sigParams)
						mt.ParamTypes = aligned
						mt.ReturnType = sigRet
					}
				}
				if os.Getenv("JDEC_METHOD_TYPEPARAMS_OFF") == "" {
					methodTypeParams = tps
					methodTypeParamNames = types.MethodFormalTypeParamNames(sigStr)
				}
			}
			break
		}
	}
	// A synthetic access-bridge constructor carries NO Signature attribute, so its parameters stay
	// erased to their descriptor types (Object / raw). Its only body is `this(args...)` forwarding to
	// the private target ctor, so when that target declares a type variable (e.g. guava
	// Equivalence.Wrapper(Equivalence, Object, Equivalence$1) -> this(var1, var2) into the private
	// (Equivalence<? super T>, T) ctor) the bare `Object` arg fails "Object cannot be converted to T".
	// Lift the target ctor's GENERIC param types onto the bridge: the bridge erases to the same
	// descriptor (byte-faithful) and its (raw) call sites still type-check. This recurs across every
	// nested class with a private generic ctor reached from its encloser, so the fix is broad.
	// Kill-switch: JDEC_NO_SYN_BRIDGE_CTOR_RETYPE=1.
	if name == "<init>" && os.Getenv("JDEC_NO_SYN_BRIDGE_CTOR_RETYPE") == "" &&
		c.isSyntheticAccessBridgeCtor(descriptor, method.AccessFlags) {
		c.reTypeSyntheticBridgeCtorParams(descriptor, methodType.FunctionType())
	}
	// Bring the method's own type-variable names into scope for the duration of this method's render
	// so renderers (e.g. typeVarReturnCast) recognize them like class-level ones, then restore the
	// class-scope set afterward so they never leak into sibling members.
	if len(methodTypeParamNames) > 0 && c.FuncCtx != nil {
		savedTypeParams := c.FuncCtx.TypeParams
		c.FuncCtx.TypeParams = append(slices.Clone(savedTypeParams), methodTypeParamNames...)
		defer func() { c.FuncCtx.TypeParams = savedTypeParams }()
	}
	// Record the current method's raw generic Signature so renderers can recover a receiver whose static
	// type is a bare method-scope type variable with a parameterized bound (`C var1` where
	// `<C extends Collection<? super E>>`). Set on entry, restored on exit so it never leaks into sibling
	// members. Kill-switch JDEC_TYPEVAR_BOUND_RECV_OFF (the consumer that reads it).
	if c.FuncCtx != nil && os.Getenv("JDEC_TYPEVAR_BOUND_RECV_OFF") == "" {
		savedMethodSig := c.FuncCtx.CurrentMethodSig
		c.FuncCtx.CurrentMethodSig = methodSigStr
		defer func() { c.FuncCtx.CurrentMethodSig = savedMethodSig }()
	}
	c.MethodType = methodType.FunctionType()
	returnTypeStr := methodType.FunctionType().ReturnType.String(c.FuncCtx)
	code := ""
	c.Tab()
	c.CurrentMethod = method
	funcCtx := c.FuncCtx
	funcCtx.FunctionName = name
	//if name != "scope" {
	//	return &dumpedMethods{}, nil
	//}
	//println(name)
	finalFieldMap := map[string]struct{}{}
	finalFieldRenderNameToRaw := map[string]string{}
	classStaticInitializersMustHoist := slices.Contains(c.obj.AccessFlagsVerbose, "interface") || slices.Contains(c.obj.AccessFlagsVerbose, "annotation")
	for _, field := range c.obj.Fields {
		var finalFalg uint16 = 0x0010
		if field.AccessFlags&finalFalg == finalFalg {
			rawName := c.obj.ConstantPoolManager.GetUtf8(int(field.NameIndex)).Value
			finalFieldMap[rawName] = struct{}{}
			finalFieldRenderNameToRaw[class_context.SafeIdentifier(rawName)] = rawName
		}
	}
	annoStrs := []string{}
	funcCtx.FunctionType = c.MethodType
	var paramsNewStr string
	var lambdaParamNames []string
	var exceptions string
	for _, attribute := range method.Attributes {
		if exceptionAttr, ok := attribute.(*ExceptionsAttribute); ok {
			exceptions = " throws "
			expList := []string{}
			for _, u := range exceptionAttr.ExceptionIndexTable {
				info, err := c.obj.getConstantInfo(u)
				if err != nil {
					continue
				}
				classInfo := info.(*ConstantClassInfo)
				name, err := c.obj.getUtf8(classInfo.NameIndex)
				if err != nil {
					continue
				}
				name = strings.Replace(name, "/", ".", -1)
				funcCtx.Import(name)
				name = funcCtx.ShortTypeName(name)
				if name != "" {
					expList = append(expList, name)
				}
			}
			exceptions += strings.Join(expList, ", ")
		}
		if anno, ok := attribute.(*RuntimeVisibleAnnotationsAttribute); ok {
			for _, annotation := range anno.Annotations {
				res, err := c.DumpAnnotation(annotation)
				if err != nil {
					return dumped, err
				}
				annoStrs = append(annoStrs, res)
			}
		}
		if codeAttr, ok := attribute.(*CodeAttribute); ok {
			params, statementList, err := ParseBytesCode(c, codeAttr, id)
			if err != nil {
				return dumped, utils.Wrap(err, "ParseBytesCode failed")
			}
			thisRemoved := false
			if len(params) > 0 {
				if v, ok := params[0].(*values.JavaRef); ok && v.IsThis {
					params = params[1:]
					thisRemoved = true
				}
			}
			// For a synthetic lambda body, the leading parameters are captured variables that
			// javac prepended to the impl signature; they are not lambda parameters. Drop them from
			// the arrow list and rename each to a capture placeholder so every body reference resolves
			// to the captured value at the invokedynamic call site (see bootstrap_methods.go). For an
			// instance lambda the receiver was captured as the first dynamic arg but is represented by
			// the impl method's `this` (already stripped above), so its placeholder index is offset.
			samParams := params
			if isLambda {
				if n := c.lambdaCaptureCount[name+descriptor]; n > 0 {
					capArgOffset := 0
					if thisRemoved {
						capArgOffset = 1
					}
					drop := n - capArgOffset
					if drop > 0 && drop <= len(params) {
						for i := 0; i < drop; i++ {
							if ref, ok := params[i].(*values.JavaRef); ok && ref.Id != nil {
								ref.Id.SetName(fmt.Sprintf("\x00LCAP%d\x00", i+capArgOffset))
							}
						}
						samParams = params[drop:]
					}
				}
			}
			// A genuine enum constructor carries two synthetic leading parameters (String
			// name, int ordinal) that javac injects and forbids in source. Drop them from the
			// rendered signature; the synthetic super(name, ordinal) call is stripped from the
			// body below.
			isEnumCtor := name == "<init>" && c.isGenuineEnum()
			if isEnumCtor && len(samParams) >= 2 {
				samParams = samParams[2:]
			}
			// A lambda body is emitted as an arrow expression `(Type p0, Type p1) -> ...`
			// inline in the enclosing method, so Java requires its parameter names to be unique
			// across the entire method scope (no shadowing). The fresh root namespace gives
			// them var0, var1, ... which can still collide with the enclosing method's own
			// params/locals (var1, var2, ...). Rename each SAM param to an `l<N>` name that the
			// slot-based scheme never generates, eliminating the collision while keeping the body
			// consistent (every body reference shares the same JavaRef/Id).
			if isLambda {
				// A nested lambda (depth>=2) namespaces its parameters by nesting depth so they cannot
				// shadow an enclosing lambda parameter; a top-level lambda keeps the flat `l<i>` name so
				// the overwhelmingly common single-lambda case stays byte-for-byte unchanged.
				nestScope := c.lambdaDepth >= 2 && os.Getenv("JDEC_LAMBDA_PARAM_SCOPE_OFF") == ""
				for i, val := range samParams {
					if ref, ok := val.(*values.JavaRef); ok && ref.Id != nil && !ref.IsThis {
						var name string
						if nestScope {
							name = fmt.Sprintf("l%d_%d", c.lambdaDepth, i)
						} else {
							name = fmt.Sprintf("l%d", i)
						}
						ref.Id.SetName(name)
						lambdaParamNames = append(lambdaParamNames, name)
					}
				}
			}
			ensureUniqueParameterNames(samParams, funcCtx)
			paramsNewStrList := []string{}
			// A lambda arrow parameter whose type is a GENERIC class rendered RAW is best emitted WITHOUT
			// an explicit type: the bytecode only preserves the ERASED impl-method descriptor (e.g.
			// `Predicate` for a SAM `matches(Predicate<String>)`), so `(Predicate l0) -> ...` fails to bind
			// against the parameterized SAM ("incompatible parameter types in lambda expression" --
			// spring-core ProfilesParser.or/and/not/equals, SimpleAliasRegistry, the ReactiveAdapterRegistry
			// registrars, codec encoders). An IMPLICIT `(l0) -> ...` binds to the inferred SAM type instead.
			//
			// But dropping the type is NOT free: when the lambda flows into a RAW-cast generic call (e.g.
			// `Collections.sort((List)(var1), (Integer x, Integer y) -> ...)`), the EXPLICIT concrete param
			// type is what DRIVES the method's type-variable inference (T=Integer); made implicit, T infers
			// to Object and the body's `x.intValue()` no longer resolves (the synthetic round-trip guard).
			// So go implicit only when the erasure would actually mismatch -- i.e. at least one param is a
			// generic-capable class rendered raw -- and keep the explicit type otherwise (concrete types
			// like Integer/String help inference and match exactly).
			// Kill-switch: JDEC_LAMBDA_IMPLICIT_PARAMS_OFF=1 forces explicit-typed lambda parameters.
			if isLambda && os.Getenv("JDEC_LAMBDA_IMPLICIT_PARAMS_OFF") == "" && c.lambdaParamsShouldBeImplicit(samParams) {
				// Implicit: emit only the (unique) parameter names, letting Java infer their types.
				for _, val := range samParams {
					nm := ""
					if val != nil {
						nm = val.String(c.FuncCtx)
					}
					paramsNewStrList = append(paramsNewStrList, nm)
				}
			} else if !isLambda && name != "<init>" && name != "<clinit>" && len(samParams) != descriptorParamCount {
				paramSlotOffset := 0
				if !funcCtx.IsStatic {
					paramSlotOffset = 1
				}
				for i, pt := range descriptorParamTypes {
					paramName := fmt.Sprintf("var%d", i+paramSlotOffset)
					if i == len(descriptorParamTypes)-1 && isVarArgs && pt.IsArray() {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s... %s", pt.ElementType().String(c.FuncCtx), paramName))
					} else {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s %s", pt.String(c.FuncCtx), paramName))
					}
				}
			} else {
				// samParams and descriptorParamTypes share the SAME trailing source parameters; any
				// synthetic prefix (enum ctor's String,int) lives only at the FRONT of the descriptor
				// list and was sliced off samParams above, so align them from the tail.
				descTailOffset := descriptorParamCount - len(samParams)
				for i, val := range samParams {
					typ := val.Type()
					if i == len(samParams)-1 && isVarArgs && typ != nil && typ.IsArray() {
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s... %s", typ.ElementType().String(c.FuncCtx), val.String(c.FuncCtx)))
					} else {
						typName := "java.lang.Object"
						if typ != nil {
							typName = typ.String(c.FuncCtx)
						}
						// A narrow int-category parameter (char/byte/short) whose slot was widened to
						// int by an in-body int reassignment must still be DECLARED with its authoritative
						// descriptor type, or assigning it to a same-typed field/return is a "possible
						// lossy conversion from int to char" javac error (e.g. guava ArrayBasedCharEscaper's
						// `char safeMin` ctor). The descriptor is the ground truth for primitive params.
						if !isLambda && descTailOffset >= 0 {
							if dt := paramDescriptorNarrowType(descriptorParamTypes, descTailOffset+i, typ); dt != nil {
								typName = dt.String(c.FuncCtx)
							}
						}
						paramsNewStrList = append(paramsNewStrList, fmt.Sprintf("%s %s", typName, val.String(c.FuncCtx)))
					}
				}
			}
			c.MethodType = methodType.FunctionType()
			paramsNewStr = strings.Join(paramsNewStrList, ", ")

			// Rename locals whose slot-derived names collide across nested scopes (e.g. two
			// nested catch parameters both named var4) so the emitted Java is re-compilable.
			resolveLocalNameCollisions(params, statementList)

			// Per-constructor/<clinit> count of field assignments. A final field is only safe to
			// lift into a field initializer when assigned exactly once in this body; a blank final
			// assigned across multiple branches must keep its in-body assignments (see
			// countConstructorFieldAssignments).
			ctorFieldAssignCount := countConstructorFieldAssignments(statementList, funcCtx.ClassName)

			// Cross-constructor/<clinit> totals: a final field assigned exactly once HERE may still
			// be assigned in another overloaded constructor. Hoisting it then double-assigns a final
			// field, so the hoist guards below additionally require the class-wide store count <= 1.
			fieldStoreTotal := c.constructorFieldStoreTotals()

			sourceCode := "\n"
			hoistableStaticInitLocals := map[string]string{}
			// Contiguous-prefix hoist barrier for <clinit>. The dumper emits every lifted field
			// initializer as a field declaration ABOVE the static{} block, so a <clinit> assignment
			// may only be lifted while every preceding top-level <clinit> statement was also lifted.
			// Once a side-effecting / non-hoistable statement stays in the static block (a loop, a
			// `someField.set(...)` call, a branch), any later field initializer that reads the state
			// those statements produced (e.g. commons-codec URLCodec `WWW_FORM_URL = (BitSet)
			// WWW_FORM_URL_SAFE.clone()` emitted after all the `WWW_FORM_URL_SAFE.set(...)` calls) must
			// stay too: lifting it would reorder the read ahead of the writes AND forward-reference a
			// field declared later. Kept as a blank-final store in the static block instead.
			// staticHoistAllowedHere defaults true so non-<clinit> bodies and the pre-barrier prefix
			// are unaffected. Interfaces/annotations (classStaticInitializersMustHoist) are EXCLUDED:
			// they cannot declare static{} blocks, so every constant initializer MUST be lifted to its
			// declaration and there is no place to leave a deferred store — the barrier only applies to
			// ordinary classes, which can hold blank-final stores in a static block.
			// Kill-switch: JDEC_NO_CLINIT_HOIST_BARRIER=1 restores the old behavior.
			clinitHoistBarrierOn := funcCtx.FunctionName == "<clinit>" && !classStaticInitializersMustHoist && os.Getenv("JDEC_NO_CLINIT_HOIST_BARRIER") == ""
			staticHoistBarrierHit := false
			staticHoistAllowedHere := true
			hoistEventCount := 0
			statementSet := utils.NewSet[statements.Statement]()
			var statementToString func(statement statements.Statement) string
			var statementListToString func(statements []statements.Statement) string
			statementListToString = func(statementList []statements.Statement) string {
				c.Tab()
				defer c.UnTab()
				var res []string
				for i, statement := range statementList {
					if _, ok := statement.(*statements.MiddleStatement); ok {
						continue
					}
					_, ok := statement.(*statements.StackAssignStatement)
					if ok {
						continue
					}
					// A static initializer block (<clinit> -> `static {}`) cannot contain a `return`
					// statement (javac: "return outside of method"). Source cannot express an early
					// return in a <clinit>, so javac never emits one; a void `return;` sitting at the
					// tail of ANY block (top-level or inside the normal-completion try body) is just
					// the terminal flow-exit opcode. Dropping it preserves semantics and yields legal
					// Java (e.g. commons-codec DaitchMokotoffSoundex's twr <clinit> rendered a bare
					// `return;` inside `try{...}` which javac rejected). Restricting to the block tail
					// avoids enabling any dead trailing siblings. Kill-switch:
					// JDEC_NO_CLINIT_RETURN_DROP=1. (Bug AC)
					if funcCtx.FunctionName == "<clinit>" && os.Getenv("JDEC_NO_CLINIT_RETURN_DROP") == "" {
						if rs, ok := statement.(*statements.ReturnStatement); ok && rs.JavaValue == nil && i == len(statementList)-1 {
							break
						}
					}
					res = append(res, statementToString(statement))
					// Drop unreachable trailing siblings: once an unconditional terminal
					// (return/throw/break/continue) is emitted, anything after it in the same
					// block is dead code that javac would reject (e.g. a synthetic `break;`
					// appended after a `return;` by the loop rewriter).
					if isUnconditionalTerminalStatement(statement, funcCtx) {
						break
					}
				}
				return strings.Join(res, "\n")
			}
			statementToString = func(statement statements.Statement) (statementStr string) {
				defer func() {
					if debugMode {
						log.Info("\n" + statementStr)
					}
				}()
				//if statementSet.Has(statement) {
				//	panic("statement already exists")
				//}
				statementSet.Add(statement)
				switch ret := statement.(type) {
				case *statements.AssignStatement:
					foundFieldInit := false
					if ret.LeftValue != nil && ret.JavaValue != nil && funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist {
						if ref, ok := ret.LeftValue.(*values.JavaRef); ok && !ref.IsThis {
							if rhs := ret.JavaValue.String(funcCtx); staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) {
								hoistableStaticInitLocals[strings.TrimSpace(ref.String(funcCtx))] = rhs
								foundFieldInit = true
								hoistEventCount++
							}
						}
					}
					if v, ok := ret.LeftValue.(*values.RefMember); ok && ret.JavaValue != nil {
						obj := core.UnpackSoltValue(v.Object)
						if v1, ok := obj.(*values.JavaRef); ok && v1.IsThis && (funcCtx.FunctionName == "<init>" || funcCtx.FunctionName == funcCtx.ClassName) {
							if _, ok := finalFieldMap[v.Member]; ok {
								if rhs := ret.JavaValue.String(funcCtx); canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
									(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[v.Member] == 1 && crossCtorStoreOK(fieldStoreTotal, v.Member) && !rhsReadsInstanceField(rhs))) {
									foundFieldInit = true
									c.fieldDefaultValue[v.Member] = rhs
								}
							}
						}
					} else if v, ok := ret.LeftValue.(*values.JavaClassMember); ok && ret.JavaValue != nil {
						if (funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist) || v.Name == funcCtx.ClassName {
							if _, ok := finalFieldMap[v.Member]; ok {
								if rhs := ret.JavaValue.String(funcCtx); staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
									(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[v.Member] <= 1 && crossCtorStoreOK(fieldStoreTotal, v.Member))) {
									foundFieldInit = true
									c.fieldDefaultValue[v.Member] = rhs
									hoistEventCount++
								}
							}
						}
					}
					if !foundFieldInit && ret.LeftValue != nil && ret.JavaValue != nil && funcCtx.FunctionName == "<clinit>" && classStaticInitializersMustHoist {
						lhs := strings.TrimSpace(ret.LeftValue.String(funcCtx))
						if strings.HasPrefix(lhs, c.GetConstructorMethodName()+".") {
							lhs = strings.TrimPrefix(lhs, c.GetConstructorMethodName()+".")
						}
						if rawName, ok := finalFieldRenderNameToRaw[lhs]; ok {
							rhs := ret.JavaValue.String(funcCtx)
							if ref, ok := values.UnpackSoltValue(ret.JavaValue).(*values.JavaRef); ok {
								if localInit, ok := hoistableStaticInitLocals[strings.TrimSpace(ref.String(funcCtx))]; ok {
									rhs = localInit
								}
							}
							if staticHoistAllowedHere && canHoistFieldValueInitializer(ret.JavaValue, rhs) &&
								(!EnableFieldInitHoistGuard || (ctorFieldAssignCount[rawName] <= 1 && crossCtorStoreOK(fieldStoreTotal, rawName))) {
								foundFieldInit = true
								c.fieldDefaultValue[rawName] = rhs
								hoistEventCount++
							}
						}
					}
					if !foundFieldInit {
						statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
					}
				case *statements.SynchronizedStatement:
					// A field lock desugars to `getfield; dup; astore tmp; monitorenter`; the
					// synthetic temp backs the implicit finally's monitorexit. After the
					// synchronized rewriter removes that monitorexit the temp is dead, but it
					// survives in the monitor position as an inline `tmp = lock` assignment,
					// which references an undeclared variable. Strip it back to the lock
					// expression (safe: the temp has no other use).
					arg := monitorTempAssignRe.ReplaceAllString(ret.Argument.String(funcCtx), "$1")
					statementStr = fmt.Sprintf(c.GetTabString()+"synchronized(%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", arg, statementListToString(ret.Body))
				case *statements.TryCatchStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"try{\n"+
						"%s\n"+
						c.GetTabString()+"}", statementListToString(ret.TryBody))
					// Two catch handlers of the SAME type are illegal Java (a try may not declare two
					// handlers of the same exception type), but they are exactly what bytecode emits for
					// try-with-resources / try-catch-finally: a Throwable primaryExc-capture handler whose
					// region is nested inside a Throwable cleanup ("any") handler. Collapse such adjacent
					// pairs back into one handler so the source recompiles. Kill-switch:
					// JDEC_NO_CATCH_MERGE=1 restores the raw duplicate-catch output.
					catchExc := ret.Exception
					catchBodies := ret.CatchBodies
					if os.Getenv("JDEC_NO_CATCH_MERGE") == "" {
						catchExc, catchBodies = mergeNestedSameTypeCatches(funcCtx, catchExc, catchBodies)
					}
					for i, body := range catchBodies {
						excType := normalizeCatchClauseType(catchExc[i].Type().String(funcCtx))
						statementStr += fmt.Sprintf("catch(%s %s){\n"+
							"%s\n"+
							c.GetTabString()+"}", excType, catchExc[i].String(funcCtx), statementListToString(body))
					}
					haveCatch := len(catchBodies) > 0
					if !haveCatch {
						body := statementListToString(ret.TryBody)
						if canFlattenNoCatchTry(body) {
							// A try without catch/finally has no Java-level effect. Some legacy bytecode
							// patterns (for example an EOFException edge inside a loop with an enclosing
							// IOException handler) can lose the inner handler during CFG structuring while
							// the body itself is still sound. Preserve the executable statements instead of
							// stubbing the method.
							statementStr = body
						} else {
							// A try with no catch/finally is malformed (structuring failed). Emit the
							// internal marker so the method degrades to a stub rather than leaking the
							// broken body that produced this bare try.
							statementStr += "catch(Exception e) { throw e; /* " + malformedTryNoCatchMarker + " */ }"
						}
					}
				case *statements.WhileStatement:
					statementStr = fmt.Sprintf(c.GetTabString()+"while (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx), statementListToString(ret.Body))
				case *statements.DoWhileStatement:
					body := normalizeDoWhileBreakGuardSource(statementListToString(statements.NormalizeDoWhileDecrementGuard(ret.Body, funcCtx)))
					statementStr = fmt.Sprintf(c.GetTabString()+"do{\n"+
						"%s\n"+
						c.GetTabString()+"} while (%s);", body, values.SimplifyConditionValue(ret.ConditionValue).String(funcCtx))
					if ret.Label != "" {
						statementStr = fmt.Sprintf("%s%s:\n%s", c.GetTabString(), ret.Label, statementStr)
					}
				case *statements.SwitchStatement:
					getBody := func(caseItems []*statements.CaseItem) string {
						var res []string
						for _, st := range caseItems {
							if st.IsDefault {
								res = append(res, c.GetTabString()+fmt.Sprintf("default:\n%s", statementListToString(st.Body)))
								continue
							}
							res = append(res, c.GetTabString()+fmt.Sprintf("case %d:\n%s", st.IntValue, statementListToString(st.Body)))
						}
						return strings.Join(res, "\n")
					}
					statementStr = fmt.Sprintf(c.GetTabString()+"switch (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", ret.Value.String(funcCtx), getBody(ret.Cases))
				case *statements.IfStatement:
					if isEmptyAssertionsDisabledGuard(ret, funcCtx) {
						statementStr = ""
						break
					}
					if stmt := buildReturnFromEmptyGuardTernary(ret, funcCtx); stmt != "" {
						statementStr = c.GetTabString() + stmt + ";"
						break
					}
					// Recover short-circuit boolean returns: when a method returns boolean and the
					// if-then is empty (or only a `return true`) while the else is `return expr`,
					// rewrite to `return condition || expr`. This is the simplest case of the
					// boolean short-circuit DAG where the true arm shares a constant leaf.
					if isBoolReturnIfElse(ret, funcCtx) {
						if stmt := buildBoolReturnFromIfElse(ret, funcCtx); stmt != "" {
							statementStr = c.GetTabString() + stmt + ";"
							break
						}
					}
					statementStr = fmt.Sprintf(c.GetTabString()+"if (%s){\n"+
						"%s\n"+
						c.GetTabString()+"}", values.SimplifyConditionValue(ret.Condition).String(funcCtx), statementListToString(ret.IfBody))
					if len(ret.ElseBody) > 0 {
						statementStr += fmt.Sprintf("else{\n"+
							"%s\n"+
							c.GetTabString()+"}", statementListToString(ret.ElseBody))
					}
				case *statements.ReturnStatement:
					statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
				case *statements.ForStatement:
					datas := []string{}
					datas = append(datas, ret.InitVar.String(funcCtx))
					datas = append(datas, fmt.Sprintf("%s", values.SimplifyConditionValue(ret.Condition.Condition).String(funcCtx)))
					datas = append(datas, ret.EndExp.String(funcCtx))
					var lines []string
					for _, subStatement := range ret.SubStatements {
						lines = append(lines, c.GetTabString()+"\t"+subStatement.String(funcCtx)+";")
					}
					s := fmt.Sprintf("%sfor(%s; %s; %s) {\n%s\n%s}", c.GetTabString(), datas[0], datas[1], datas[2], strings.Join(lines, "\n"), c.GetTabString())
					statementStr = s
				default:
					statementStr = c.GetTabString() + statement.String(funcCtx) + ";"
				}
				return statementStr
			}
			statementCodes := []string{}
			supperInvokeStr := ""
			for i, statement := range statementList {
				if i == len(statementList)-1 && methodType.FunctionType().ReturnType.String(funcCtx) == "void" {
					if _, ok := statement.(*statements.ReturnStatement); ok {
						continue
					}
				}
				if v, ok := statement.(*statements.ExpressionStatement); ok {
					if v1, ok := v.Expression.(*values.FunctionCallExpression); ok && v1.IsSupperConstructorInvoke(funcCtx) {
						supperInvokeStr = fmt.Sprintf("%s\n", statementToString(statement))
						continue
					}
				}
				if clinitHoistBarrierOn {
					staticHoistAllowedHere = !staticHoistBarrierHit
				}
				hoistBefore := hoistEventCount
				statementStr := statementToString(statement)
				if clinitHoistBarrierOn && !staticHoistBarrierHit {
					switch statement.(type) {
					case *statements.MiddleStatement, *statements.StackAssignStatement:
						// Structural markers are skipped from the body (see statementListToString);
						// treat them as transparent so they never trip the barrier.
					default:
						// A top-level <clinit> statement that produced no hoist event stays in the
						// static block; from here on nothing may be lifted ahead of it.
						if hoistEventCount == hoistBefore {
							staticHoistBarrierHit = true
						}
					}
				}
				if statementStr == "" {
					continue
				}
				statementCodes = append(statementCodes, fmt.Sprintf("%s\n", statementStr))
			}

			if isEnumCtor {
				// The only super() call in an enum constructor is the synthetic
				// super(name, ordinal); enum constructors cannot call super explicitly.
				supperInvokeStr = ""
			}
			if name != "<init>" && name != "<clinit>" &&
				needsTrailingIncompleteControlFlowThrow(statementList, methodType.FunctionType().ReturnType, funcCtx) {
				statementCodes = append(statementCodes, fmt.Sprintf("%sthrow new RuntimeException(\"incomplete control flow\");\n", c.GetTabString()))
			}
			sourceCode += supperInvokeStr + strings.Join(statementCodes, "")
			receiverType := ""
			if !funcCtx.IsStatic && name != "<clinit>" {
				receiverType = c.GetConstructorMethodName()
			}
			sourceCode = hoistSameTypeEscapedLocals(sourceCode)
			sourceCode = hoistCastGuardedEscapedLocals(sourceCode)
			methodReturnTypeStr := ""
			if mt := methodType.FunctionType(); mt != nil && mt.ReturnType != nil {
				methodReturnTypeStr = mt.ReturnType.String(funcCtx)
			}
			sourceCode = addMissingGeneratedLocalDecls(sourceCode, paramsNewStr, receiverType, c.methodReturnTypeByName(), methodReturnTypeStr)
			code = sourceCode
		}
	}
	c.UnTab()

	if paramsNewStr == "" && abstractMethod {
		paramList := []string{}
		// fetch from method type
		paramTypes := methodType.FunctionType().ParamTypes
		// An ABSTRACT method's parameters must NOT standalone-erase an enclosing type variable to Object:
		// a no-own-formal sibling override (guava AbstractMapBasedMultimap$1.output(K,V), K/V its own
		// injected params) would then clash with the erased `output(Object,Object)`. Keeping the bare
		// (undeclared) variable is no worse than before the erasure existed. Restored right after.
		prevSuppress := funcCtx.SuppressStandaloneErase
		funcCtx.SuppressStandaloneErase = true
		for idx, t := range paramTypes {
			typeName := t.String(funcCtx)
			// 末参为 varargs 时必须渲染成「元素类型 + ...」(如 Feature...), 不能是「数组类型 + ...」
			// (Feature[]...)。后者会被 javac 当成 Feature[] 的 varargs (descriptor [[LFeature;), 与子类
			// 重写的 Feature... (descriptor [LFeature;) 不再 override-equivalent → 子类报「is not abstract
			// and does not override」。这里此前漏掉了 ElementType 剥离 (拼接式方法/lambda/stub 路径都对),
			// 是 fastjson2 JSONPath.set 等抽象 varargs 方法的整族重编译失败根因。
			if isVarArgs && idx == len(paramTypes)-1 && t.IsArray() && os.Getenv("JDEC_VARARGS_ABSTRACT_FIX_OFF") == "" {
				paramList = append(paramList, fmt.Sprintf("%s... var%d", t.ElementType().String(funcCtx), idx))
			} else if isVarArgs && idx == len(paramTypes)-1 {
				paramList = append(paramList, fmt.Sprintf("%s... var%d", typeName, idx))
			} else {
				paramList = append(paramList, fmt.Sprintf("%s var%d", typeName, idx))
			}
		}
		funcCtx.SuppressStandaloneErase = prevSuppress
		paramsNewStr = strings.Join(paramList, ", ")
	}
	if isLambda {
		// A lambda arrow body is spliced inline into the enclosing method. Lift its own locals into a
		// private `lv<seq>_N` namespace so they never shadow an enclosing local/parameter or a captured
		// variable (Java: "variable varN is already defined in method"). Nested lambda bodies were
		// dumped earlier and already carry their own `lv<innerseq>` names, so this outer rewrite (which
		// only matches `varN`) leaves them untouched.
		c.lambdaLocalSeq++
		code = renameLambdaBodyLocals(code, c.lambdaLocalSeq)
		// When EVERY lambda parameter is unused in the body, render them WITHOUT explicit types
		// (implicit `(l0, l1) -> ...`). An explicit parameter type recovered from the impl-method
		// descriptor clashes with a RAW functional-interface target (fastjson2
		// `rawMap.computeIfAbsent(k, (Integer l0) -> new ArrayList())`: the raw `Function` SAM is
		// `apply(Object)`, so `(Integer l0)` is rejected as "incompatible parameter types in lambda
		// expression"). Letting javac infer the parameter types from the target is both more faithful
		// (this is the idiomatic source form) and lets the lambda bind. Restricted to the
		// ALL-UNUSED case so a body that references a parameter as a specific type keeps its explicit
		// declaration (no behavioral change, no overload-disambiguation risk).
		// Kill-switch: JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF=1.
		if len(lambdaParamNames) > 0 && os.Getenv("JDEC_LAMBDA_IMPLICIT_UNUSED_PARAM_OFF") == "" && !lambdaParamsUsed(code, lambdaParamNames) {
			paramsNewStr = strings.Join(lambdaParamNames, ", ")
		}
		res := fmt.Sprintf("(%s) -> {%s", paramsNewStr, code)
		res += strings.Repeat("\t", c.TabNumber()) + "}"
		dumped.methodName = name
		dumped.code = res
		dumped.bodyCode = code
		return dumped, nil
	}
	if name == "<clinit>" && strings.TrimSpace(code) == "" {
		dumped.methodName = name
		dumped.code = ""
		dumped.bodyCode = code
		return dumped, nil
	}
	methodSourceBuffer := strings.Builder{}
	isInterfaceType := slices.Contains(c.obj.AccessFlagsVerbose, "interface")
	writeAccessFlags := func(buffer io.Writer) {
		if accessFlags != "" {
			methodSourceBuffer.Write([]byte(accessFlags + " "))
		}
		// A non-abstract, non-static instance method declared in an interface is a default
		// method and must carry the `default` keyword, otherwise javac rejects the body.
		if isInterfaceType && !abstractMethod && name != "<init>" && name != "<clinit>" && !strings.Contains(accessFlags, "static") {
			methodSourceBuffer.Write([]byte("default "))
		}
	}
	writeName := func(buffer io.Writer) {
		if name == "<init>" {
			methodSourceBuffer.Write([]byte(c.GetConstructorMethodName()))
		} else {
			methodSourceBuffer.Write([]byte(class_context.SafeIdentifier(name)))
		}
	}
	writeArguments := func(buffer io.Writer) {
		methodSourceBuffer.Write([]byte(fmt.Sprintf("(%s)%s", paramsNewStr, exceptions)))
	}
	// A synthetic access-bridge constructor `C(C$N marker)` whose body decompiled to empty bridges the
	// PRIVATE no-arg ctor `C()` via `this()` (the trailing anonymous marker only disambiguates the
	// signature from `C()`); the decompiler strips that no-arg `this()` delegation. Leaving the body
	// empty makes javac insert an implicit `super()` instead -- which, when the SUPERCLASS's no-arg ctor
	// is private/absent, fails "constructor ... has private access" (guava AbstractFuture$UnsafeAtomicHelper
	// / $SynchronizedHelper / AggregateFutureState$SynchronizedAtomicHelper all extend a private-no-arg
	// AtomicHelper). The base class survives only because it extends Object (implicit super() == Object()).
	// Emitting the faithful `this()` delegation is correct for BOTH (it re-routes through the same-class
	// no-arg ctor the bridge actually targets). Restricted to MARKER-ONLY bridges (single param) so an
	// arg-forwarding bridge `C(int,C$N){ this(x); }` is never mis-rendered as no-arg `this()`. Kill-switch:
	// JDEC_SYN_BRIDGE_THIS_OFF=1.
	emitBridgeThisCall := name == "<init>" && strings.TrimSpace(code) == "" && os.Getenv("JDEC_SYN_BRIDGE_THIS_OFF") == "" &&
		c.isSyntheticAccessBridgeCtor(descriptor, method.AccessFlags) &&
		len(methodParamFieldDescriptors(descriptor)) == 1
	writeBlock := func(buffer io.Writer) {
		if abstractMethod {
			// An abstract method of an @interface is an annotation element; if it carries an
			// AnnotationDefault attribute we must re-emit its `default <value>` clause, otherwise
			// any use site that omits the element fails javac ("missing a default value").
			methodSourceBuffer.Write([]byte(c.annotationElementDefaultClause(method) + ";"))
		} else if emitBridgeThisCall {
			methodSourceBuffer.Write([]byte(fmt.Sprintf(" {\n%sthis();\n%s}",
				strings.Repeat("\t", c.TabNumber()+1), strings.Repeat("\t", c.TabNumber()))))
		} else if code == "" {
			methodSourceBuffer.Write([]byte(" {}"))
		} else {
			body := fmt.Sprintf(" {%s%s}", code, strings.Repeat("\t", c.TabNumber()))
			methodSourceBuffer.WriteString(body)
		}
	}
	writeReturnType := func(buffer io.Writer) {
		methodSourceBuffer.Write([]byte(returnTypeStr + " "))
	}
	// writeMethodTypeParams emits a generic method's own formal type-parameter declaration ("<T> ")
	// after the access flags and before the return type, e.g. `public static <T> T checkNotNull(T x)`.
	writeMethodTypeParams := func(buffer io.Writer) {
		if methodTypeParams != "" {
			methodSourceBuffer.Write([]byte(methodTypeParams + " "))
		}
	}
	var writerSeq []func(io.Writer)
	switch name {
	case "<init>":
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeMethodTypeParams,
			writeName,
			writeArguments,
			writeBlock,
		}
	case "<clinit>":
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeBlock,
		}
	default:
		writerSeq = []func(io.Writer){
			writeAccessFlags,
			writeMethodTypeParams,
			writeReturnType,
			writeName,
			writeArguments,
			writeBlock,
		}
	}
	methodSource := ""
	for _, writer := range writerSeq {
		writer(&methodSourceBuffer)
	}
	methodSource = methodSourceBuffer.String()
	if len(annoStrs) == 0 {
		dumped.code = methodSource
		dumped.methodName = name
		dumped.bodyCode = code
		return dumped, nil
	} else {
		c.Tab()
		annoStr := strings.Join(annoStrs, c.GetTabString()+"\n")
		c.UnTab()
		originCode := annoStr + "\n" + c.GetTabString() + methodSource
		dumped.code = originCode
		dumped.methodName = name
		dumped.bodyCode = code
		return dumped, nil
	}
}

type dumpedMethods struct {
	methodName string
	code       string
	bodyCode   string
	// member/descriptor are retained so the post-decompile syntax safety net can rebuild a
	// stub for a method whose generated body turns out to be un-parseable.
	member     *MemberInfo
	descriptor string
}

// javaFloatLiteral renders a float constant as a valid Java float literal (with the
// mandatory 'F' suffix), handling NaN/Infinity which have no plain literal form.
// localDeclVarId returns the VariableId of a local-variable value (var0, var1, ...),
// or nil for `this`, fields, statics, or values that do not render via their slot id.
func localDeclVarId(v values.JavaValue) *utils2.VariableId {
	if v == nil {
		return nil
	}
	ref, ok := values.UnpackSoltValue(v).(*values.JavaRef)
	if !ok || ref == nil || ref.IsThis || ref.Id == nil {
		return nil
	}
	// CustomValue/StackVar refs do not render via the slot id, so renaming the id would not
	// change the emitted text; skip them.
	if ref.CustomValue != nil || ref.StackVar != nil {
		return nil
	}
	return ref.Id
}

// declareLocalInScope records a local declaration in the current scope, renaming it when its
// generated name (varN, derived from slot depth) already belongs to a *different* variable
// that is still live in an enclosing scope. The JVM reuses local slots, so two distinct
// variables in nested source scopes can collapse to the same varN, which javac rejects
// ("variable varN is already defined"). The rename uses a `_<n>` suffix the decompiler never
// generates, guaranteeing it cannot clash with a real slot name.
func declareLocalInScope(id *utils2.VariableId, live map[string]*utils2.VariableId) {
	if id == nil {
		return
	}
	name := id.String()
	if existing, ok := live[name]; ok && existing != id {
		for i := 1; ; i++ {
			cand := fmt.Sprintf("%s_%d", name, i)
			if _, taken := live[cand]; !taken {
				id.SetName(cand)
				name = cand
				break
			}
		}
	}
	live[name] = id
}

// declareCatchParamInScope registers a catch parameter in its catch-block scope, resolving the two
// distinct ways its generated name can collide with an enclosing local. Java forbids a catch
// parameter from shadowing a variable declared in an enclosing block, yet JVM slot reuse routinely
// gives a catch parameter and an unrelated local the same var<slot> name.
//
//   - Distinct ids, same printed name: the catch parameter owns its VariableId and merely renders the
//     same varN as a still-live enclosing local. declareLocalInScope renames it in place (its own id
//     gets a `_<n>` suffix). This is the common case and matches the pre-existing behavior.
//   - Shared id: slot-reuse variable merging unified the catch slot with an enclosing local that
//     occupies the same JVM slot, so they share ONE VariableId AND, in practice, the same JavaRef
//     OBJECT (the decompiler reuses one ref per slot, repointed in place by the rewriter). Renaming
//     the shared id - or mutating that ref's Id - would rename every other use of the enclosing local
//     too, leaving the clash in place and corrupting unrelated lines. The catch parameter is instead
//     split off by replacing only the exception SLICE ENTRY with a fresh clone ref that carries a new,
//     uniquely-named id. The shared object is left untouched, so the enclosing local is unaffected and
//     only the printed catch-parameter name changes.
func declareCatchParamInScope(exSlot **values.JavaRef, enclosing, inner map[string]*utils2.VariableId) {
	ex := *exSlot
	id := localDeclVarId(ex)
	if id == nil {
		return
	}
	if existing, ok := enclosing[id.String()]; ok && existing == id {
		fresh := &utils2.VariableId{}
		fresh.SetName(freshScopedName(id.String(), enclosing, inner))
		clone := *ex
		clone.Id = fresh
		*exSlot = &clone
		inner[fresh.String()] = fresh
		return
	}
	declareLocalInScope(id, inner)
}

// freshScopedName returns base with the first `_<n>` suffix that is unused in both scope maps.
func freshScopedName(base string, a, b map[string]*utils2.VariableId) string {
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if _, taken := a[cand]; taken {
			continue
		}
		if _, taken := b[cand]; taken {
			continue
		}
		return cand
	}
}

func cloneScope(live map[string]*utils2.VariableId) map[string]*utils2.VariableId {
	out := make(map[string]*utils2.VariableId, len(live)+4)
	for k, v := range live {
		out[k] = v
	}
	return out
}

// resolveLocalNameCollisions walks the method body in lexical-scope order and renames any
// local declaration whose slot-derived name collides with a still-live variable from an
// enclosing scope (see declareLocalInScope). Renaming only fires on a genuine collision, so
// output for the overwhelmingly common non-colliding case is byte-for-byte unchanged. This
// fixes nested catch parameters and reused slots that javac would otherwise reject.
func resolveLocalNameCollisions(params []values.JavaValue, body []statements.Statement) {
	live := map[string]*utils2.VariableId{}
	for _, p := range params {
		if id := localDeclVarId(p); id != nil {
			live[id.String()] = id
		}
	}
	renameStatementsInScope(body, live)
}

// paramDescriptorNarrowType returns the parameter's authoritative descriptor type when that type is a
// narrow integer-category primitive (char/byte/short) but the inferred slot type has been widened to
// int. It returns nil otherwise. The JVM stores char/byte/short locals in int-sized slots and an
// in-body reassignment (e.g. `var2 = 65535`) makes the decompiler infer the slot as int; declaring the
// parameter as int then breaks a same-typed field/return assignment with "possible lossy conversion".
// The method descriptor is the ground truth for a primitive parameter's declared type, so we trust it.
// Kill-switch: JDEC_PARAM_DESC_NARROW_OFF.
func paramDescriptorNarrowType(descTypes []types.JavaType, idx int, inferred types.JavaType) types.JavaType {
	if os.Getenv("JDEC_PARAM_DESC_NARROW_OFF") != "" {
		return nil
	}
	if idx < 0 || idx >= len(descTypes) || descTypes[idx] == nil || inferred == nil {
		return nil
	}
	dp, ok := descTypes[idx].RawType().(*types.JavaPrimer)
	if !ok {
		return nil
	}
	switch dp.Name {
	case types.JavaChar, types.JavaByte, types.JavaShort:
	default:
		return nil
	}
	ip, ok := inferred.RawType().(*types.JavaPrimer)
	if !ok || ip.Name != types.JavaInteger {
		return nil
	}
	return descTypes[idx]
}

// boxedPrimitiveLambdaParamTypes are the java.lang wrapper classes that appear as lambda parameters
// with NO lost type arguments -- rendering them explicitly always matches the SAM and, crucially, an
// explicit `(Integer l0)` DRIVES a generic call's type-variable inference (Collections.sort's T) that a
// bare `(l0)` would leave as Object. They must therefore stay explicit (see lambdaParamsShouldBeImplicit).
var boxedPrimitiveLambdaParamTypes = map[string]bool{
	"java.lang.Integer":   true,
	"java.lang.Long":      true,
	"java.lang.Short":     true,
	"java.lang.Byte":      true,
	"java.lang.Character": true,
	"java.lang.Boolean":   true,
	"java.lang.Float":     true,
	"java.lang.Double":    true,
}

// lambdaParamsShouldBeImplicit decides whether a lambda arrow renders its parameters WITHOUT explicit
// types. The bytecode only preserves the ERASED impl-method descriptor, so a GENERIC parameter surfaces
// raw (`Predicate` for `Predicate<String>`) and an explicit `(Predicate l0) -> ...` fails to bind
// against the parameterized SAM -- those must go implicit. But a PRIMITIVE or BOXED-primitive parameter
// (`int`, `Integer`) carries no lost type argument: explicit is always assignable AND is what lets a
// generic call infer its type variable (e.g. `Collections.sort((List)(v), (Integer x, Integer y) ->
// ...)` infers T=Integer; made implicit x/y infer Object and `x.intValue()` stops resolving -- the
// synthetic round-trip guard). So go implicit only when at least one parameter is a "real" (generic-
// capable) reference type; keep everything explicit when every parameter is primitive/boxed.
func (c *ClassObjectDumper) lambdaParamsShouldBeImplicit(samParams []values.JavaValue) bool {
	sawGenericCapable := false
	for _, val := range samParams {
		if val == nil {
			continue
		}
		t := val.Type()
		if t == nil {
			continue
		}
		if p, ok := t.RawType().(*types.JavaPrimer); ok {
			switch p.Name {
			case types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger,
				types.JavaLong, types.JavaFloat, types.JavaDouble, types.JavaBoolean:
				continue // a genuine primitive lambda param (IntPredicate etc.): explicit is exact
			}
		}
		if fqn, ok := types.ClassFQNOf(t); ok && boxedPrimitiveLambdaParamTypes[fqn] {
			continue // boxed primitive: explicit matches and drives generic inference
		}
		sawGenericCapable = true
	}
	return sawGenericCapable
}

func ensureUniqueParameterNames(params []values.JavaValue, funcCtx *class_context.ClassContext) {
	seen := map[string]bool{}
	for i, p := range params {
		name := ""
		if p != nil {
			name = p.String(funcCtx)
		}
		if name == "" || seen[name] {
			id := localDeclVarId(p)
			if id == nil {
				continue
			}
			base := name
			if base == "" {
				base = fmt.Sprintf("var%d", i)
			}
			for suffix := 1; ; suffix++ {
				candidate := fmt.Sprintf("%s_%d", base, suffix)
				if !seen[candidate] {
					id.SetName(candidate)
					name = candidate
					break
				}
			}
		}
		seen[name] = true
	}
}

func renameStatementsInScope(stmts []statements.Statement, live map[string]*utils2.VariableId) {
	for _, st := range stmts {
		switch s := st.(type) {
		case *statements.AssignStatement:
			if (s.IsFirst || s.IsDeclare) && s.ArrayMember == nil {
				declareLocalInScope(localDeclVarId(s.LeftValue), live)
			}
		case *statements.IfStatement:
			renameStatementsInScope(s.IfBody, cloneScope(live))
			renameStatementsInScope(s.ElseBody, cloneScope(live))
		case *statements.DoWhileStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.WhileStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.ForStatement:
			inner := cloneScope(live)
			if s.InitVar != nil {
				renameStatementsInScope([]statements.Statement{s.InitVar}, inner)
			}
			renameStatementsInScope(s.SubStatements, inner)
		case *statements.SwitchStatement:
			// Java switch cases share a single block scope (fallthrough), so declarations in
			// one case are visible to later cases: use one shared inner scope.
			inner := cloneScope(live)
			for _, c := range s.Cases {
				renameStatementsInScope(c.Body, inner)
			}
		case *statements.SynchronizedStatement:
			renameStatementsInScope(s.Body, cloneScope(live))
		case *statements.TryCatchStatement:
			renameStatementsInScope(s.TryBody, cloneScope(live))
			for i, body := range s.CatchBodies {
				inner := cloneScope(live)
				if i < len(s.Exception) && s.Exception[i] != nil {
					declareCatchParamInScope(&s.Exception[i], live, inner)
				}
				renameStatementsInScope(body, inner)
			}
		}
	}
}

// localSlotRefRe matches a decompiler-generated local/parameter reference (var0, var1, ...) INCLUDING
// the collision-renamed form `varN_M` (resolveLocalNameCollisions disambiguates two same-slot-name
// locals as varN / varN_1). `this`, instance fields (this.x), and static members (Class.x) never
// render this way, so a match means the expression depends on a method-scoped value. The `(?:_\d+)*`
// suffix is load-bearing: the bare `\bvar\d+\b` failed to match `var7_1` (the `_` after the digits is
// a word char, so there is no `\b` there), letting a final field assigned from a renamed constructor
// local (`this.hashCode64 = var7_1`) be wrongly lifted to `final long hashCode64 = var7_1;` -> the
// initializer references an out-of-scope local (fastjson2 SymbolTable.hashCode64 / FactoryFunction.function).
var localSlotRefRe = regexp.MustCompile(`\bvar\d+(?:_\d+)*\b`)
var generatedLocalRefRe = regexp.MustCompile(`\bvar\d+(?:_\d+)?\b`)

// lambdaLocalRe captures the numeric tail of a slot-derived local reference (var9, var9_1) so a
// lambda body's own locals can be lifted into a private namespace. It is applied ONLY to a fully
// rendered lambda arrow body, where every other `varN`-shaped token has already been resolved away:
// lambda parameters were renamed to `l0,l1,...`, captured variables to `\x00LCAP%d\x00` placeholders,
// and any nested lambda body already carries its own `lv<seq>_` names. What remains is exactly the
// lambda's own locals, which must not shadow the enclosing scope they are spliced into.
var lambdaLocalRe = regexp.MustCompile(`\bvar(\d+(?:_\d+)?)\b`)

// renameLambdaBodyLocals rewrites a rendered lambda body's own local references from the slot-derived
// `varN` form into a per-lambda `lv<seq>_N` namespace that the enclosing slot/parameter schemes never
// produce. This is the structural fix for "variable varN is already defined in method": an inlined
// lambda arrow body cannot legally declare a local that shadows an enclosing local/parameter or a
// captured variable (which resolves to the enclosing `varN`). Kill-switch: JDEC_NO_LAMBDA_LOCAL_RENAME.
func renameLambdaBodyLocals(body string, seq int) string {
	if os.Getenv("JDEC_NO_LAMBDA_LOCAL_RENAME") != "" {
		return body
	}
	prefix := fmt.Sprintf("lv%d_", seq)
	return lambdaLocalRe.ReplaceAllString(body, prefix+"$1")
}

// lambdaParamsUsed reports whether any of the given lambda parameter names (l0,l1,...) appears as a
// word-bounded token in the rendered lambda body. The `lN` names are assigned exclusively to lambda
// parameters (DumpMethodWithInitialId) and the body's own locals were renamed to `lv<seq>_N`, so a
// word-boundary match is unambiguous. Used to decide whether the parameters can be rendered implicitly.
func lambdaParamsUsed(body string, names []string) bool {
	for _, n := range names {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(n) + `\b`)
		if re.MatchString(body) {
			return true
		}
	}
	return false
}

// The optional `(?:\s*\.\.\.)?` after the type recognizes a varargs parameter declaration
// (`int[]... var0`, `String... var1`). Without it the ellipsis broke the `Type varN` match, so a
// varargs parameter was treated as an UNDECLARED local and addMissingGeneratedLocalDecls injected a
// bogus `Object varN = null;` that shadowed the real parameter (guava Ints.concat / Longs.concat).
var generatedLocalDeclRe = regexp.MustCompile(`\b(?:boolean|byte|char|short|int|long|float|double|String|[A-Za-z_$][A-Za-z0-9_$.<>?,]*(?:\[\])*)(?:\s*\.\.\.)?\s+(var\d+(?:_\d+)?)\b`)
var mismatchedDoWhileIndexDeclRe = regexp.MustCompile(`int\s+(var\d+(?:_\d+)?)\s*=\s*0;\n(\s*)do\{\n(\s*)if \(\((var\d+)\) <`)

// monitorTempAssignRe matches a dead synthetic monitor temp left in the synchronized()
// argument position, e.g. `var2 = this.lock`, capturing the lock expression itself.
var monitorTempAssignRe = regexp.MustCompile(`^var\d+ = (.+)$`)
var doWhileBreakGuardRe = regexp.MustCompile(`^(\s*)if \(([^\n{}]*)\)\{\n\s*break;\n\s*\}else\{`)

// methodReturnTypeByName builds (and caches) a same-class method-name -> rendered-return-type map.
// Constructors and void methods are skipped; a name overloaded with conflicting return types is
// dropped so the safety net never guesses wrong. The return types are read from the authoritative
// method descriptors, not by parsing rendered source.
func (c *ClassObjectDumper) methodReturnTypeByName() map[string]string {
	if c.methodReturnTypes != nil {
		return c.methodReturnTypes
	}
	m := map[string]string{}
	ambiguous := map[string]bool{}
	for _, info := range c.obj.Methods {
		name, err := c.obj.getUtf8(info.NameIndex)
		if err != nil || name == "<init>" || name == "<clinit>" {
			continue
		}
		descriptor, err := c.obj.getUtf8(info.DescriptorIndex)
		if err != nil {
			continue
		}
		mt, perr := types.ParseMethodDescriptor(descriptor)
		if perr != nil || mt == nil {
			continue
		}
		ret := mt.FunctionType().ReturnType.String(c.FuncCtx)
		if ret == "" || ret == "void" {
			continue
		}
		if existing, ok := m[name]; ok && existing != ret {
			ambiguous[name] = true
			continue
		}
		m[name] = ret
	}
	for name := range ambiguous {
		delete(m, name)
	}
	c.methodReturnTypes = m
	return m
}

func addMissingGeneratedLocalDecls(body, params, receiverType string, methodReturnTypes map[string]string, methodReturnType string) string {
	body = repairMismatchedDoWhileIndexDecls(body)
	returnDeclFix := os.Getenv("JDEC_RETURN_DECL_FIX_OFF") == ""
	declared := map[string]bool{}
	for _, match := range generatedLocalDeclRe.FindAllStringSubmatch(params+"\n"+body, -1) {
		if len(match) > 1 {
			// `return varN;` / `throw varN;` match generatedLocalDeclRe's type-identifier alternative
			// (the keyword `return` looks like a type token) but are USES, not declarations. Counting
			// them as declarations masked a genuinely-undeclared local whose ONLY definition is an
			// embedded `(varN = expr)` baked into a condition (fastjson2 JSONReaderJSONB
			// readLocalDateTime12/14/16 family: `if ((var2 = DateUtils.parse...) == null){}else{return
			// var2;}` -> var2 never declared -> `cannot find symbol`). Skip keyword-led matches so such
			// a local is recognized as missing and gets a synthesized declaration below.
			if returnDeclFix && castEscapeTypeKeywords[castEscapeFirstToken(match[0])] {
				continue
			}
			declared[match[1]] = true
		}
	}
	// Every varN token in the rendered parameter list IS, by construction, a declared parameter
	// (the param list contains only `Type varN` pairs; a type's generic args never contain a `var<digits>`
	// token). Relying solely on generatedLocalDeclRe to spot them is brittle: its type prefix cannot match
	// a parameter whose final type token before the name is a wildcard (`Map<?, ?> var2` -> the token `?>`
	// has no leading identifier char) or otherwise carries spaces, so such a parameter looked UNDECLARED
	// and a bogus `Object varN = null;` was injected that shadowed it (guava base Joiner$MapJoiner: every
	// `appendTo(StringBuilder, Map<?, ?>)` / `join(Iterable<? extends Entry<?, ?>>)` body). Mark all
	// parameter slot names declared directly so wildcard-typed parameters are never re-declared.
	for _, name := range generatedLocalRefRe.FindAllString(params, -1) {
		declared[name] = true
	}
	// generatedLocalDeclRe's type token forbids spaces, so a declaration whose type is a
	// multi-argument / wildcard generic (`Foo<K, V, ? extends Bar<K, V, ?>> var4 = ...`) is NOT
	// recognized: the run immediately before `var4` is `?>>`, which does not start with an identifier
	// char, and no earlier start can bridge the internal spaces. The local then looks undeclared and a
	// bogus `Object var4 = null;` is injected that duplicates the real declaration (javac "variable
	// var4 is already defined"; guava MapMakerInternalMap$StrongKeyWeakValueSegment /
	// $WeakKeyWeakValueSegment.setWeakValueReferenceForTesting, whose second parameter type is
	// `WeakValueReference<K, V, ? extends InternalEntry<K, V, ?>>`). castEscapeDeclLineRe is the
	// space-tolerant, line-anchored declaration matcher already used by the escaped-local hoisters;
	// scan each rendered line with it (skipping keyword-led pseudo-types like `return varN;`) so such
	// generic declarations are recognized. This is purely additive to `declared`: it can only SUPPRESS
	// a bogus phantom, never inject one. Kill-switch: JDEC_GENERIC_DECL_DETECT_OFF=1.
	if os.Getenv("JDEC_GENERIC_DECL_DETECT_OFF") == "" {
		for _, ln := range strings.Split(body, "\n") {
			m := castEscapeDeclLineRe.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			if castEscapeTypeKeywords[castEscapeFirstToken(m[2])] {
				continue
			}
			declared[m[3]] = true
		}
	}
	missing := []string{}
	seen := map[string]bool{}
	for _, name := range generatedLocalRefRe.FindAllString(body, -1) {
		if (name != "var0" || receiverType == "") && declared[name] || seen[name] {
			continue
		}
		seen[name] = true
		missing = append(missing, name)
	}
	if len(missing) == 0 {
		return body
	}
	sort.Slice(missing, func(i, j int) bool {
		return missing[i] < missing[j]
	})
	lines := make([]string, 0, len(missing))
	for _, name := range missing {
		typ, zero := "Object", "null"
		if name == "var0" && receiverType != "" {
			typ, zero = receiverType, "this"
		} else if generatedLocalLooksInt(body, name) {
			typ, zero = "int", "0"
		} else if rt := inferGeneratedLocalRefType(body, params, name, methodReturnTypes); rt != "" {
			// A REFERENCE-typed local whose value only arrives through an embedded
			// assignment in a condition ((s = next(...)) != null); without a recovered type the
			// default `Object` makes every member access (s.length()) fail to recompile.
			typ, zero = rt, "null"
		} else if rt := returnedLocalDeclType(body, name, methodReturnType, returnDeclFix); rt != "" {
			// The local is RETURNED (`return varN;`) and the embedded-assign RHS type was not
			// recoverable textually (a cross-class call like DateUtils.parseLocalDateTime12 whose
			// return type no symbol-free scan can resolve). A returned value must be assignable to the
			// method return type, so declaring the local AS that type (initialized null) is always
			// legal and lets the condition's `(varN = expr)` still set it before the return.
			typ, zero = rt, "null"
		}
		lines = append(lines, fmt.Sprintf("\t%s %s = %s;\n", typ, name, zero))
	}
	return "\n" + strings.Join(lines, "") + strings.TrimPrefix(body, "\n")
}

// initProximateSplitSlotDecl initializes bare `Type varN;` declarations (no initializer) to
// `Type varN = null;` when a dead-store `Object varM;` sibling exists within a small line window
// (proximity gate). This repairs the definite-assignment error from a split slot (String in the
// if-branch, Object in the else-branch) WITHOUT attempting a structural variable merge.
//
// The proximity gate (≤ maxLines apart) is the key safety mechanism: it ensures the Object
// dead-store is in the SAME if/else block as the bare declaration, not in an unrelated part of the
// method. Canonical case: fastjson2 JSON.copyTo — `String var16;` (line 4351) + `Object var17;`
// (line 4358, 7 lines apart). Kill-switch: JDEC_INIT_PROX_SPLIT_OFF=1.
func initProximateSplitSlotDecl(body string) string {
	if os.Getenv("JDEC_INIT_PROX_SPLIT_OFF") == "1" {
		return body
	}
	dbg := false
	const maxLines = 10 // max line distance between bare decl and Object dead-store sibling
	lines := strings.Split(body, "\n")
	// Pre-collect all Object dead-store declaration line numbers.
	type objDecl struct {
		name string
		line int
	}
	var objDecls []objDecl
	for i, ln := range lines {
		// Match `Object varM;` at any indentation.
		m := objDeclLineRe.FindStringSubmatch(strings.TrimRight(ln, "\r"))
		if m != nil {
			varM := m[1]
			// Check if varM is a dead store (never read in the full body).
			if !hasReadInBody(body, varM) {
				objDecls = append(objDecls, objDecl{name: varM, line: i})
			}
		}
	}
	if len(objDecls) == 0 {
		if dbg {
			fmt.Fprintf(os.Stderr, "[PROX] no Object dead stores found\n")
		}
		return body
	}
	if dbg {
		fmt.Fprintf(os.Stderr, "[PROX] found %d Object dead stores\n", len(objDecls))
	}
	// For each bare `Type varN;` declaration, check if an Object dead-store exists within maxLines.
	bareDeclRe := regexp.MustCompile(`^(\t+)([A-Za-z_$][\w$.<>\[\]?, ]*?)\s+(var\d+(?:_\d+)?)\s*;(\s*)$`)
	for i, ln := range lines {
		lnClean := strings.TrimRight(ln, "\r")
		m := bareDeclRe.FindStringSubmatch(lnClean)
		if m == nil {
			continue
		}
		indent := m[1]
		declType := strings.TrimSpace(m[2])
		varN := m[3]
		trailing := m[4]
		// Skip Java keywords that look like type tokens (throw, return, new, etc.).
		switch declType {
		case "throw", "return", "new", "if", "else", "for", "while", "do", "switch", "case",
			"break", "continue", "try", "catch", "finally", "synchronized", "assert":
			continue
		}
		// Determine the initializer value based on type.
		initVal := "null"
		switch declType {
		case "int", "long", "short", "byte":
			initVal = "0"
		case "double":
			initVal = "0.0"
		case "float":
			initVal = "0.0F"
		case "char":
			initVal = "'\\0'"
		case "boolean":
			initVal = "false"
		}
		// varN must be read somewhere (has a non-assignment use).
		if !hasReadInBody(body, varN) {
			if dbg && varN == "var16" {
				fmt.Fprintf(os.Stderr, "[PROX] var16 not read\n")
			}
			continue
		}
		// Gate: either (a) an Object dead-store sibling is nearby (the split-slot signature), or
		// (b) varN is assigned ≥1 time and is a primitive type (multi-branch if/else chain where one
		// branch may not cover all paths — primitives are safe to default-init to 0/false/'\0').
		// Check proximity: is there an Object dead-store within maxLines?
		found := false
		for _, od := range objDecls {
			if abs(i-od.line) <= maxLines {
				found = true
				break
			}
		}
		// Fallback gate for primitives: assigned ≥1 time (the variable is used in a branch chain).
		isPrimitive := false
		switch declType {
		case "int", "long", "short", "byte", "double", "float", "char", "boolean":
			isPrimitive = true
		}
		if !found && isPrimitive && countAssignTargets(body, varN) >= 1 {
			found = true
		}
		// Fallback gate for reference types: assigned ≥2 times (multi-branch chain).
		if !found && !isPrimitive && countAssignTargets(body, varN) >= 2 {
			found = true
		}
		if !found {
			if dbg && varN == "var16" {
				fmt.Fprintf(os.Stderr, "[PROX] var16 no proximate dead store (line=%d)\n", i)
			}
			continue
		}
		// Initialize: `Type varN;` → `Type varN = <initVal>;`
		if dbg {
			fmt.Fprintf(os.Stderr, "[PROX] INITIALIZING %s %s = %s; (line=%d)\n", declType, varN, initVal, i)
		}
		lines[i] = indent + declType + " " + varN + " = " + initVal + ";" + trailing
	}
	return strings.Join(lines, "\n")
}

var objDeclLineRe = regexp.MustCompile(`^\t+Object\s+(var\d+(?:_\d+)?)\s*;`)

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// hasReadInBody reports whether name appears as a READ (not declaration, not assignment target)
// anywhere in body.
func hasReadInBody(body, name string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	locs := re.FindAllStringIndex(body, -1)
	for _, loc := range locs {
		before := body[:loc[0]]
		after := body[loc[1]:]
		// Skip declarations: preceded by a type token on the same line.
		lineStart := strings.LastIndex(before, "\n") + 1
		lineBefore := strings.TrimSpace(before[lineStart:])
		if lineBefore != "" {
			c := lineBefore[len(lineBefore)-1]
			// A declaration is preceded by a type token ending in an identifier char, ], >, or ?.
			// NOT ')' — that would match casts like `(Type) (varN)`.
			if isWordByteDump(c) || c == ']' || c == '>' || c == '?' {
				continue
			}
		}
		// Skip assignment targets: followed by `=` but not `==`.
		afterTrim := strings.TrimLeft(after, " \t")
		if strings.HasPrefix(afterTrim, "=") && !strings.HasPrefix(afterTrim, "==") {
			continue
		}
		return true
	}
	return false
}

func isWordByteDump(b byte) bool {
	return b == '_' || b == '$' || (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// fixTryCatchExceptionPlacement detects the pattern where exception-throwing calls (getDeclaredConstructor,
// newInstance, etc.) are rendered OUTSIDE a try block, while the catch clause lists those exception types.
// It moves the calls into the inner try body so javac sees them as caught.
//
// Pattern detected:
//   if (cond){
//       <exception-throwing call 1>
//       <exception-throwing call 2>
//   }
//   try{
//       try{
//           <non-throwing statement>
//       }catch(ExceptionType1 | ExceptionType2 ...){
//           throw new JSONException(...);
//       }
//   }catch(Throwable){ }
//
// Fix: move the exception-throwing calls from the if-body into the inner try-body.
// Kill-switch: JDEC_FIX_TRYCATCH_OFF=1.
func fixTryCatchExceptionPlacement(body string) string {
	if os.Getenv("JDEC_FIX_TRYCATCH_OFF") == "1" {
		return body
	}
	lines := strings.Split(body, "\n")
	// Scan for the pattern: an if-block whose body contains calls to methods that throw
	// checked exceptions (getDeclaredConstructor, newInstance, getMethod, etc.), immediately
	// followed by a try block whose catch lists those exception types.
	throwingCallRe := regexp.MustCompile(`\.(getDeclaredConstructor|newInstance|getMethod|getConstructor)\(`)
	for i := 0; i < len(lines); i++ {
		// Check if this line starts an if block.
		lnClean := strings.TrimRight(lines[i], "\r")
		if !strings.Contains(lnClean, "if (") || !strings.HasSuffix(strings.TrimSpace(lnClean), "{") {
			continue
		}
		ifIndent := ""
		for _, c := range lnClean {
			if c == '\t' {
				ifIndent += "\t"
			} else {
				break
			}
		}
		// Collect the if-body lines (at ifIndent + 1 tab).
		bodyIndent := ifIndent + "\t"
		var ifBodyLines []int
		closingBrace := -1
		for j := i + 1; j < len(lines) && j < i+20; j++ {
			jl := strings.TrimRight(lines[j], "\r")
			if strings.TrimSpace(jl) == "}" && strings.HasPrefix(jl, ifIndent) && !strings.HasPrefix(jl, bodyIndent) {
				closingBrace = j
				break
			}
			ifBodyLines = append(ifBodyLines, j)
		}
		if closingBrace < 0 || len(ifBodyLines) == 0 {
			continue
		}
		// Check if any if-body line has a throwing call.
		hasThrowingCall := false
		for _, idx := range ifBodyLines {
			if throwingCallRe.MatchString(lines[idx]) {
				hasThrowingCall = true
				break
			}
		}
		if !hasThrowingCall {
			continue
		}
		// Check if the NEXT non-empty line after closingBrace is a try block.
		tryLine := -1
		for j := closingBrace + 1; j < len(lines) && j < closingBrace + 3; j++ {
			jl := strings.TrimRight(lines[j], "\r")
			if strings.TrimSpace(jl) == "" {
				continue
			}
			if strings.Contains(jl, "try{") {
				tryLine = j
			}
			break
		}
		if tryLine < 0 {
			continue
		}
		// Find the inner try body: the line after `try{` at tryIndent+1.
		innerTryIndent := ifIndent + "\t"
		innerTryStart := -1
		for j := tryLine + 1; j < len(lines) && j < tryLine + 5; j++ {
			jl := strings.TrimRight(lines[j], "\r")
			if strings.Contains(jl, "try{") && strings.HasPrefix(jl, innerTryIndent) {
				innerTryStart = j + 1
				break
			}
		}
		if innerTryStart < 0 {
			continue
		}
		// Move the throwing-call lines from the if-body to before the inner try's first statement.
		// Build the new lines: remove throwing calls from if-body, insert them at innerTryStart.
		var throwingLines []string
		var newIfBody []string
		for _, idx := range ifBodyLines {
			jl := strings.TrimRight(lines[idx], "\r")
			if throwingCallRe.MatchString(jl) {
				// Re-indent to innerTryIndent + 1 tab.
				trimmed := strings.TrimLeft(jl, "\t")
				throwingLines = append(throwingLines, innerTryIndent+"\t"+trimmed)
			} else {
				newIfBody = append(newIfBody, jl)
			}
		}
		if len(throwingLines) == 0 {
			continue
		}
		// Replace the if-body: keep non-throwing lines only.
		// The if-body lines span from i+1 to closingBrace-1.
		// First, blank out all original if-body lines.
		for _, idx := range ifBodyLines {
			lines[idx] = ""
		}
		// Re-fill with non-throwing lines.
		bodyOffset := i + 1
		for _, l := range newIfBody {
			lines[bodyOffset] = l
			bodyOffset++
		}
		// If the if-body is now empty (all lines were throwing), make the if body just `{}`.
		if len(newIfBody) == 0 {
			// Merge the if line and closing brace into `{` on same line.
			lines[i] = strings.TrimRight(lines[i], "\r")
			lines[closingBrace] = ""
			// Move closing brace to if line.
			// Actually keep as is — an empty if body `{ }` is fine.
			// Put an empty line where the body was.
			lines[i+1] = ifIndent + "\t"
		}
		// Insert throwing lines before the inner try's first statement.
		// We need to shift all subsequent lines down.
		insertAt := innerTryStart
		for _, tl := range throwingLines {
			lines = append(lines[:insertAt], append([]string{tl}, lines[insertAt:]...)...)
			insertAt++
		}
	}
	return strings.Join(lines, "\n")
}

// countAssignTargets counts how many times name appears as an assignment target (`name =` but not
// `name ==`) in body. Used to detect multi-branch if/else chains.
func countAssignTargets(body, name string) int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	locs := re.FindAllStringIndex(body, -1)
	count := 0
	for _, loc := range locs {
		after := body[loc[1]:]
		afterTrim := strings.TrimLeft(after, " \t")
		if strings.HasPrefix(afterTrim, "=") && !strings.HasPrefix(afterTrim, "==") {
			// Skip declarations (preceded by type token).
			before := body[:loc[0]]
			lineStart := strings.LastIndex(before, "\n") + 1
			lineBefore := strings.TrimSpace(before[lineStart:])
			if lineBefore != "" {
				c := lineBefore[len(lineBefore)-1]
				if isWordByteDump(c) || c == ']' || c == '>' || c == '?' {
					continue
				}
			}
			count++
		}
	}
	return count
}

// castEscapeDeclLineRe matches a single rendered line that DECLARES a generated local
// (`<type> varN = ...` or `<type> varN;`), capturing the indent (1), the type expression (2), the
// slot name (3) and the `= rhs` / `;` tail (4). The type char class deliberately allows spaces,
// `<>?,` and `[]` so generic and array types (`Map<?, ?> var2`, `int[] var3`) are recognized; it
// excludes `()` and `=` so a cast/return/call line (`return (T) (var2);`, `this.items.add(var8);`)
// can never be mistaken for a declaration.
var castEscapeDeclLineRe = regexp.MustCompile(`^(\s*)([A-Za-z_$][A-Za-z0-9_$.<>?,\[\] ]*?)\s+(var\d+(?:_\d+)?)(\s*[=;].*)$`)

// castEscapeTypeKeywords are leading words that look like a type to castEscapeDeclLineRe but are not:
// `return varN;` / `throw varN;` etc. must be classified as USES, never declarations.
var castEscapeTypeKeywords = map[string]bool{
	"return": true, "throw": true, "new": true, "instanceof": true,
	"else": true, "assert": true, "case": true, "yield": true,
	"break": true, "continue": true,
}

var castEscapeScalarPrimitives = map[string]bool{
	"boolean": true, "byte": true, "char": true, "short": true,
	"int": true, "long": true, "float": true, "double": true,
}

func castEscapeFirstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t<.[("); i >= 0 {
		return s[:i]
	}
	return s
}

func castEscapeLastToken(s string) string {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// castEscapePrecedingIdentType reports the simple/qualified type identifier that immediately precedes
// a `varN` occurrence when the text before it has the shape `... <identifier> ` (an identifier token
// followed by whitespace, i.e. a `Type varN` declaration: catch parameter, for-init, for-each, or a
// try-with-resources). It returns "" when the preceding run is not such an identifier - a binary
// operator (`a > varN`), an assignment (`= varN`), a comma/paren-delimited argument (`(varN`,
// `, varN`), a cast (`) varN`), or a keyword-led statement (`return varN`) - so genuine reads are
// never mistaken for declarations. A non-empty, non-keyword result marks the slot as carrying an
// unanchored declaration and disqualifies it from the same-type hoist.
func castEscapePrecedingIdentType(pre string) string {
	if pre == "" {
		return ""
	}
	if c := pre[len(pre)-1]; c != ' ' && c != '\t' {
		return ""
	}
	q := strings.TrimRight(pre, " \t")
	i := len(q)
	for i > 0 {
		c := q[i-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$' || c == '.' {
			i--
			continue
		}
		break
	}
	tid := q[i:]
	if tid == "" || castEscapeTypeKeywords[castEscapeFirstToken(tid)] {
		return ""
	}
	return tid
}

// castEscapeClassifyUse classifies a single NON-declaration occurrence of a generated local by the
// text immediately around it: 1 = explicit cast (`(X)(name)`, `(X) (name)`, `(X)name` - the run just
// before name reduces to a closing `)`), 2 = bare assignment LHS (`name = ...`, preceded only by
// whitespace, a single `=`), 0 = anything else (member access `name.f`, index `name[i]`, an uncast
// argument/return, an arithmetic/relational operand) - i.e. a use for which an `Object` declaration
// would be unsound.
func castEscapeClassifyUse(pre, post string) int {
	q := strings.TrimRight(pre, " \t")
	if strings.HasSuffix(q, "(") {
		q = strings.TrimRight(q[:len(q)-1], " \t")
	}
	if strings.HasSuffix(q, ")") {
		return 1
	}
	if strings.TrimLeft(pre, " \t") == "" {
		t := strings.TrimLeft(post, " \t")
		if strings.HasPrefix(t, "=") && !strings.HasPrefix(t, "==") {
			return 2
		}
	}
	return 0
}

// hoistSameTypeEscapedLocals closes the SAME-rendered-type subfamily of the escaped-local-read shape
// of Bug AL - the sound complement of hoistCastGuardedEscapedLocals. A JVM slot reused for a
// logically-single variable (or several disjoint ranges that all carry the SAME type) is first
// declared INSIDE one or more nested scopes - an if/else arm, a switch case, a try/catch, a loop body,
// or any combination - and then READ after the join, at a shallower indentation than every
// declaration. The arms carry distinct VarUids that render the IDENTICAL type token T (e.g. every arm
// declares `JSONReader$Context varN = ...`), so the AST pass parallelArmDeclHoist (if/else only) and
// the cross-scope placement never reach this shape, and javac rejects the post-join read as
// "cannot find symbol: variable varN".
//
// Unlike hoistCastGuardedEscapedLocals - which has NO recovered join type and so can only fire on the
// provably-Object shape (every use a cast or assignment) - this pass already KNOWS the single type
// token T shared by every declaration, so it hoists a real `T varN = null;` to method top and demotes
// each inner `T varN = rhs` to `varN = rhs`. Because T is exactly the type every store produced and
// every read consumed in the original bytecode, the result type-checks for ANY use - member access,
// indexing, an uncast argument/return, arithmetic - not only casts. Merging several disjoint same-type
// ranges into one top-level variable is sound: the ranges never overlap and each is reassigned before
// it is read.
//
// It fires ONLY when every declaration of the slot renders the identical REFERENCE type token T (a
// primitive T would need a typed zero and is left to the int path / addMissingGeneratedLocalDecls);
// the slot has NO bare-assignment definition (`varN = rhs` with no type prefix), because such a store
// could belong to a DIFFERENT, foreign-typed live range of the same slot (e.g. a map key reusing an
// ObjectReader's slot, fastjson2 ObjectReaderImplMapTyped) over which hoisting T would mistype it; and
// at least one READ sits at a shallower indentation than every declaration (the orphan post-join
// read). Any other shape leaves the slot untouched, so a file keeps its pre-existing error rather than
// regressing. "Escaped" is detected by indentation, exactly as in hoistCastGuardedEscapedLocals.
// Kill-switch: JDEC_SAMETYPE_HOIST_OFF=1.
func hoistSameTypeEscapedLocals(body string) string {
	if os.Getenv("JDEC_SAMETYPE_HOIST_OFF") != "" {
		return body
	}
	lines := strings.Split(body, "\n")
	declAt := make([]string, len(lines))
	for i, ln := range lines {
		m := castEscapeDeclLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if castEscapeTypeKeywords[castEscapeFirstToken(m[2])] {
			continue
		}
		declAt[i] = m[3]
	}

	type rec struct {
		declDepths  []int
		readDepths  []int
		declType    string
		typeClash   bool
		bareAssign  bool
		foreignDecl bool
	}
	recs := map[string]*rec{}
	get := func(name string) *rec {
		r := recs[name]
		if r == nil {
			r = &rec{}
			recs[name] = r
		}
		return r
	}
	for i, ln := range lines {
		depth := len(ln) - len(strings.TrimLeft(ln, " \t"))
		for _, loc := range generatedLocalRefRe.FindAllStringIndex(ln, -1) {
			name := ln[loc[0]:loc[1]]
			r := get(name)
			if declAt[i] == name {
				tail := strings.TrimLeft(ln[loc[1]:], " \t")
				if strings.HasPrefix(tail, "=") || strings.HasPrefix(tail, ";") {
					typeTok := strings.TrimSpace(castEscapeDeclLineRe.FindStringSubmatch(ln)[2])
					if r.declType == "" {
						r.declType = typeTok
					} else if r.declType != typeTok {
						r.typeClash = true
					}
					r.declDepths = append(r.declDepths, depth)
					continue
				}
			}
			// A `<non-keyword-identifier> varN` shape is a DECLARATION that castEscapeDeclLineRe could
			// not anchor (a catch parameter `catch(EncoderException varN)`, a for-init `for(int varN`,
			// a for-each, or a try-with-resources). It declares a slot-sharing - usually
			// DIFFERENT-typed - variable, so hoisting our type T over it and injecting a top-level decl
			// would duplicate it ("variable varN is already defined", commons-codec StringEncoderComparator
			// catch(EncoderException var4)). Disqualify the name. Genuine reads (member access, indexing,
			// argument, return, arithmetic, cast) never have a non-keyword identifier immediately before
			// the name, so they are unaffected - that is the whole point of the same-type hoist.
			if tid := castEscapePrecedingIdentType(ln[:loc[0]]); tid != "" {
				r.foreignDecl = true
			}
			switch castEscapeClassifyUse(ln[:loc[0]], ln[loc[1]:]) {
			case 2:
				r.bareAssign = true
			default:
				r.readDepths = append(r.readDepths, depth)
			}
		}
	}

	fired := map[string]bool{}
	fireType := map[string]string{}
	for name, r := range recs {
		if r.typeClash || r.bareAssign || r.foreignDecl || len(r.declDepths) == 0 || len(r.readDepths) == 0 {
			continue
		}
		typeTok := r.declType
		if typeTok == "" || castEscapeTypeKeywords[castEscapeFirstToken(typeTok)] {
			continue
		}
		// Reference types only: a scalar primitive would need a typed zero and is handled elsewhere.
		if !strings.Contains(typeTok, "[") && castEscapeScalarPrimitives[castEscapeLastToken(typeTok)] {
			continue
		}
		minDecl := r.declDepths[0]
		for _, d := range r.declDepths[1:] {
			if d < minDecl {
				minDecl = d
			}
		}
		for _, d := range r.readDepths {
			if d < minDecl {
				fired[name] = true
				fireType[name] = typeTok
				break
			}
		}
	}
	if len(fired) == 0 {
		return body
	}

	out := make([]string, 0, len(lines)+len(fired))
	for i, ln := range lines {
		if name := declAt[i]; name != "" && fired[name] {
			m := castEscapeDeclLineRe.FindStringSubmatch(ln)
			if strings.HasPrefix(strings.TrimLeft(m[4], " \t"), "=") {
				out = append(out, m[1]+name+m[4])
			}
			// a bare `T varN;` is dropped: the injected `T varN = null;` carries the slot
			continue
		}
		out = append(out, ln)
	}

	names := make([]string, 0, len(fired))
	for n := range fired {
		names = append(names, n)
	}
	sort.Strings(names)
	insertIdx := 0
	for insertIdx < len(out) && strings.TrimSpace(out[insertIdx]) == "" {
		insertIdx++
	}
	indent := "\t"
	if insertIdx < len(out) {
		if t := strings.TrimSpace(out[insertIdx]); strings.HasPrefix(t, "super(") || strings.HasPrefix(t, "this(") {
			insertIdx++
		}
		if insertIdx < len(out) {
			indent = out[insertIdx][:len(out[insertIdx])-len(strings.TrimLeft(out[insertIdx], " \t"))]
		}
	}
	inject := make([]string, 0, len(names))
	for _, n := range names {
		inject = append(inject, indent+fireType[n]+" "+n+" = null;")
	}
	merged := make([]string, 0, len(out)+len(inject))
	merged = append(merged, out[:insertIdx]...)
	merged = append(merged, inject...)
	merged = append(merged, out[insertIdx:]...)
	return strings.Join(merged, "\n")
}

// hoistCastGuardedEscapedLocals closes the "if/else parallel-phi orphan read, DIFFERENT-rendered-type
// subfamily" (the least-upper-bound subfamily of Bug AL) that the AST pass parallelArmDeclHoist
// cannot: a JVM slot reused for logically-one variable that is first-declared INSIDE two or more arms
// of an if/else (possibly nested) with DIFFERENT rendered types - e.g.
// `ParameterizedType var3 = ...` vs `ParameterizedTypeImpl var3 = ...` (fastjson2
// ObjectWriters.fieldWriterList), or `Object`/`List`/`Object var2` across three nested arms
// (JSONStreamReaderUTF8.readLineObject) - and then READ after the join. The arms carry different
// VarUids, and parallelArmDeclHoist only merges arms whose rendered type tokens AGREE (widening
// genuinely-different types would need a common-supertype facility this decompiler does not have), so
// each arm keeps its own decl, the post-join read binds a slot name whose every declaration lives in
// a non-dominating inner scope, and javac rejects it as "cannot find symbol: variable varN".
//
// Computing the true least-upper-bound of the arm types requires a cross-class hierarchy the
// decompiler cannot resolve, so this pass NEVER guesses a join type. It fires ONLY on the shape where
// `Object varN` is PROVABLY sound regardless of the LUB: every non-declaration use of the escaped slot
// is an explicit CAST (`(X)(varN)`) or a bare assignment, and no declaration of it is a scalar
// primitive. For that shape an `Object varN = null;` at method top is always type-correct - each arm
// store accepts any reference value and each read down-casts from Object - while every store keeps its
// own RHS type. Any other use (member access, index, uncast argument/return, arithmetic) makes Object
// unsound and leaves the slot untouched, so the file simply keeps its one pre-existing error (no
// regression). The transform demotes each inner `T varN = rhs` to `varN = rhs`, drops a bare
// `T varN;`, and injects the single `Object varN = null;`; addMissingGeneratedLocalDecls (run next)
// then sees the name declared and adds nothing.
//
// "Escaped" is detected by indentation: the dumper indents one tab per nesting level, so a cast-use
// whose leading-whitespace depth is SHALLOWER than every declaration of the same name is, by
// construction, outside all the arms that declare it - exactly the orphan read. A slot whose
// declaration already dominates its reads (decl depth <= every read depth) is never shallower-read and
// is left alone. Kill-switch: JDEC_CAST_ESCAPE_HOIST_OFF=1.
func hoistCastGuardedEscapedLocals(body string) string {
	if os.Getenv("JDEC_CAST_ESCAPE_HOIST_OFF") != "" {
		return body
	}
	lines := strings.Split(body, "\n")
	declAt := make([]string, len(lines))
	primDecl := map[string]bool{}
	for i, ln := range lines {
		m := castEscapeDeclLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if castEscapeTypeKeywords[castEscapeFirstToken(m[2])] {
			continue
		}
		declAt[i] = m[3]
		typeTok := strings.TrimSpace(m[2])
		if !strings.Contains(typeTok, "[") && castEscapeScalarPrimitives[castEscapeLastToken(typeTok)] {
			primDecl[m[3]] = true
		}
	}

	type rec struct {
		declDepths []int
		castDepths []int
		bad        bool
	}
	recs := map[string]*rec{}
	get := func(name string) *rec {
		r := recs[name]
		if r == nil {
			r = &rec{}
			recs[name] = r
		}
		return r
	}
	for i, ln := range lines {
		depth := len(ln) - len(strings.TrimLeft(ln, " \t"))
		for _, loc := range generatedLocalRefRe.FindAllStringIndex(ln, -1) {
			name := ln[loc[0]:loc[1]]
			if declAt[i] == name {
				tail := strings.TrimLeft(ln[loc[1]:], " \t")
				if strings.HasPrefix(tail, "=") || strings.HasPrefix(tail, ";") {
					get(name).declDepths = append(get(name).declDepths, depth)
					continue
				}
			}
			switch castEscapeClassifyUse(ln[:loc[0]], ln[loc[1]:]) {
			case 1:
				get(name).castDepths = append(get(name).castDepths, depth)
			case 2:
				// benign assignment LHS - neither proves escape nor unsoundness
			default:
				get(name).bad = true
			}
		}
	}

	fired := map[string]bool{}
	for name, r := range recs {
		if r.bad || primDecl[name] || len(r.declDepths) == 0 || len(r.castDepths) == 0 {
			continue
		}
		minDecl := r.declDepths[0]
		for _, d := range r.declDepths[1:] {
			if d < minDecl {
				minDecl = d
			}
		}
		for _, d := range r.castDepths {
			if d < minDecl {
				fired[name] = true
				break
			}
		}
	}
	if len(fired) == 0 {
		return body
	}

	out := make([]string, 0, len(lines)+len(fired))
	for i, ln := range lines {
		if name := declAt[i]; name != "" && fired[name] {
			m := castEscapeDeclLineRe.FindStringSubmatch(ln)
			if strings.HasPrefix(strings.TrimLeft(m[4], " \t"), "=") {
				out = append(out, m[1]+name+m[4])
			}
			// a bare `T varN;` is dropped: the injected `Object varN = null;` carries the slot
			continue
		}
		out = append(out, ln)
	}

	names := make([]string, 0, len(fired))
	for n := range fired {
		names = append(names, n)
	}
	sort.Strings(names)
	// Insert after any leading blank lines (keep the body's leading newline) and after a
	// constructor's super()/this() chain call (which must remain the first statement). The injected
	// declarations adopt the indentation of the first real statement so they align with the body.
	insertIdx := 0
	for insertIdx < len(out) && strings.TrimSpace(out[insertIdx]) == "" {
		insertIdx++
	}
	indent := "\t"
	if insertIdx < len(out) {
		if t := strings.TrimSpace(out[insertIdx]); strings.HasPrefix(t, "super(") || strings.HasPrefix(t, "this(") {
			insertIdx++
		}
		if insertIdx < len(out) {
			indent = out[insertIdx][:len(out[insertIdx])-len(strings.TrimLeft(out[insertIdx], " \t"))]
		}
	}
	inject := make([]string, 0, len(names))
	for _, n := range names {
		inject = append(inject, indent+"Object "+n+" = null;")
	}
	merged := make([]string, 0, len(out)+len(inject))
	merged = append(merged, out[:insertIdx]...)
	merged = append(merged, inject...)
	merged = append(merged, out[insertIdx:]...)
	return strings.Join(merged, "\n")
}

// repairMismatchedDoWhileIndexDecls repairs the narrow case where the decompiler mis-named the
// declaration of a do-while loop index: `int X = 0;\ndo{\n if ((Y) < ...` where the loop body
// actually iterates on Y, X is a stale name the rest of the body never uses, and Y has no other
// declaration. There the `int X = 0` is the index initializer wearing the wrong name, so renaming
// it to `int Y = 0` makes the source compile.
//
// It must NOT fire on a continued-variable tail loop, where the declaration immediately before the
// do-while is a DIFFERENT, legitimate local than the one the condition tests - e.g.
// `int j = 0; do { if (i < n) { ...; j++; } }` keeps incrementing the outer index `i` while a new
// `j` is initialized first. There Y (`i`/var1) is already declared above and X (`j`/var3) is used
// inside the loop body, so the old unconditional rewrite both dropped `j`'s declaration and aliased
// it onto the already-live `i`, producing a duplicate `int var1 = 0` plus a phantom hoisted
// `int var3 = 0` (Bug C). The two guards below skip exactly that shape: only a genuinely misnamed,
// otherwise-unused declaration of an otherwise-undeclared index is rewritten. Kill-switch:
// JDEC_DOWHILE_INDEX_REPAIR_OFF=1.
func repairMismatchedDoWhileIndexDecls(body string) string {
	if os.Getenv("JDEC_DOWHILE_INDEX_REPAIR_OFF") != "" {
		return body
	}
	return mismatchedDoWhileIndexDeclRe.ReplaceAllStringFunc(body, func(match string) string {
		parts := mismatchedDoWhileIndexDeclRe.FindStringSubmatch(match)
		if len(parts) != 5 || parts[1] == parts[4] {
			return match
		}
		declaredName, indexName := parts[1], parts[4]
		// The loop index Y must be otherwise undeclared: if it already has a declaration
		// (a continued outer index), renaming X to Y would duplicate/alias a live variable.
		if generatedLocalIsDeclared(body, indexName) {
			return match
		}
		// The declared name X must be a stale name used nowhere else: a single occurrence in
		// the whole body is the declaration itself. More than one means X is a real, separate
		// variable (e.g. `j` read/incremented inside the loop) that must keep its declaration.
		if generatedLocalOccurrences(body, declaredName) > 1 {
			return match
		}
		return fmt.Sprintf("int %s = 0;\n%sdo{\n%sif ((%s) <", indexName, parts[2], parts[3], indexName)
	})
}

// generatedLocalIsDeclared reports whether name has any `T name` declaration in body.
func generatedLocalIsDeclared(body, name string) bool {
	for _, match := range generatedLocalDeclRe.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 && match[1] == name {
			return true
		}
	}
	return false
}

// generatedLocalOccurrences counts whole-token references to a generated local name in body.
func generatedLocalOccurrences(body, name string) int {
	return len(regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\b`).FindAllString(body, -1))
}

func generatedLocalLooksInt(body, name string) bool {
	quoted := regexp.QuoteMeta(name)
	patterns := []string{
		`\b` + quoted + `\s*(?:\+\+|--)`,
		`(?:\+\+|--)\s*` + quoted + `\b`,
		// A bare RELATIONAL comparison (`(v) < n`) is numeric-only in Java -> int category for
		// any right operand.
		`\(` + quoted + `\)\s*(?:<|>|<=|>=)`,
		// A bare EQUALITY comparison (`(v) == X` / `(v) != X`) is also valid for REFERENCES, so it
		// only proves int when the right operand is a numeric literal (`(0)`, `-1`). A reference
		// comparison such as `(v) != (HashMap.class)` or `(v) != (var2)` must NOT be read as int,
		// otherwise an `objectClass`-style local (fastjson2 JSONWriter.checkAndWriteTypeName, whose
		// value is `(v = obj.getClass()) != type`) is mis-declared `int v = 0` and every Class
		// comparison/use fails to recompile.
		`\(` + quoted + `\)\s*(?:==|!=)\s*\(?-?\d`,
		`\[\s*` + quoted + `\s*\]`,
	}
	// Embedded-assignment-in-condition form produced by the dup-collapse, e.g.
	// `(var4 = expr) == (0)` / `(var4 = expr) < (n)` (commons-codec Metaphone /
	// MatchRatingApproachEncoder, and the synthetic EmbeddedAssignDecl battery). Such a variable has
	// no ordinary `T v = ...` declaration, so the safety net must synthesize one; without these
	// signals it guessed `Object v = null`, which breaks the int store and any arithmetic read.
	//   - A RELATIONAL comparison (`< > <= >=`) is numeric-only in Java, so the embedded-assign
	//     target is int-category regardless of the right operand.
	//   - An EQUALITY comparison (`== !=`) is numeric ONLY when the right operand is a numeric
	//     literal (`(0)`, `-1`); it must NOT match a reference null-check like
	//     `(o = foo()) != null`, which legitimately compiles with the `Object o = null` default.
	// Kill-switch: JDEC_NO_EMBED_ASSIGN_INT=1 restores the pre-fix (Object-defaulting) behavior.
	if os.Getenv("JDEC_NO_EMBED_ASSIGN_INT") == "" {
		// The embedded-assign RHS must tolerate ONE level of parentheses so the ubiquitous
		// `while ((c = this.read()) != -1)` / `(c = in.read()) < n` idiom is recognised: the RHS is a
		// method call (`this.read()`), whose `()` a bare `[^()]*` cannot span, so it previously fell
		// through to `Object c = null` and then failed to recompile ("bad operand types for binary
		// operator '!='/'<'" -- spring-core UpdateMessageDigestInputStream, and any InputStream-drain
		// loop). `[^()]*(?:\([^()]*\)[^()]*)*` matches a RHS with a single (possibly-nested-arg-free)
		// call. The int-category guarantees are unchanged: a relational operator is numeric-only in
		// Java regardless of the RHS, and the equality form still requires a NUMERIC-LITERAL right
		// operand (`\(?-?\d`), so a reference null-check `(o = foo()) != null` is still NOT matched.
		rhs := `[^()]*(?:\([^()]*\)[^()]*)*`
		patterns = append(patterns,
			`\(`+quoted+` = `+rhs+`\)\s*(?:<|>|<=|>=)`,
			`\(`+quoted+` = `+rhs+`\)\s*(?:==|!=)\s*\(?-?\d`,
		)
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(body) {
			return true
		}
	}
	return false
}

// embeddedAssignRHSRe / arrayLoadRHSRe / bareCallRHSRe / typedArrayDeclRe / methodReturnDeclRe back
// inferGeneratedLocalRefType. They are package-level so the regex compiles once.
var arrayLoadRHSRe = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)\[.+\]$`)
var bareCallRHSRe = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)\(.*\)$`)

// embeddedAssignRHS returns the right-hand side of the FIRST embedded assignment to name, i.e. the
// balanced expression X in `(name = X)`. It scans with explicit paren-depth tracking because the RHS
// itself may contain parentheses (a method call), which a regex cannot balance.
func embeddedAssignRHS(body, name string) (string, bool) {
	marker := "(" + name + " = "
	idx := strings.Index(body, marker)
	if idx < 0 {
		return "", false
	}
	start := idx + len(marker)
	depth := 1 // the '(' that opened the embedded-assignment group
	for i := start; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(body[start:i]), true
			}
		}
	}
	return "", false
}

// declaredArrayElementType returns the element type of an array local/param named arr by reading its
// `T[]... arr` declaration from text, or "" when none is found. For a multi-dimensional array it
// strips exactly one dimension (String[][] arr -> String[]).
func declaredArrayElementType(text, arr string) string {
	re := regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$.<>?,]*)\s*((?:\[\s*\])+)\s+` + regexp.QuoteMeta(arr) + `\b`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	base := m[1]
	dims := strings.Count(m[2], "[")
	if dims <= 1 {
		return base
	}
	return base + strings.Repeat("[]", dims-1)
}

// declaredMethodReturnType returns the declared return type of an in-class method named method by
// matching its `T method(params) {` definition in body, or "" when not found. The trailing `{`
// requirement distinguishes a method DEFINITION from a call site (`return method(...)` has no brace),
// and a leading `.` rules out an instance-call spelled `recv.method(`.
func declaredMethodReturnType(body, method string) string {
	re := regexp.MustCompile(`(?:^|[^.\w$])([A-Za-z_$][A-Za-z0-9_$.<>?,\[\]]*)\s+` + regexp.QuoteMeta(method) + `\s*\([^;{}]*\)\s*\{`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		ret := m[1]
		switch ret {
		case "return", "new", "throw", "else", "instanceof", "case":
			continue
		}
		return ret
	}
	return ""
}

// inferGeneratedLocalRefType recovers the reference type of an undeclared local that only receives
// its value through an embedded assignment in a condition, e.g.
//
//	if ((var4 = next(a, b)) != null) { var4.length(); }     // -> next()'s return type
//	while ((var4 = arr[i]) != null) { ... }                 // -> arr's element type
//
// The dup-collapse drops the standalone `T var4 = ...` declaration, so the string-level safety net
// would default it to `Object` and every member access fails to recompile (`cannot find symbol`).
// The type is recovered using ONLY information already present in the rendered body (the array's own
// declaration / the in-class method's signature), so it needs no symbol table and stays self
// contained. Anything it cannot resolve confidently returns "" so the caller keeps the safe `Object`
// default - so this can only turn a non-compiling unit into a compiling one, never the reverse.
// Kill-switch: JDEC_NO_EMBED_ASSIGN_REF=1 restores the pre-fix (Object-defaulting) behavior.
// returnedLocalDeclType returns the enclosing method's return type to declare an undeclared local
// `name` with, but ONLY when that local is RETURNED bare (`return name;`) and the return type is a
// usable reference type. Rationale: a returned value must be assignable to the method's declared
// return type (JLS 14.17), so an undeclared local whose value is computed in an embedded condition
// assignment and then returned (fastjson2 readLocalDateTime12 family) can always be declared as the
// return type initialized to null - the condition's `(name = expr)` assigns it before the return on
// every path that reaches the return. This is the textual last resort after the RHS-type scan
// (inferGeneratedLocalRefType) fails because the value comes from a cross-class call. Returns "" when
// the local is not returned, the return type is void/primitive, or the fix is disabled.
func returnedLocalDeclType(body, name, methodReturnType string, enabled bool) string {
	if !enabled || methodReturnType == "" || methodReturnType == "void" {
		return ""
	}
	if castEscapeScalarPrimitives[methodReturnType] {
		return ""
	}
	if !regexp.MustCompile(`\breturn\s+` + regexp.QuoteMeta(name) + `\b`).MatchString(body) {
		return ""
	}
	return methodReturnType
}

func inferGeneratedLocalRefType(body, params, name string, methodReturnTypes map[string]string) string {
	if os.Getenv("JDEC_NO_EMBED_ASSIGN_REF") != "" {
		return ""
	}
	rhs, ok := embeddedAssignRHS(body, name)
	if !ok {
		return ""
	}
	// `recv.getClass()` always yields java.lang.Class; the raw `Class` type recompiles for every
	// use (Class comparisons, Class-typed arguments). This is the single most common reference
	// embedded-assign RHS whose type a textual scan can resolve without a symbol table
	// (fastjson2 JSONWriter.checkAndWriteTypeName `objectClass = obj.getClass()`).
	if strings.HasSuffix(rhs, ".getClass()") {
		return "Class"
	}
	if m := arrayLoadRHSRe.FindStringSubmatch(rhs); m != nil {
		return declaredArrayElementType(params+"\n"+body, m[1])
	}
	if m := bareCallRHSRe.FindStringSubmatch(rhs); m != nil {
		// Prefer the authoritative class method table; fall back to an in-body declaration scan
		// (covers methods rendered in the same unit that are not in the descriptor map).
		if methodReturnTypes != nil {
			if ret := methodReturnTypes[m[1]]; ret != "" {
				return ret
			}
		}
		return declaredMethodReturnType(body, m[1])
	}
	return ""
}

// canHoistFieldInitializer reports whether a `final`-field assignment found inside <init>/
// <clinit> may be lifted into a field initializer. A real field initializer cannot reference
// constructor parameters or local variables; the JVM models those as slot locals that the
// decompiler renders as varN. If the right-hand side mentions any such local, lifting it would
// emit illegal Java (e.g. `final String id = var1;` where var1 is a constructor parameter), so
// the assignment is kept in the constructor/static block instead. Erring toward NOT hoisting is
// always safe: `this.f = expr;` / `f = expr;` compiles whether or not it could have been an
// initializer.
func canHoistFieldInitializer(rhs string) bool {
	// Kill-switch: restore the legacy narrow `\bvar\d+\b` matcher (the `_M` hole) so the
	// renamed-local mis-hoist reproduces for the load-bearing test.
	if os.Getenv("JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF") != "" {
		return !localSlotRefReNarrowLegacy.MatchString(rhs)
	}
	return !localSlotRefRe.MatchString(rhs)
}

// localSlotRefReNarrowLegacy is the pre-fix matcher that misses the collision-renamed `varN_M` form;
// retained only behind the JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF kill-switch for the load-bearing test.
var localSlotRefReNarrowLegacy = regexp.MustCompile(`\bvar\d+\b`)

func canHoistFieldValueInitializer(value values.JavaValue, rhs string) bool {
	if canHoistFieldInitializer(rhs) {
		return true
	}
	if cv, ok := values.UnpackSoltValue(value).(*values.CustomValue); ok && cv.Flag == "lambda" && cv.NoOuterCapture {
		return true
	}
	return false
}

// EnableFieldInitHoistGuard gates the safety guard that prevents a constructor/<clinit> field
// assignment from being wrongly lifted into a field initializer. Set to false to restore the
// legacy (over-eager) hoisting behavior for debugging/regression bisection.
var EnableFieldInitHoistGuard = true

// EnableCrossConstructorHoistGuard gates ONLY the class-wide (cross-constructor) half of the hoist
// guard: a blank final assigned exactly once per constructor body but in several overloaded
// constructors must still not be hoisted. Set to false to drop just this cross-constructor check
// (keeping the per-body guard) for debugging/regression bisection; the BlankFinalMultiCtor battery
// regresses when it is off, proving the check is load-bearing.
var EnableCrossConstructorHoistGuard = true

// crossCtorStoreOK reports whether the class-wide store count permits hoisting field name. When the
// cross-constructor guard is disabled it is a no-op (always true), isolating its effect.
func crossCtorStoreOK(totals map[string]int, name string) bool {
	if !EnableCrossConstructorHoistGuard {
		return true
	}
	return totals[name] <= 1
}

// rhsReadsInstanceField reports whether a candidate field-initializer right-hand side reads
// another instance field via `this.`. A real field initializer may reference earlier-declared
// fields, but the decompiler cannot prove the referenced field already holds its final value at
// initializer time (a blank final assigned later in the constructor still reads its default 0
// here). Lifting such an assignment silently changes the value, so we keep it in the constructor
// body where the referenced field is already assigned — always safe and value-preserving.
func rhsReadsInstanceField(rhs string) bool {
	return strings.Contains(rhs, "this.")
}

// countConstructorFieldAssignments walks a constructor (<init>) or static-initializer (<clinit>)
// body, recursing into every nested block, and counts how many times each field is assigned:
// instance fields via `this.f` and same-class static fields via `Class.f`. A genuine field
// initializer is assigned exactly once (javac copies it into the constructor prologue); a blank
// final assigned across multiple conditional branches is assigned 2+ times. Only a single
// assignment may be lifted into a field initializer — otherwise the remaining branch assignments
// stay in the body and javac rejects the now double-assigned final ("cannot assign a value to
// final variable").
func countConstructorFieldAssignments(stmts []statements.Statement, className string) map[string]int {
	counts := map[string]int{}
	tally := func(st *statements.AssignStatement) {
		if st.LeftValue == nil {
			return
		}
		switch lv := st.LeftValue.(type) {
		case *values.RefMember:
			if ref, ok := core.UnpackSoltValue(lv.Object).(*values.JavaRef); ok && ref.IsThis {
				counts[lv.Member]++
			}
		case *values.JavaClassMember:
			if lv.Name == className {
				counts[lv.Member]++
			}
		}
	}
	var walk func(list []statements.Statement)
	walkOne := func(st statements.Statement) {
		if st != nil {
			walk([]statements.Statement{st})
		}
	}
	walk = func(list []statements.Statement) {
		for _, st := range list {
			switch s := st.(type) {
			case *statements.AssignStatement:
				tally(s)
			case *statements.IfStatement:
				walk(s.IfBody)
				walk(s.ElseBody)
			case *statements.ForStatement:
				walkOne(s.InitVar)
				walk(s.SubStatements)
				walkOne(s.EndExp)
			case *statements.WhileStatement:
				walk(s.Body)
			case *statements.DoWhileStatement:
				walk(s.Body)
			case *statements.TryCatchStatement:
				walk(s.TryBody)
				for _, b := range s.CatchBodies {
					walk(b)
				}
			case *statements.SwitchStatement:
				for _, c := range s.Cases {
					walk(c.Body)
				}
			case *statements.SynchronizedStatement:
				walk(s.Body)
			}
		}
	}
	walk(stmts)
	return counts
}

// constructorFieldStoreTotals returns, per field name, how many putfield/putstatic targets it has
// across EVERY <init> and <clinit> body of this class. The result is computed once via a read-only
// opcode pre-scan (core.Decompiler.CountFieldStores) and cached on the dumper.
//
// This complements the per-body countConstructorFieldAssignments: a blank-final field may be
// assigned exactly once in each of two overloaded constructors (per-body count == 1 in both), yet
// hoisting it into a field initializer is still illegal, because every constructor would then carry
// both the initializer copy and its own assignment, double-assigning a final field. Only a field
// assigned in a single place across the whole class is safe to hoist, so callers gate hoisting on
// this total being <= 1.
//
// The scan is best-effort: if a constructor body fails to parse, its counts are simply skipped, so
// the totals never over-report (they can only under-report), and an under-report degrades to the
// pre-existing per-body guard rather than to an incorrect hoist.
func (c *ClassObjectDumper) constructorFieldStoreTotals() map[string]int {
	if c.fieldStoreTotals != nil {
		return c.fieldStoreTotals
	}
	totals := map[string]int{}
	c.fieldStoreTotals = totals
	if c.obj == nil {
		return totals
	}
	for _, info := range c.obj.Methods {
		name, err := c.obj.getUtf8(info.NameIndex)
		if err != nil || (name != "<init>" && name != "<clinit>") {
			continue
		}
		var codeAttr *CodeAttribute
		for _, attribute := range info.Attributes {
			if ca, ok := attribute.(*CodeAttribute); ok {
				codeAttr = ca
				break
			}
		}
		if codeAttr == nil {
			continue
		}
		func() {
			defer func() { recover() }()
			parser := core.NewDecompiler(codeAttr.Code, func(id int) values.JavaValue {
				return GetValueFromCP(c.ConstantPool, id)
			})
			counts, err := parser.CountFieldStores()
			if err != nil {
				return
			}
			for k, v := range counts {
				totals[k] += v
			}
		}()
	}
	return totals
}

func javaFloatLiteral(f float32) string {
	v := float64(f)
	switch {
	case math.IsNaN(v):
		return "Float.NaN"
	case math.IsInf(v, 1):
		return "Float.POSITIVE_INFINITY"
	case math.IsInf(v, -1):
		return "Float.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(v, 'g', -1, 32) + "F"
}

// javaDoubleLiteral renders a double constant as a valid Java double literal (with a
// 'D' suffix so an integral value is not mistaken for an int), handling NaN/Infinity.
func javaDoubleLiteral(f float64) string {
	switch {
	case math.IsNaN(f):
		return "Double.NaN"
	case math.IsInf(f, 1):
		return "Double.POSITIVE_INFINITY"
	case math.IsInf(f, -1):
		return "Double.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(f, 'g', -1, 64) + "D"
}

// DecompileStubMarker tags a method body that could not be decompiled and was replaced by a
// throwing stub (graceful degradation). Tooling such as the jdsc self-check can scan decompiled
// output for this marker to detect partial results and keep surfacing method-level bugs.
const DecompileStubMarker = "yak-decompiler:"

// malformedTryNoCatchMarker is an internal sentinel emitted when a TryCatchStatement ends up with
// no catch (or finally) handler. That is always a structuring failure -- e.g. a value-producing
// ternary inside the try region confuses the CFG and the catch handler is mis-attributed, leaking
// broken Java like `Exception v = Exception;` that the ANTLR syntax net still accepts. Detecting
// the marker degrades the whole method to an honest stub instead of emitting silently-wrong code.
// It never survives into final output because the offending method is re-rendered as a stub.
const malformedTryNoCatchMarker = "yak-decompiler-internal: try without catch handler"

func normalizeDoWhileBreakGuardSource(body string) string {
	match := doWhileBreakGuardRe.FindStringSubmatchIndex(body)
	if len(match) < 6 {
		return body
	}
	conditionStart, conditionEnd := match[4], match[5]
	condition := strings.TrimSpace(body[conditionStart:conditionEnd])
	if !shouldInvertDoWhileBreakGuard(condition) {
		return body
	}
	return body[:conditionStart] + "!(" + condition + ")" + body[conditionEnd:]
}

func shouldInvertDoWhileBreakGuard(condition string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || strings.HasPrefix(condition, "!") {
		return false
	}
	// Only invert the common structured-loop shape where the positive loop/body
	// condition (`i < n` / `i <= n`) was attached to the synthetic break arm.
	// Already-negative break guards such as `i >= n` are semantically correct as-is.
	if strings.Contains(condition, ">=") || strings.Contains(condition, ">") ||
		strings.Contains(condition, "==") || strings.Contains(condition, "!=") {
		return false
	}
	return strings.Contains(condition, "<")
}

func canFlattenNoCatchTry(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if strings.Contains(body, malformedTryNoCatchMarker) ||
		strings.Contains(body, values.EmptySlotValuePlaceholder) ||
		strings.Contains(body, "= Exception;") ||
		strings.Contains(body, "= Exception\n") {
		return false
	}
	return true
}

// safeDumpMethod wraps DumpMethod with panic recovery and tab-state restoration so a
// single broken method cannot abort the whole class. DumpMethod uses a non-deferred
// Tab()/UnTab() pair, which leaves the indentation stack unbalanced if it panics midway;
// we rewind it here.
func (c *ClassObjectDumper) safeDumpMethod(name, descriptor string) (res *dumpedMethods, err error) {
	tabSaved := c.deepStack.Len()
	defer func() {
		if rec := recover(); rec != nil {
			if os.Getenv("DEC_PANIC_STACK") != "" {
				err = utils.Errorf("panic: %v\n%s", rec, debug.Stack())
			} else {
				err = utils.Errorf("panic: %v", rec)
			}
		}
		for c.deepStack.Len() > tabSaved {
			c.deepStack.Pop()
		}
	}()
	return c.DumpMethod(name, descriptor)
}

// aggressiveRedumpMethod re-decompiles a SINGLE method in aggressive mode. It is the gated retry at
// the heart of the longtail strategy: it is only ever called after the conservative dump already
// failed/degraded for this method, so methods that decompile cleanly never reach it (zero regression
// by construction). It toggles the per-dumper aggressive flag for the duration, evicts the method's
// cache entry so the re-dump actually re-runs, and returns the fresh result only if it is "clean"
// (no error, no leaked internal placeholder / malformed-try marker). On any failure it restores the
// pre-retry cache entry and returns nil, so the caller falls back to its normal stub degradation.
//
// A method is retried at most once (aggressiveRetried guard): it may reach both degradation points
// (DumpMethods and degradeInvalidMethods), but the aggressive path is deterministic so repeating it
// would only waste work and produce the same outcome.
func (c *ClassObjectDumper) aggressiveRedumpMethod(name, descriptor string) *dumpedMethods {
	traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
	if c.aggressiveRetried[traitId] {
		return nil
	}
	c.aggressiveRetried[traitId] = true

	savedAggressive := c.aggressive
	savedEntry, hadEntry := c.dumpedMethodsSet[traitId]
	c.aggressive = true
	delete(c.dumpedMethodsSet, traitId)
	defer func() { c.aggressive = savedAggressive }()

	res, err := c.safeDumpMethod(name, descriptor)
	clean := err == nil && res != nil &&
		!strings.Contains(res.code, values.EmptySlotValuePlaceholder) &&
		!strings.Contains(res.code, malformedTryNoCatchMarker) &&
		// A leaked `varN = Exception;` caught-throwable sentinel is broken Java ("cannot find symbol");
		// reject it so the method keeps its honest stub instead of adopting silently-broken output.
		(os.Getenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF") != "" ||
			(!strings.Contains(res.code, "= Exception;") && !strings.Contains(res.code, "= Exception\n"))) &&
		// Reject results that are syntactically valid but reference a local before its declaration
		// (a slot-reuse renaming bug). Adopting such a result would replace an honest stub with
		// silently-wrong code; keeping the stub upholds the never-emit-broken-code contract until the
		// underlying data-flow bug is fixed.
		!usesLocalBeforeDeclaration(res.code) &&
		// Reject results containing an empty `{ }` block: in aggressive structuring this is the
		// fingerprint of a dropped statement (the assert-ternary idiom collapsing into an empty if
		// body with a leaked unconditional throw). Such output is valid Java but semantically wrong.
		!containsEmptyControlBlock(res.bodyCode)
	if !clean {
		// Restore the exact pre-retry cache state so downstream rendering is unchanged.
		if hadEntry {
			c.dumpedMethodsSet[traitId] = savedEntry
		} else {
			delete(c.dumpedMethodsSet, traitId)
		}
		return nil
	}
	return res
}

// dumpStubMethod builds a syntactically-valid placeholder for a method whose body could
// not be decompiled. It reconstructs the signature purely from the access flags and the
// method descriptor (independent of the bytecode), so a single un-decompilable method
// degrades gracefully instead of failing the entire class. Returns nil when even the
// signature cannot be derived, in which case the caller should drop the method.
func (c *ClassObjectDumper) dumpStubMethod(method *MemberInfo, name, descriptor, reason string) (stub *dumpedMethods) {
	defer func() {
		if rec := recover(); rec != nil {
			stub = nil
		}
	}()
	methodType, perr := types.ParseMethodDescriptor(descriptor)
	if perr != nil || methodType == nil || methodType.FunctionType() == nil {
		return nil
	}
	ft := methodType.FunctionType()
	funcCtx := c.FuncCtx
	funcCtx.IsStatic = method.AccessFlags&StaticFlag == StaticFlag
	accessFlagsVerbose, accessFlags := getMethodAccessFlagsVerbose(method.AccessFlags)
	isVarArgs := slices.Contains(accessFlagsVerbose, "varargs")
	isAbstract := slices.Contains(accessFlagsVerbose, "abstract") || slices.Contains(accessFlagsVerbose, "native")
	isInterface := slices.Contains(c.obj.AccessFlagsVerbose, "interface")

	// Prefer the generic Signature attribute's parameter/return types over the erased descriptor: a stub
	// built from the raw descriptor loses `<...>`, which can break OVERLOAD SPECIFICITY at the stub's call
	// sites (guava Joiner `appendTo(StringBuilder, Iterator)` erased vs `appendTo(A, Iterator<?>)` ->
	// "reference to appendTo is ambiguous"). Gated: only when the method declares NO formal type parameters
	// of its own (sig starts with '(' -- a leading `<...>` would reference method-scope vars this stub does
	// not declare) and the sig parameter count matches the descriptor (offset-safe). Class-scope type vars
	// referenced by the generic types are always in scope for the stub. Kill-switch JDEC_STUB_GENERIC_SIG_OFF.
	paramTypesForRender := ft.ParamTypes
	returnTypeForRender := ft.ReturnType
	if os.Getenv("JDEC_STUB_GENERIC_SIG_OFF") == "" {
		for _, attr := range method.Attributes {
			sigAttr, ok := attr.(*SignatureAttribute)
			if !ok {
				continue
			}
			if sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex); err == nil && strings.HasPrefix(sigStr, "(") {
				if _, sigParams, sigRet := types.ParseMethodSignatureFull(sigStr, funcCtx); sigRet != nil && len(sigParams) == len(ft.ParamTypes) {
					allNonNil := true
					for _, sp := range sigParams {
						if sp == nil {
							allNonNil = false
							break
						}
					}
					if allNonNil {
						paramTypesForRender = sigParams
						returnTypeForRender = sigRet
					}
				}
			}
			break
		}
	}

	paramList := []string{}
	for idx, pt := range paramTypesForRender {
		if isVarArgs && idx == len(paramTypesForRender)-1 && pt.IsArray() {
			paramList = append(paramList, fmt.Sprintf("%s... var%d", pt.ElementType().String(funcCtx), idx))
		} else {
			paramList = append(paramList, fmt.Sprintf("%s var%d", pt.String(funcCtx), idx))
		}
	}
	paramsStr := strings.Join(paramList, ", ")

	// sanitize the failure reason so it can live inside a block comment on one line
	reason = strings.ReplaceAll(reason, "*/", "* /")
	reason = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(reason)
	if len(reason) > 160 {
		reason = reason[:160]
	}

	prefix := ""
	if accessFlags != "" {
		prefix = accessFlags + " "
	}
	// A non-abstract, non-static interface method is a default method.
	if isInterface && !isAbstract && name != "<clinit>" && !strings.Contains(prefix, "static") {
		prefix += "default "
	}
	throwBody := fmt.Sprintf(" { throw new RuntimeException(%s); /* %s %s */ }",
		strconv.Quote(DecompileStubMarker+" undecompilable method body"), DecompileStubMarker, reason)

	var src string
	switch name {
	case "<clinit>":
		src = fmt.Sprintf("static { /* %s undecompilable <clinit>: %s */ }", DecompileStubMarker, reason)
	case "<init>":
		src = fmt.Sprintf("%s%s(%s)%s", prefix, c.GetConstructorMethodName(), paramsStr, throwBody)
	default:
		if isAbstract {
			src = fmt.Sprintf("%s%s %s(%s);", prefix, returnTypeForRender.String(funcCtx), name, paramsStr)
		} else {
			src = fmt.Sprintf("%s%s %s(%s)%s", prefix, returnTypeForRender.String(funcCtx), name, paramsStr, throwBody)
		}
	}
	return &dumpedMethods{methodName: name, code: src, bodyCode: "stub"}
}

// isGenuineEnum reports whether this class is a real `enum` declaration (ACC_ENUM and a
// direct java.lang.Enum supertype), as opposed to a synthetic enum-constant subclass.
func (c *ClassObjectDumper) isGenuineEnum() bool {
	if !slices.Contains(c.obj.AccessFlagsVerbose, "enum") {
		return false
	}
	sup := strings.Replace(c.obj.GetSupperClassName(), "/", ".", -1)
	return sup == "java.lang.Enum"
}

// isSyntheticEnumMethod reports whether a method is one javac auto-generates for every enum
// (values(), valueOf(String), $values()). These must not be emitted: javac re-synthesizes
// them, and emitting them yields "method X is already defined".
func (c *ClassObjectDumper) isSyntheticEnumMethod(name, descriptor string) bool {
	if name == "$values" {
		return true
	}
	selfDesc := "L" + c.obj.GetClassName() + ";"
	if name == "values" && descriptor == "()["+selfDesc {
		return true
	}
	if name == "valueOf" && descriptor == "(Ljava/lang/String;)"+selfDesc {
		return true
	}
	// Synthetic "marker" constructor javac emits for enums that have constant-specific bodies:
	// `<init>(String name, int ordinal, <Enum>$N marker)`. Its sole purpose is to give the constant-body
	// subclasses an accessible super-ctor; its body just forwards to the real `<init>(String,int)`.
	// Emitting it is ALWAYS wrong (it references a synthetic `$N` type that no longer exists once bodies
	// are folded, and renders an illegal `this(...)` after local declarations); javac re-synthesizes it
	// from the folded constant bodies on recompile. Identified by a trailing parameter typed as this
	// enum's OWN anonymous subclass `L<self>$<digits>;` -- a shape impossible to write in source, so the
	// match is exact. Kill-switch JDEC_NO_ENUM_MARKER_CTOR restores the raw (broken) emission.
	if name == "<init>" && os.Getenv("JDEC_NO_ENUM_MARKER_CTOR") == "" && c.isEnumMarkerCtorDescriptor(descriptor) {
		return true
	}
	return false
}

// isEnumMarkerCtorDescriptor reports whether descriptor's LAST parameter is an anonymous synthetic
// class `L<binary>$<digits>;` (e.g. "Lcodec/AccEnum$1;" OR "Lcom/google/common/base/Predicates$1;"),
// the signature of the javac-generated enum marker constructor. The marker's owner is whichever class
// javac happened to allocate the anonymous slot in: for a TOP-LEVEL enum it is the enum's own
// `<self>$N`, but for an enum NESTED in another class (e.g. Predicates.ObjectPredicate) javac names it
// after the ENCLOSING class (`Predicates$1`). So we only require the trailing param to be SOME
// anonymous class (simple name = all digits) -- a shape impossible to write in source, so the match is
// unambiguous. This check is reached only for genuine enums (see DumpMethods), so it never affects
// ordinary classes.
func (c *ClassObjectDumper) isEnumMarkerCtorDescriptor(descriptor string) bool {
	params := methodParamFieldDescriptors(descriptor)
	if len(params) == 0 {
		return false
	}
	last := params[len(params)-1]
	// Must be a plain (non-array) object type Lxxx; .
	if !strings.HasPrefix(last, "L") || !strings.HasSuffix(last, ";") {
		return false
	}
	bin := last[1 : len(last)-1]
	dollar := strings.LastIndexByte(bin, '$')
	if dollar < 0 || dollar == len(bin)-1 {
		return false
	}
	digits := bin[dollar+1:]
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

// methodParamFieldDescriptors splits a method descriptor's parameter list into raw JVM field
// descriptors (e.g. "(Ljava/lang/String;I[B)V" -> ["Ljava/lang/String;", "I", "[B"]).
func methodParamFieldDescriptors(descriptor string) []string {
	open := strings.IndexByte(descriptor, '(')
	closeIdx := strings.IndexByte(descriptor, ')')
	if open < 0 || closeIdx < 0 || closeIdx < open {
		return nil
	}
	params := descriptor[open+1 : closeIdx]
	var out []string
	for i := 0; i < len(params); {
		start := i
		for i < len(params) && params[i] == '[' {
			i++
		}
		if i >= len(params) {
			break
		}
		if params[i] == 'L' {
			semi := strings.IndexByte(params[i:], ';')
			if semi < 0 {
				break
			}
			i += semi + 1
		} else {
			i++
		}
		out = append(out, params[start:i])
	}
	return out
}

// enumConstantArgs derives the explicit constructor arguments for an enum constant from the
// `new <EnumType>(name, ordinal, args...)` expression captured in <clinit>. The first two
// arguments are the synthetic name/ordinal javac injects; the remainder are the source-level
// arguments (e.g. PLANET(mass, radius)). Returns "" for a plain constant with no extra args.
func (c *ClassObjectDumper) enumConstantArgs(name string) string {
	raw := strings.TrimSpace(c.fieldDefaultValue[name])
	if !strings.HasPrefix(raw, "new ") || !strings.HasSuffix(raw, ")") {
		return ""
	}
	open := strings.Index(raw, "(")
	if open < 0 {
		return ""
	}
	parts := splitTopLevelArgs(raw[open+1 : len(raw)-1])
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[2:], ", ")
}

// splitTopLevelArgs splits a comma-separated argument list, ignoring commas nested inside
// (), [], {} or string/char literals.
func splitTopLevelArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i++
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" || len(parts) > 0 {
		parts = append(parts, tail)
	}
	return parts
}

// isSyntheticAccessBridgeCtor reports whether a method is the synthetic access-bridge constructor
// javac emits (pre-nestmates) so an enclosing class can reach a nested class's PRIVATE constructor: an
// ACC_SYNTHETIC `<init>` whose LAST parameter is a synthetic anonymous marker class (binary name
// Outer$N, N all digits). A source-declared constructor can never have an anonymous-class parameter
// type, so this shape is unambiguous.
func (c *ClassObjectDumper) isSyntheticAccessBridgeCtor(descriptor string, accessFlags uint16) bool {
	if !isSyntheticMethod(accessFlags) {
		return false
	}
	mt, err := types.ParseMethodDescriptor(descriptor)
	if err != nil || mt == nil || mt.FunctionType() == nil {
		return false
	}
	pts := mt.FunctionType().ParamTypes
	if len(pts) == 0 {
		return false
	}
	cls, ok := pts[len(pts)-1].RawType().(*types.JavaClass)
	if !ok {
		return false
	}
	name := cls.Name
	if i := strings.LastIndexAny(name, "./"); i >= 0 {
		name = name[i+1:]
	}
	d := strings.LastIndexByte(name, '$')
	if d < 0 || d == len(name)-1 {
		return false
	}
	for _, r := range name[d+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hasOuterThisField reports whether the class carries a synthetic enclosing-instance field `this$0`,
// which javac emits for every non-static inner (member/local/anonymous) class. Its presence identifies
// a class whose constructor's synthetic FIRST parameter is the outer instance -- and whose generic
// Signature attribute OMITS that leading parameter, so the Signature param list aligns to the TRAILING
// descriptor parameters. Used to safely lift generic constructor param types onto the erased descriptor.
func (c *ClassObjectDumper) hasOuterThisField() bool {
	for _, f := range c.obj.Fields {
		if n, err := c.obj.getUtf8(f.NameIndex); err == nil && n == "this$0" {
			return true
		}
	}
	return false
}

// reTypeSyntheticBridgeCtorParams replaces a synthetic access-bridge constructor's erased parameter
// types with the GENERIC parameter types of the private target constructor it forwards to. The bridge
// (last param is an anonymous marker class) carries no Signature attribute; its non-marker parameters
// therefore render as their erased descriptor types. We locate the unique non-synthetic `<init>` whose
// erased parameter list equals the bridge's leading (marker-stripped) parameters, parse its Signature
// attribute, and adopt those generic types in-place on ft.ParamTypes. The marker parameter is left
// untouched. No-op when no matching target or signature is found (output then stays as before).
func (c *ClassObjectDumper) reTypeSyntheticBridgeCtorParams(bridgeDesc string, ft *types.JavaFuncType) {
	if ft == nil {
		return
	}
	bridgeParams := methodParamFieldDescriptors(bridgeDesc)
	targetArity := len(bridgeParams) - 1 // drop the trailing synthetic marker parameter
	if targetArity <= 0 {
		return
	}
	prefix := bridgeParams[:targetArity]
	for _, m := range c.obj.Methods {
		if isSyntheticMethod(m.AccessFlags) {
			continue
		}
		if n, _ := c.obj.getUtf8(m.NameIndex); n != "<init>" {
			continue
		}
		d, _ := c.obj.getUtf8(m.DescriptorIndex)
		if !slices.Equal(methodParamFieldDescriptors(d), prefix) {
			continue
		}
		for _, attr := range m.Attributes {
			sigAttr, ok := attr.(*SignatureAttribute)
			if !ok {
				continue
			}
			sigStr, e := c.obj.getUtf8(sigAttr.SignatureIndex)
			if e != nil || sigStr == "" {
				return
			}
			_, sigParams, _ := types.ParseMethodSignatureFull(sigStr, c.FuncCtx)
			if len(sigParams) != targetArity {
				return
			}
			for i := 0; i < targetArity && i < len(ft.ParamTypes); i++ {
				if sigParams[i] != nil {
					ft.ParamTypes[i] = sigParams[i]
				}
			}
			return
		}
		return
	}
}

func (c *ClassObjectDumper) DumpMethods() ([]*dumpedMethods, error) {
	c.Tab()
	defer c.UnTab()
	genuineEnum := c.isGenuineEnum()
	var result []*dumpedMethods
	for _, method := range c.obj.Methods {
		name, err := c.obj.getUtf8(method.NameIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.NameIndex)
		}
		descriptor, err := c.obj.getUtf8(method.DescriptorIndex)
		if err != nil {
			return nil, utils.Wrapf(err, "getUtf8(%v) failed", method.DescriptorIndex)
		}
		if genuineEnum && c.isSyntheticEnumMethod(name, descriptor) {
			continue
		}
		if v := c.lambdaMethods[name]; slices.Contains(v, descriptor) {
			continue
		}
		// Synthetic lambda bodies (javac emits "lambda$...") must never be dumped as
		// standalone methods: they are only valid inlined as lambda expressions.
		// Dumping them here would also poison the method cache with a method-declaration
		// form, breaking later inline rendering at the invokedynamic call site.
		if strings.HasPrefix(name, "lambda$") && isSyntheticMethod(method.AccessFlags) {
			continue
		}
		// Compiler-generated bridge methods (ACC_BRIDGE, always also ACC_SYNTHETIC) implement
		// covariant returns and generic erasure. They are not source-level declarations; dumping
		// them yields illegal Java (two methods differing only by return type, e.g. `String build()`
		// plus a synthetic `Object build()`). Suppress them so the output mirrors the original
		// source. CFR and Vineflower suppress bridge methods as well.
		if isBridgeMethod(method.AccessFlags) {
			continue
		}
		// if name != "isSymlink" {
		// 	continue
		// }
		res, err := c.safeDumpMethod(name, descriptor)
		if err == nil && res != nil && name == "<clinit>" && c.isInterfaceLike() && isIgnorableAssertionOnlyClinit(res.code) {
			continue
		}
		if err == nil && res != nil && strings.Contains(res.code, values.EmptySlotValuePlaceholder) {
			// The decompiled body leaked an internal placeholder ("empty slot value"),
			// which means the stack simulation was incomplete and the emitted source is
			// not valid Java. Degrade to a stub instead of producing un-compilable code.
			if os.Getenv("DEBUG_EMPTYSLOT") == "" {
				err = utils.Errorf("incomplete stack simulation: empty stack slot leaked into method body")
			} else {
				log.Errorf("DEBUG_EMPTYSLOT method %s%s:\n%s", name, descriptor, res.code)
			}
		}
		if err == nil && res != nil && strings.Contains(res.code, malformedTryNoCatchMarker) {
			// The try-region structuring failed and produced a try with no catch handler,
			// which means the body is semantically corrupted (e.g. the caught-exception
			// placeholder leaked into the try). Degrade to a stub.
			if os.Getenv("DEBUG_TRYNOCATCH") != "" {
				log.Errorf("DEBUG_TRYNOCATCH method %s%s:\n%s", name, descriptor, res.code)
			}
			err = utils.Errorf("try-region structuring failed: try without catch handler")
		}
		if err == nil && res != nil && os.Getenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF") == "" &&
			(strings.Contains(res.code, "= Exception;") || strings.Contains(res.code, "= Exception\n")) {
			// A bare `varN = Exception;` is the fingerprint of a try/finally (or synchronized-region)
			// structuring failure: the handler's caught-throwable stack value could not be bound to a
			// real local, so it rendered as the bare type name `Exception` -- valid to the ANTLR syntax
			// net but "cannot find symbol" to javac (guava Monitor.enterWhen/enterWhenUninterruptibly,
			// InetAddresses). The same signal already blocks no-catch-try flattening (canFlattenNoCatchTry);
			// promote it to a full degradation trigger so the method is retried aggressively and, failing
			// that, degraded to an honest compiling stub instead of leaking broken code. Kill-switch:
			// JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF.
			if os.Getenv("DEBUG_EXCEPTION_SENTINEL") != "" {
				log.Errorf("DEBUG_EXCEPTION_SENTINEL method %s%s:\n%s", name, descriptor, res.code)
			}
			err = utils.Errorf("exception-handler structuring failed: caught-throwable sentinel leaked into method body")
		}
		if err != nil {
			// Gated aggressive retry: this method failed conservative decompilation (error, leaked
			// empty slot, or malformed try). Re-decompile ONLY this method in aggressive mode and
			// adopt the result if it now produces a clean body. Whole-class syntax validation still
			// runs afterwards, so an aggressive result that is clean-looking but invalid Java is
			// caught and re-degraded at the degradeInvalidMethods stage.
			if retry := c.aggressiveRedumpMethod(name, descriptor); retry != nil {
				log.Infof("aggressive retry recovered method %s%s", name, descriptor)
				res = retry
				err = nil
			}
		}
		if name == "<clinit>" && c.isInterfaceLike() {
			// Interfaces and annotations cannot declare a source-level static initializer.
			// DumpMethodWithInitialId has already hoisted any representable final-field
			// assignments into field initializers; leftover helper-array stores have no legal
			// method form and must not be emitted only to be dropped by the syntax safety net.
			continue
		}
		if err != nil {
			// Graceful degradation: an un-decompilable method body must not fail the whole
			// class. Emit a stub method (correct signature, throwing body) so the rest of
			// the class still decompiles.
			log.Warnf("decompile method %s%s failed, emitting stub: %v", name, descriptor, err)
			stub := c.dumpStubMethod(method, name, descriptor, err.Error())
			if stub == nil {
				// even the signature could not be derived; drop the method to keep output valid
				log.Warnf("stub for method %s%s could not be built, skipping", name, descriptor)
				continue
			}
			traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
			c.dumpedMethodsSet[traitId] = stub
			res = stub
		}
		accessFlagsVerbose, _ := getMethodAccessFlagsVerbose(method.AccessFlags)
		if strings.TrimSpace(res.bodyCode) == "" {
			// A synthetic access-bridge constructor whose body decompiled to empty (its `this()`
			// delegation to a trivial no-arg ctor was stripped) must be KEPT, not dropped. javac emits
			// this package-private bridge (pre-nestmates) so an enclosing class can reach a nested
			// class's PRIVATE no-arg constructor; the call site is `new Outer$Inner((Outer$N)null)`.
			// Once nested classes are decompiled as flat top-level `Outer$Inner` units, dropping the
			// bridge leaves that call resolving to no constructor ("constructor cannot be applied to
			// given types" - the single largest guava `base` recompile blocker via
			// Platform$JdkPatternCompiler). The empty body implicitly calls super() exactly as the
			// no-arg target did, so it is semantically faithful. Kill-switch: JDEC_NO_SYN_BRIDGE_CTOR=1.
			isSynBridgeCtor := name == "<init>" && os.Getenv("JDEC_NO_SYN_BRIDGE_CTOR") == "" &&
				c.isSyntheticAccessBridgeCtor(descriptor, method.AccessFlags)
			// A genuinely-declared constructor whose body decompiled to just the implicit super()
			// must be KEPT unless it is indistinguishable from the constructor javac auto-generates
			// when a class declares none. Dropping a programmer-declared no-arg ctor while OTHER
			// ctors exist removes it from the API (e.g. guava `VerifyException()` -> `new
			// VerifyException()` no longer resolves); dropping a non-public SOLE ctor (singleton
			// `private Foo(){}`) silently widens accessibility; dropping a PARAMETERIZED empty-body
			// ctor (`Foo(int){ super(); }`) deletes a real overload. All are semantic regressions
			// the syntax safety net cannot catch. Kill-switch: JDEC_NO_KEEP_DECLARED_CTOR=1.
			keepDeclaredCtor := name == "<init>" && !isSynBridgeCtor &&
				os.Getenv("JDEC_NO_KEEP_DECLARED_CTOR") == "" &&
				!c.isOmittableDefaultCtor(descriptor, accessFlagsVerbose)
			if isSynBridgeCtor || keepDeclaredCtor {
				// keep res as the empty-body constructor (faithful: empty body == implicit super())
			} else if !slices.Contains(accessFlagsVerbose, "abstract") && !slices.Contains(accessFlagsVerbose, "annotation") && !slices.Contains(accessFlagsVerbose, "interface") && !slices.Contains(accessFlagsVerbose, "enum") {
				methodType, perr := types.ParseMethodDescriptor(descriptor)
				descBroken := perr != nil || methodType == nil || methodType.FunctionType() == nil
				isVoid := !descBroken && methodType.FunctionType().ReturnType.String(c.FuncCtx) == "void"
				if descBroken || isVoid {
					// A genuinely-empty void method (its bytecode is just `return`) is a faithful
					// `void f(...) {}` and MUST be emitted, not dropped: dropping silently removes the
					// method from the API and, when it overrides an abstract method (a no-op override
					// such as ObjectWriterBaseModule$VoidObjectWriter.write(JSONWriter,Object,Object,
					// Type,long){}), makes the subclass "not abstract and does not override". Only the
					// trivial-return shape is kept; a void body that decompiled to empty but is NOT
					// backed by a bare return (real content lost) keeps the legacy drop so no half-
					// decompiled body is emitted as if empty. Kill-switch: JDEC_NO_EMIT_EMPTY_VOID=1.
					if isVoid && os.Getenv("JDEC_NO_EMIT_EMPTY_VOID") == "" && methodBodyIsTriviallyEmpty(method) {
						// keep res: renders as the faithful empty-body `void f(...) {}`
					} else {
						continue
					}
				} else {
					stub := c.dumpStubMethod(method, name, descriptor, "empty method body after decompilation")
					if stub == nil {
						continue
					}
					traitId := fmt.Sprintf("name:%s,desc:%s", name, descriptor)
					c.dumpedMethodsSet[traitId] = stub
					res = stub
				}
			}
		}
		// retain identity so the syntax safety net can re-derive a stub if needed
		if res.member == nil {
			res.member = method
		}
		if res.descriptor == "" {
			res.descriptor = descriptor
		}
		result = append(result, res)
	}
	return result, nil
}

func (c *ClassObjectDumper) isInterfaceLike() bool {
	return slices.Contains(c.obj.AccessFlagsVerbose, "interface") || slices.Contains(c.obj.AccessFlagsVerbose, "annotation")
}

// methodBodyIsTriviallyEmpty reports whether the method's bytecode is a genuinely empty body: only
// `nop` (0x00) padding plus exactly one `return` (0xb1, the void return). Such a method is a faithful
// `void f(...) {}` whose empty decompiled body is correct, so it must be emitted rather than dropped.
// The {nop,return}-only test is sound: any opcode carrying operand bytes (one of which could happen to
// be 0xb1) is itself outside {0x00,0xb1}, so its presence is detected and excludes the method, leaving
// only truly empty bodies. A method that decompiled to empty but has richer bytecode (real content the
// decompiler failed to recover) returns false and keeps the legacy drop behavior.
func methodBodyIsTriviallyEmpty(method *MemberInfo) bool {
	if method == nil {
		return false
	}
	var code []byte
	for _, attr := range method.Attributes {
		if ca, ok := attr.(*CodeAttribute); ok {
			code = ca.Code
			break
		}
	}
	if len(code) == 0 {
		return false
	}
	returns := 0
	for _, b := range code {
		switch b {
		case 0x00: // nop
		case 0xb1: // return (void)
			returns++
		default:
			return false
		}
	}
	return returns == 1
}

// isOmittableDefaultCtor reports whether an empty-body constructor (its body decompiled to just the
// implicit super()) is indistinguishable from the no-arg constructor javac auto-generates when a
// class declares NONE, so dropping it is loss-less (javac regenerates an identical one). This holds
// ONLY when all of the following are true:
//   - it takes no parameters (descriptor "()V") -- javac never auto-generates a parameterized ctor;
//   - it is the class's SOLE constructor -- if other ctors exist, no default is generated, so this
//     no-arg ctor was written explicitly and is part of the public API;
//   - its accessibility matches the implicit default's (public for a public class, package-private
//     otherwise) -- a non-public sole ctor (singleton pattern) must be kept or accessibility widens.
//
// Any empty-body constructor failing these is programmer-declared and MUST be emitted.
func (c *ClassObjectDumper) isOmittableDefaultCtor(descriptor string, ctorAccessVerbose []string) bool {
	if descriptor != "()V" {
		return false
	}
	ctorCount := 0
	for _, m := range c.obj.Methods {
		if n, _ := c.obj.getUtf8(m.NameIndex); n == "<init>" {
			ctorCount++
		}
	}
	if ctorCount != 1 {
		return false
	}
	if slices.Contains(ctorAccessVerbose, "protected") || slices.Contains(ctorAccessVerbose, "private") {
		return false
	}
	return slices.Contains(c.obj.AccessFlagsVerbose, "public") == slices.Contains(ctorAccessVerbose, "public")
}

func isIgnorableAssertionOnlyClinit(code string) bool {
	body := strings.TrimSpace(code)
	if !strings.Contains(body, "$assertionsDisabled") {
		return false
	}
	body = strings.TrimPrefix(body, "static")
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "{")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "\n", "")
	body = strings.ReplaceAll(body, "\t", "")
	body = strings.ReplaceAll(body, " ", "")
	return body == "" ||
		body == "if(Record$1.$assertionsDisabled){}else{return;}" ||
		strings.HasPrefix(body, "if(") && strings.Contains(body, "$assertionsDisabled)") &&
			strings.HasSuffix(body, "{}else{return;}")
}

func (c *ClassObjectDumper) dumpConstantPool() ([]string, error) {
	result := []string{}
	for _, constant := range c.obj.ConstantPool {
		switch ret := constant.(type) {
		case *ConstantIntegerInfo:
		case *ConstantFloatInfo:
		case *ConstantLongInfo:
		case *ConstantDoubleInfo:
		case *ConstantUtf8Info:
			result = append(result, ret.Value)
		case *ConstantStringInfo:
		case *ConstantClassInfo:
		case *ConstantFieldrefInfo:
		case *ConstantMethodrefInfo:
		case *ConstantInterfaceMethodrefInfo:
		case *ConstantNameAndTypeInfo:
		case *ConstantMethodTypeInfo:
		case *ConstantMethodHandleInfo:
		case *ConstantInvokeDynamicInfo:
		case *ConstantModuleInfo:
		case *ConstantPackageInfo:
		}
	}
	return result, nil
}

// isBoolReturnIfElse detects the pattern where an if-then-else in a boolean-returning
// method has an empty (or trivially `return true`) then-body and a boolean return in the
// else-body. This is the simplest manifestation of the boolean short-circuit DAG where the
// compiler shared a constant true leaf across both the short-circuit and the fallback.
// We can recover `return cond || elseReturnExpr` from it.
func isBoolReturnIfElse(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) bool {
	// Only applies to boolean-returning methods.
	if funcCtx.FunctionType == nil {
		return false
	}
	retType := ""
	if ft, ok := funcCtx.FunctionType.(*types.JavaFuncType); ok {
		retType = ft.ReturnType.String(funcCtx)
	}
	if retType != "boolean" {
		return false
	}
	// Then-body must be empty or contain only `return true`.
	thenIsTrue := len(ifSt.IfBody) == 0
	if !thenIsTrue && len(ifSt.IfBody) == 1 {
		if rs, ok := ifSt.IfBody[0].(*statements.ReturnStatement); ok {
			thenIsTrue = rs.JavaValue != nil && rs.JavaValue.String(funcCtx) == "true"
		}
	}
	if !thenIsTrue {
		return false
	}
	// Else-body must end with a boolean return.
	if len(ifSt.ElseBody) == 0 {
		return false
	}
	lastElse := ifSt.ElseBody[len(ifSt.ElseBody)-1]
	rs, ok := lastElse.(*statements.ReturnStatement)
	if !ok || rs.JavaValue == nil {
		return false
	}
	return true
}

func buildReturnFromEmptyGuardTernary(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) string {
	if !isEffectivelyEmptyBody(ifSt.IfBody) || ifSt.Condition == nil {
		return ""
	}
	meaningfulElse := meaningfulStatements(ifSt.ElseBody)
	if len(meaningfulElse) != 1 {
		return ""
	}
	ret, ok := meaningfulElse[0].(*statements.ReturnStatement)
	if !ok || ret.JavaValue == nil {
		return ""
	}
	tern, ok := values.UnpackSoltValue(ret.JavaValue).(*values.TernaryExpression)
	if !ok || tern.Condition == nil || tern.TrueValue == nil || tern.FalseValue == nil {
		return ""
	}
	slot, ok := tern.Condition.(*values.SlotValue)
	if !ok || slot.GetValue() != nil {
		return ""
	}
	guard := values.SimplifyConditionValue(values.NewUnaryExpression(
		ifSt.Condition,
		values.Not,
		types.NewJavaPrimer(types.JavaBoolean),
	))
	return fmt.Sprintf("return (%s) ? (%s) : (%s)", guard.String(funcCtx), tern.TrueValue.String(funcCtx), tern.FalseValue.String(funcCtx))
}

func isEffectivelyEmptyBody(body []statements.Statement) bool {
	return len(meaningfulStatements(body)) == 0
}

func isEmptyAssertionsDisabledGuard(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) bool {
	if ifSt == nil || ifSt.Condition == nil {
		return false
	}
	if !isEffectivelyNoOpBody(ifSt.IfBody) || !isEffectivelyNoOpBody(ifSt.ElseBody) {
		return false
	}
	return strings.Contains(ifSt.Condition.String(funcCtx), "$assertionsDisabled")
}

func isEffectivelyNoOpBody(body []statements.Statement) bool {
	for _, st := range meaningfulStatements(body) {
		ret, ok := st.(*statements.ReturnStatement)
		if !ok || ret.JavaValue != nil {
			return false
		}
	}
	return true
}

func meaningfulStatements(body []statements.Statement) []statements.Statement {
	var out []statements.Statement
	for _, st := range body {
		switch st.(type) {
		case *statements.MiddleStatement, *statements.StackAssignStatement:
			continue
		default:
			out = append(out, st)
		}
	}
	return out
}

// buildBoolReturnFromIfElse emits `return cond || elseExpr` from the detected if-else pattern.
func buildBoolReturnFromIfElse(ifSt *statements.IfStatement, funcCtx *class_context.ClassContext) string {
	cond := values.SimplifyConditionValue(ifSt.Condition).String(funcCtx)
	// Extract the return expression from the else body.
	lastElse := ifSt.ElseBody[len(ifSt.ElseBody)-1]
	rs := lastElse.(*statements.ReturnStatement)
	elseExpr := rs.JavaValue.String(funcCtx)
	// If the else body has statements before the return, we can't fold into a single
	// expression; fall back to emitting the if-else as-is.
	if len(ifSt.ElseBody) > 1 {
		return "" // signal: caller should use normal rendering
	}
	return fmt.Sprintf("return (%s) || (%s)", cond, elseExpr)
}
