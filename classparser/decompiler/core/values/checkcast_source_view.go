package values

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// NewOriginalCheckCast retains the actual JVM target independently of later
// source-type adaptation. Planning/overload casts must not acquire this witness.
func NewOriginalCheckCast(value JavaValue, target types.JavaType, pc int) *CastExpression {
	descriptor := checkCastReferenceDescriptor(target, nil, 0)
	return &CastExpression{Value: value, TargetType: target, OriginPC: pc, OriginalCheckCast: true, originalCheckCastDescriptor: descriptor}
}

// OriginalCheckCastWitness binds a retained source cast to its immutable
// decoded target and PC. Synthetic binding casts and later target mutations
// cannot supply an original runtime-check witness.
func (c *CastExpression) OriginalCheckCastWitness(ctx *class_context.ClassContext) (int, string, bool) {
	if c == nil || ctx == nil || c.Binding || !c.OriginalCheckCast || c.OriginPC < 0 || c.originalCheckCastDescriptor == "" || checkCastReferenceDescriptor(c.TargetType, ctx, 0) != c.originalCheckCastDescriptor {
		return 0, "", false
	}
	return c.OriginPC, c.originalCheckCastDescriptor, true
}

// This is a widening reference view, not a second runtime check. It is kept
// separate from CastExpression so effects and original target ownership remain
// attached to the single original CHECKCAST.
type checkCastObjectView struct{ Value JavaValue }

func (v *checkCastObjectView) Type() types.JavaType { return types.NewJavaClass("java.lang.Object") }
func (v *checkCastObjectView) String(ctx *class_context.ClassContext) string {
	return "((java.lang.Object)(" + v.Value.String(ctx) + "))"
}
func (v *checkCastObjectView) ReplaceVar(old, next *utils.VariableId) { v.Value.ReplaceVar(old, next) }

func (c *CastExpression) needsObjectCheckCastView(operand JavaValue, ctx *class_context.ClassContext) bool {
	if c == nil || c.Binding || !c.OriginalCheckCast || c.OriginPC < 0 || operand == nil || ctx == nil || ctx.InvocationMetadata == nil {
		return false
	}
	target := checkCastReferenceDescriptor(c.TargetType, ctx, 0)
	if target == "" || target != c.originalCheckCastDescriptor {
		return false
	}
	if !checkCastFixedSourceOperand(operand, ctx, map[*JavaRef]bool{}, 0) {
		return false
	}
	source := checkCastReferenceDescriptor(operand.Type(), ctx, 0)
	if source == "" || source == target {
		return false
	}
	return checkCastDisjoint(source, target, ctx.InvocationMetadata, 0)
}

// Read raw class identity, never a source short-name rendering. Type variables,
// opaque types and cycles/deep array wrappers do not supply descriptor evidence.
func checkCastReferenceDescriptor(t types.JavaType, ctx *class_context.ClassContext, depth int) string {
	if t == nil || depth > 255 {
		return ""
	}
	if t.IsArray() {
		arr, ok := t.RawType().(*types.JavaArrayType)
		if !ok || arr == nil || arr.JavaType == nil || arr.Dimension < 1 || arr.Dimension > 255-depth {
			return ""
		}
		element := checkCastReferenceDescriptor(arr.JavaType, ctx, depth+arr.Dimension)
		if element == "" {
			if primitive, ok := arr.JavaType.RawType().(*types.JavaPrimer); ok {
				element = map[string]string{"byte": "B", "char": "C", "double": "D", "float": "F", "int": "I", "long": "J", "short": "S", "boolean": "Z"}[primitive.Name]
			}
		}
		if element == "" {
			return ""
		}
		return strings.Repeat("[", arr.Dimension) + element
	}
	var name string
	switch raw := t.RawType().(type) {
	case *types.JavaClass:
		if raw == nil {
			return ""
		}
		name = raw.Name
	case *types.JavaParameterizedType:
		if raw == nil {
			return ""
		}
		name = raw.RawClassName
	default:
		return ""
	}
	name = strings.ReplaceAll(name, ".", "/")
	if name == "" || strings.ContainsAny(name, "[];<>(): ") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return ""
	}
	// Unqualified names can be caller formals. Accept them only when an exact
	// declaration provider independently resolves their binary identity.
	if ctx != nil && !strings.Contains(name, "/") {
		if ctx.InvocationMetadata == nil {
			return ""
		}
		decl, ok := ctx.InvocationMetadata(name)
		if !ok || decl.Name != name {
			return ""
		}
	}
	return "L" + name + ";"
}

func checkCastDisjoint(source, target string, provider callbinding.Provider, depth int) bool {
	if depth > 255 || source == target || source == "Ljava/lang/Object;" || target == "Ljava/lang/Object;" {
		return false
	}
	sa, ta := strings.HasPrefix(source, "["), strings.HasPrefix(target, "[")
	if sa && ta {
		sc, tc := source[1:], target[1:]
		sr, tr := callbinding.Reference(sc), callbinding.Reference(tc)
		if !sr || !tr {
			return sc != tc
		}
		return checkCastDisjoint(sc, tc, provider, depth+1)
	}
	if sa || ta {
		named := source
		if sa {
			named = target
		}
		return named != "Ljava/lang/Cloneable;" && named != "Ljava/io/Serializable;"
	}
	if !callbinding.Reference(source) || !callbinding.Reference(target) || provider == nil {
		return false
	}
	s, sok := provider(callbinding.Name(source))
	t, tok := provider(callbinding.Name(target))
	if !sok || !tok || s.Name != callbinding.Name(source) || t.Name != callbinding.Name(target) {
		return false
	}
	if s.IsInterface && t.IsInterface {
		return false
	}
	// A missing path does not prove non-subtyping. Every ancestor declaration
	// must be present, identity matched and complete within fixed work bounds.
	relatedSource, completeSource := checkCastAncestorClosure(s.Name, t.Name, provider)
	relatedTarget, completeTarget := checkCastAncestorClosure(t.Name, s.Name, provider)
	if !completeSource || !completeTarget || relatedSource || relatedTarget {
		return false
	}
	if !s.IsInterface && !t.IsInterface {
		return true
	}
	return (!s.IsInterface && s.Final) || (!t.IsInterface && t.Final)
}

func checkCastAncestorClosure(source, target string, provider callbinding.Provider) (bool, bool) {
	state := map[string]uint8{}
	related := false
	nodes, edges := 0, 0
	var visit func(string, int) bool
	visit = func(name string, depth int) bool {
		if depth > 128 || state[name] == 1 {
			return false
		}
		if state[name] == 2 {
			return true
		}
		nodes++
		if nodes > 128 {
			return false
		}
		decl, ok := provider(name)
		if !ok || decl.Name != name || !decl.ParentsComplete || (decl.Final && decl.IsInterface) {
			return false
		}
		state[name] = 1
		if name == target {
			related = true
		}
		for _, parent := range decl.Parents {
			edges++
			if edges > 1024 || !visit(parent, depth+1) {
				return false
			}
		}
		state[name] = 2
		return true
	}
	complete := visit(source, 0)
	return related, complete
}

// A poly/generic producer can change when its source target context changes.
// Resolve only fixed declaration results and ordinary materialized source uses;
// unknown/custom producers and cyclic inline aliases are refused.
func checkCastFixedSourceOperand(v JavaValue, ctx *class_context.ClassContext, seen map[*JavaRef]bool, depth int) bool {
	if depth > 64 || v == nil {
		return false
	}
	v = UnpackSoltValue(v)
	switch x := v.(type) {
	case *JavaRef:
		if x == nil || seen[x] {
			return false
		}
		if x.StackVar == nil {
			return true
		}
		seen[x] = true
		return checkCastFixedSourceOperand(x.StackVar, ctx, seen, depth+1)
	case *JavaLiteral:
		return x != nil
	case *JavaClassMember:
		return x != nil
	case *JavaArrayMember:
		return x != nil
	case *CastExpression:
		return x != nil && x.TargetType != nil
	case *FunctionCallExpression:
		if x == nil || ctx == nil || ctx.InvocationMetadata == nil || x.Kind == InvokeDynamic {
			return false
		}
		owner := strings.ReplaceAll(x.ClassName, ".", "/")
		decl, known := ctx.InvocationMetadata(owner)
		if !known || decl.Name != owner || !decl.MembersComplete {
			return false
		}
		_, result, err := callbinding.Descriptor(x.Descriptor)
		if err != nil || result != checkCastReferenceDescriptor(x.Type(), ctx, 0) {
			return false
		}
		fixed := false
		for _, m := range decl.Methods {
			if m.Name == x.FunctionName && m.Desc == x.Descriptor {
				if m.Generic || m.Signature != "" {
					return false
				}
				fixed = true
			}
		}
		return fixed
	default:
		return false
	}
}
