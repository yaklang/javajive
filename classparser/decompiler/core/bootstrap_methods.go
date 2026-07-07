package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type BuildinBootstrapMethod func(d *Decompiler, sim StackSimulation, typ types.JavaType, args ...values.JavaValue) (values.JavaValue, error)

// concatArgNeedsParens reports whether a string-concatenation argument must be parenthesized to
// preserve precedence when spliced into a `"..." + arg + "..."` chain. A binary expression (e.g.
// `a & b`, `a + b`, `a << b`) renders as `(a) op (b)` WITHOUT wrapping the whole expression, so the
// surrounding concat `+` would capture only its left operand. A ternary (`c ? a : b`) is likewise
// lower precedence than `+`. Atomic operands (variables, literals, field/array accesses, method
// calls, casts, unary ops) need no extra parentheses.
func concatArgNeedsParens(v values.JavaValue) bool {
	switch e := values.UnpackSoltValue(v).(type) {
	case *values.JavaExpression:
		return len(e.Values) == 2
	case *values.TernaryExpression:
		return true
	default:
		return false
	}
}

var buildinBootstrapMethods = map[string]func(args ...values.JavaValue) BuildinBootstrapMethod{
	"java.lang.invoke.StringConcatFactory.makeConcatWithConstants": func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args2 ...values.JavaValue) (values.JavaValue, error) {
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				str1 := args1[0].String(funcCtx)

				for i := 0; i < len(args2); i++ {
					idx := len(args2) - 1 - i
					if idx < 0 || idx >= len(args2) {
						break
					}
					arg := args2[idx]
					newStr := arg.String(funcCtx)
					// String concatenation `+` binds tighter than the bitwise/shift/relational/
					// logical operators, so an interpolated sub-expression such as `n & 0xff`
					// (which renders as `(n) & (255)`) would reparse as `("prefix" + n) & 255`
					// once spliced after a `+`, yielding `String & int` - a compile error. Wrap any
					// binary/ternary concat argument in parentheses so it stays a single operand of
					// the concatenation. Atomic args (variables, literals, calls, casts) are left
					// alone to keep the common output unchanged.
					if concatArgNeedsParens(arg) {
						newStr = "(" + newStr + ")"
					}
					tag := `\u0001`
					str1 = strings.Replace(str1, tag, `" + `+newStr+` + "`, 1)
				}

				if strings.HasSuffix(str1, ` + ""`) {
					str1 = strings.TrimSuffix(str1, ` + ""`)
				}
				return str1

				//str2 := args2[0].String(funcCtx)
				//if len(str1) > 2 && str1[0] == '"' && str1[len(str1)-1] == '"' && strings.HasSuffix(str1, `\u0000`) {
				//	var ok bool
				//	str1, ok = strings.CutSuffix(str1, `\u0000"`)
				//	if ok {
				//		str1 = str1 + `"`
				//	}
				//}
				//return fmt.Sprintf("%s + %s", str1, str2)
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
	"java.lang.invoke.LambdaMetafactory.metafactory": func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args2 ...values.JavaValue) (values.JavaValue, error) {
			// args1 are the bootstrap static arguments:
			//   args1[0] = samMethodType, args1[1] = implMethod(MethodHandle), args1[2] = instantiatedMethodType
			// args2 are the dynamic captured arguments.
			if len(args1) < 2 {
				return nil, fmt.Errorf("lambda metafactory requires at least 2 bootstrap args, got %d", len(args1))
			}
			classMember, ok := args1[1].(*values.JavaClassMember)
			if !ok {
				return nil, fmt.Errorf("lambda metafactory: unexpected impl method handle type %T", args1[1])
			}
			member := classMember.Member
			implClassName := strings.ReplaceAll(classMember.Name, "/", ".")
			currentClassName := strings.ReplaceAll(d.FunctionContext.ClassName, "/", ".")
			// Synthetic lambda bodies are emitted by javac as private methods named "lambda$...".
			// Only those should be inlined as lambda expressions; everything else is a method reference.
			isSyntheticLambda := strings.HasPrefix(member, "lambda$")
			if isSyntheticLambda && implClassName == currentClassName {
				// javac prepends the captured variables to the impl method's parameter list. They are
				// not lambda parameters, so DumpClassLambdaMethod drops the leading `len(captured)`
				// params from the arrow signature and renders each as a placeholder ("\x00LCAPi\x00").
				// We resolve those placeholders here - lazily, at render time, mirroring how
				// StringConcatFactory renders args2 - to the captured value's final name (post var
				// rewrite), so `x -> x + base` reads as the captured `base`, not a spurious parameter.
				// args2 is popped off the operand stack, so it arrives in reverse capture order; restore
				// forward order so captured[i] lines up with the i-th leading impl parameter (otherwise
				// multi-capture lambdas swap their captures, e.g. x*a+y*b becomes x*b+y*a).
				captured := make([]values.JavaValue, len(args2))
				for i := range args2 {
					captured[i] = args2[len(args2)-1-i]
				}
				// A captured value is a live snapshot of an enclosing local (a JavaRef into some JVM
				// slot). It renders LAZILY (below), so its name must track the SAME variable-id
				// rewrites that RewriteVar applies to every other tree reference AFTER stack
				// simulation -- most importantly disjoint-slot SPLITTING: when a slot is reused for
				// two different types across disjoint ranges (e.g. `char c` early, `Class k` later),
				// the rewriter mints a fresh id for the later range (var9 -> var9_1) and redirects
				// every reference to it via ReplaceVar. FunctionCallExpression.ReplaceVar already
				// forwards to each argument, so `declaredFields(x, LAMBDA).ReplaceVar` reaches the
				// lambda CustomValue -- but the base CustomValue has a nil ReplaceFunc, so the
				// captured refs (stored only inside this closure, invisible to the tree walk) never
				// get redirected and keep the STALE base-slot name. Canonical breakage: fastjson2
				// BeanUtils.getField captures the `Class` split of a slot reused for a `char`, so the
				// lambda body renders it as the char `var9` -> "char cannot be dereferenced" +
				// "bad operand types for !=". Forward ReplaceVar to every captured value so they
				// participate in id-rewriting exactly like any other tree ref. ReplaceVar is a
				// no-op unless a ref's id equals oldId, so only redirects for THIS logical variable
				// apply -- structurally inert for lambdas whose captures are never rewritten.
				// Kill-switch: JDEC_LAMBDA_CAPTURE_REBIND_OFF=1 restores the (broken) nil ReplaceFunc.
				var lambdaReplace func(oldId *utils.VariableId, newId *utils.VariableId)
				if os.Getenv("JDEC_LAMBDA_CAPTURE_REBIND_OFF") == "" {
					lambdaReplace = func(oldId *utils.VariableId, newId *utils.VariableId) {
						for _, ca := range captured {
							if ca != nil {
								ca.ReplaceVar(oldId, newId)
							}
						}
					}
				}
				// Each lambda body gets its own fresh variable-id namespace so its formal
				// parameters (var0, var1, ...) never collide with the enclosing method's locals.
				// Captured values are resolved via LCAP placeholders, which are independent of
				// the id chain, so a fresh root is safe.
				methodStr, err := d.DumpClassLambdaMethod(member, classMember.Description, utils.NewRootVariableId(), len(captured))
				if err != nil {
					return nil, fmt.Errorf("dump lambda method `%s.%s` error: %w", classMember.Name, member, err)
				}
				// A lambda body that returns a generic-erased Object while its functional-interface SAM
				// returns a method type variable lost the source's unchecked `return (T) expr;` cast. The
				// erased lambda impl method returns Object (its instantiatedMethodType return is Object),
				// but the FI target -- recovered from the ENCLOSING method's Signature return type, since
				// the lambda is the direct return value and the method declares `Supplier<T>` etc. -- has a
				// type variable in the return position. javac rejects the bare body
				// ("bad return type in lambda expression: Object cannot be converted to T"). Re-emit the cast.
				// Kill-switch: JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF=1.
				var retTypevarCast string
				if os.Getenv("JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF") == "" {
					var instantiatedMT values.JavaValue
					if len(args1) >= 3 {
						instantiatedMT = args1[2]
					}
					retTypevarCast = lambdaReturnPositionTypevar(typ, instantiatedMT)
				}
				cv := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					s := methodStr
					for i, ca := range captured {
						s = strings.ReplaceAll(s, fmt.Sprintf("\x00LCAP%d\x00", i), ca.String(funcCtx))
					}
					if retTypevarCast != "" {
						castTarget := resolveLambdaReturnTypevar(funcCtx, retTypevarCast)
						if castTarget != "" {
							s = injectLambdaReturnCast(s, castTarget)
						}
					}
					return s
				}, func() types.JavaType {
					return typ
				}, lambdaReplace)
				// Mark as a lambda so a call on the lambda itself (the inlined `s.get()` shape) renders
				// with the functional-interface cast it needs to compile, e.g. ((Supplier)(() -> ...)).get().
				cv.Flag = "lambda"
				cv.NoOuterCapture = len(captured) == 0

				// Upgrade the lambda's type from the erased descriptor type to a parameterized
				// type using the instantiatedMethodType (3rd bootstrap arg). This only changes
				// the lambda VALUE's type, not the stack simulation's slot type, so it doesn't
				// interfere with slot reuse.
				if len(args1) >= 3 {
					if upgradedType := inferLambdaTypeFromInstantiated(typ, args1[2]); upgradedType != nil {
						lambdaType := upgradedType
						cv = values.NewCustomValue(cv.StringFunc, func() types.JavaType {
							return lambdaType
						}, lambdaReplace)
						cv.Flag = "lambda"
						cv.NoOuterCapture = len(captured) == 0
					}
				}
				return cv, nil
			}

			// Method reference: constructor / static / (bound|unbound) instance method.
			capturedArgs := append([]values.JavaValue{}, args2...)
			// Upgrade the raw functional-interface type to its instantiated parameterization (3rd
			// bootstrap arg), exactly as the inlined-lambda branch above does. A method reference has
			// no type of its own; without this its value type stays the RAW functional interface (e.g.
			// `Function`), so when it is stored into a local the slot is declared raw `Function` and
			// the assignment `var = Collections::synchronizedList` targets a SAM of `apply(Object)`,
			// against which the reference cannot bind ("incompatible types: invalid method reference").
			// Carrying the instantiated type `Function<List, List>` instead lets the slot adopt the
			// parameterized declaration (`Function<List, List> var = Collections::synchronizedList`),
			// restoring the source target type so the reference binds. This is the method-reference
			// companion of the inlined-lambda upgrade and clears fastjson2 ObjectReaderImpl{List,
			// ListStr,Map,MapMultiValueType} `var = Collections::synchronized*/unmodifiable*` (25
			// "invalid method reference" sites). Kill-switch: JDEC_METHODREF_INSTANTIATED_TYPE_OFF=1.
			refType := typ
			if os.Getenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF") == "" && len(args1) >= 3 {
				if up := inferLambdaTypeFromInstantiated(typ, args1[2]); up != nil {
					refType = up
				}
			}
			// Forward ReplaceVar to captured values for the SAME reason as the inlined-lambda branch:
			// a bound instance method reference `receiver::method` renders its captured receiver
			// lazily (capturedArgs[0].String below), so it must track post-simulation id-rewrites
			// (disjoint-slot splitting etc.) or it renders a stale slot name. Kill-switch shared
			// with the lambda branch: JDEC_LAMBDA_CAPTURE_REBIND_OFF=1.
			var refReplace func(oldId *utils.VariableId, newId *utils.VariableId)
			if os.Getenv("JDEC_LAMBDA_CAPTURE_REBIND_OFF") == "" {
				refReplace = func(oldId *utils.VariableId, newId *utils.VariableId) {
					for _, ca := range capturedArgs {
						if ca != nil {
							ca.ReplaceVar(oldId, newId)
						}
					}
				}
			}
			refVal := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				refMember := member
				if member == "<init>" {
					// Constructor method reference renders as `ClassName::new`. `new` is a Java KEYWORD,
					// not an identifier, so it must bypass SafeIdentifier -- which mangles `new` -> `new_`
					// and produced `ClassName::new_`, an "invalid method reference" (javac then resolves a
					// method literally named `new_`, which does not exist). Kill-switch
					// JDEC_CTOR_METHODREF_FIX_OFF restores the legacy (broken) sanitized form.
					if os.Getenv("JDEC_CTOR_METHODREF_FIX_OFF") == "" {
						return funcCtx.ShortTypeName(implClassName) + "::new"
					}
					refMember = "new"
				} else if len(capturedArgs) > 0 {
					// bound instance method reference: receiver::method
					return capturedArgs[0].String(funcCtx) + "::" + class_context.SafeIdentifier(refMember)
				}
				return funcCtx.ShortTypeName(implClassName) + "::" + class_context.SafeIdentifier(refMember)
			}, func() types.JavaType {
				return refType
			}, refReplace)
			// A method reference, like a lambda, has no target type when used directly as a call
			// receiver (`(C::m).apply(x)` does not compile); flag it so the call site adds the cast.
			refVal.Flag = "lambda"
			refVal.NoOuterCapture = len(capturedArgs) == 0
			refVal.IsMethodRef = true
			return refVal, nil
		}
	},
	"defaultBootstrapMethod": func(args ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args ...values.JavaValue) (values.JavaValue, error) {
			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return "BootstrapMethod()"
			}, func() types.JavaType {
				return typ
			}), nil
		}
	},
}

func init() {
	buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.altMetafactory"] = buildinBootstrapMethods["java.lang.invoke.LambdaMetafactory.metafactory"]
}

func init() {
	buildinBootstrapMethods["java.lang.runtime.ObjectMethods.bootstrap"] = func(args1 ...values.JavaValue) BuildinBootstrapMethod {
		return func(d *Decompiler, sim StackSimulation, typ types.JavaType, args ...values.JavaValue) (values.JavaValue, error) {
			// ObjectMethods.bootstrap args:
			//   args1[0] = recordClass (JavaClassValue)
			//   args1[1] = names ("field1;field2;..." as a JavaLiteral)
			//   args1[2+] = MethodHandles for field getters (resolved to JavaClassMember)
			// Dynamic args: args[0] = receiver (this), [args[1] = other for equals]
			invokedName := d.InvokeDynamicName

			// Extract record class name and simple name
			recordClassName := ""
			if len(args1) > 0 {
				if cv, ok := args1[0].(*values.JavaClassValue); ok {
					recordClassName = cv.String(d.FunctionContext)
				}
			}
			simpleName := recordClassName
			if idx := strings.LastIndex(simpleName, "."); idx >= 0 {
				simpleName = simpleName[idx+1:]
			}
			if idx := strings.LastIndex(simpleName, "$"); idx >= 0 {
				simpleName = simpleName[idx+1:]
			}

			// Extract field names from the "name1;name2;..." literal
			var fieldNames []string
			if len(args1) > 1 {
				if lit, ok := args1[1].(*values.JavaLiteral); ok {
					raw := lit.String(d.FunctionContext)
					// The literal renders with quotes; strip them
					raw = strings.Trim(raw, `"`)
					for _, fn := range strings.Split(raw, ";") {
						fn = strings.TrimSpace(fn)
						if fn != "" {
							fieldNames = append(fieldNames, fn)
						}
					}
				}
			}
			// Fall back to extracting field names from the getter method handles
			if len(fieldNames) == 0 {
				for _, arg := range args1[2:] {
					if cm, ok := arg.(*values.JavaClassMember); ok {
						fieldNames = append(fieldNames, cm.Member)
					}
				}
			}

			// The receiver is args[0] (popped last from stack, so it's the first dynamic arg)
			var receiver values.JavaValue
			if len(args) > 0 {
				receiver = args[0]
			}
			receiverStr := "this"
			if receiver != nil {
				receiverStr = receiver.String(d.FunctionContext)
			}

			var result string
			switch invokedName {
			case "toString":
				parts := []string{}
				for _, fn := range fieldNames {
					parts = append(parts, fn+`=" + `+receiverStr+`.`+fn)
				}
				if len(parts) > 0 {
					result = `"` + simpleName + `[` + strings.Join(parts, ` + ", `) + ` + "]"`
				} else {
					result = `"` + simpleName + `[]"`
				}

			case "hashCode":
				if len(fieldNames) == 0 {
					result = "0"
				} else {
					// java.util.Objects.hash(field1, field2, ...)
					fieldRefs := make([]string, len(fieldNames))
					for i, fn := range fieldNames {
						fieldRefs[i] = receiverStr + "." + fn
					}
					result = "java.util.Objects.hash(" + strings.Join(fieldRefs, ", ") + ")"
					d.FunctionContext.Import("java.util.Objects")
				}

			case "equals":
				if len(args) > 1 {
					other := args[1].String(d.FunctionContext)
					if len(fieldNames) == 0 {
						result = receiverStr + " == " + other
					} else {
						// this.field1 == other.field1 && this.field2 == other.field2 ...
						parts := []string{}
						for _, fn := range fieldNames {
							parts = append(parts, receiverStr+"."+fn+" == "+other+"."+fn)
						}
						result = receiverStr + " == " + other + " || (" + receiverStr + " != null && " + other + " != null && " + strings.Join(parts, " && ") + ")"
					}
				} else {
					result = "false"
				}

			default:
				return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					return "ObjectMethods(" + invokedName + ")"
				}, func() types.JavaType {
					return typ
				}), nil
			}

			return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return result
			}, func() types.JavaType {
				return typ
			}), nil
		}
	}
}

// inferLambdaTypeFromInstantiated upgrades a raw functional-interface type to a
// parameterized type using the instantiatedMethodType (3rd bootstrap arg).
// The instantiatedMethodType is a MethodType constant whose String() returns its
// descriptor (e.g. "(Ljava/lang/Integer;Ljava/lang/Integer;)Ljava/lang/Integer;").
// Only standard JDK functional interfaces are upgraded (exact FQN match).
func inferLambdaTypeFromInstantiated(rawType types.JavaType, instantiatedMethodType values.JavaValue) types.JavaType {
	var desc string
	if cv, ok := instantiatedMethodType.(*values.CustomValue); ok {
		s := cv.String(&class_context.ClassContext{})
		if strings.HasPrefix(s, "(") {
			desc = s
		}
	}
	if desc == "" {
		return nil
	}

	// Get the fully qualified class name from the raw type
	rawName := ""
	if jc, ok := rawType.RawType().(*types.JavaClass); ok {
		rawName = jc.Name
	}
	if rawName == "" {
		return nil
	}

	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		return nil
	}
	mtParams := mt.FunctionType().ParamTypes
	mtRet := mt.FunctionType().ReturnType

	typeArgs := []types.JavaType{}
	switch rawName {
	case "java.util.function.BiFunction":
		if len(mtParams) >= 2 {
			typeArgs = append(typeArgs, mtParams[0], mtParams[1])
		}
		typeArgs = append(typeArgs, mtRet)
	case "java.util.function.Function":
		if len(mtParams) >= 1 {
			typeArgs = append(typeArgs, mtParams[0])
		}
		typeArgs = append(typeArgs, mtRet)
	case "java.util.function.Predicate":
		if len(mtParams) >= 1 {
			typeArgs = append(typeArgs, mtParams[0])
		}
	case "java.util.function.Consumer":
		if len(mtParams) >= 1 {
			typeArgs = append(typeArgs, mtParams[0])
		}
	case "java.util.function.Supplier":
		typeArgs = append(typeArgs, mtRet)
	case "java.util.function.BiConsumer":
		if len(mtParams) >= 2 {
			typeArgs = append(typeArgs, mtParams[0], mtParams[1])
		}
	case "java.util.function.BiPredicate":
		if len(mtParams) >= 2 {
			typeArgs = append(typeArgs, mtParams[0], mtParams[1])
		}
	default:
		return nil
	}

	if len(typeArgs) == 0 {
		return nil
	}
	return types.NewParameterizedType(rawName, typeArgs)
}

// lambdaFIReturnPosition is the type-argument index that supplies the SAM's RETURN type for each
// standard JDK functional interface whose SAM returns a (possibly type-variable) value. -1 means
// the FI's SAM returns void (Consumer/BiConsumer) and no return cast ever applies.
var lambdaFIReturnPosition = map[string]int{
	"java.util.function.Supplier":    0,
	"java.util.function.Function":    1,
	"java.util.function.BiFunction":  2,
	"java.util.function.Predicate":   -1,
	"java.util.function.Consumer":    -1,
	"java.util.function.BiConsumer":  -1,
	"java.util.function.BiPredicate": -1,
}

// lambdaReturnPositionTypevar returns the FI's raw class name when the lambda is a JDK
// Supplier/Function/BiFunction whose instantiatedMethodType return type is Object (erased) -- the
// necessary precondition for a return-position type-variable cast. The actual type variable name
// is resolved later from the enclosing method's Signature (see resolveLambdaReturnTypevar), since
// the lambda value carries only the erased/instantiated type, not the enclosing method's type vars.
// Returns "" when no cast applies (non-Object return, or an FI whose SAM returns void).
func lambdaReturnPositionTypevar(rawType types.JavaType, instantiatedMethodType values.JavaValue) string {
	if rawType == nil {
		return ""
	}
	jc, ok := rawType.RawType().(*types.JavaClass)
	if !ok || jc == nil {
		return ""
	}
	pos, known := lambdaFIReturnPosition[jc.Name]
	if !known || pos < 0 {
		return ""
	}
	// The instantiatedMethodType's return type must be the erased Object: only then did the body lose
	// a type-variable cast. A concrete instantiated return (e.g. `()Ljava/lang/String;`) binds directly
	// and must not be cast.
	if instantiatedMethodType == nil {
		return ""
	}
	desc := ""
	if cv, ok := instantiatedMethodType.(*values.CustomValue); ok {
		if s := cv.String(&class_context.ClassContext{}); strings.HasPrefix(s, "(") {
			desc = s
		}
	}
	if desc == "" {
		return ""
	}
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		return ""
	}
	ft := mt.FunctionType()
	if ft == nil || ft.ReturnType == nil {
		return ""
	}
	retClass, ok := ft.ReturnType.RawType().(*types.JavaClass)
	if !ok || retClass == nil || retClass.Name != "java.lang.Object" {
		return ""
	}
	return jc.Name
}

// resolveLambdaReturnTypevar recovers the type-variable NAME the return cast should target, from
// the ENCLOSING method's generic Signature. The lambda is the method's direct return value (the
// fastjson2 `public <T> Supplier<T> m() { ...; return () -> ...createInstance(); }` shape), so the
// method Signature's return type is the FI with its real type-variable argument. Returns "" when the
// enclosing method has no Signature, the return type is not a matching parameterized FI, or the
// return-position type argument is not a bare type variable.
func resolveLambdaReturnTypevar(funcCtx *class_context.ClassContext, fiRawName string) string {
	if funcCtx == nil || funcCtx.CurrentMethodSig == "" {
		return ""
	}
	sig := funcCtx.CurrentMethodSig
	// Strip a leading `<TypeParam:Bound;...>` formal-type-parameter declaration so ParseMethodSignature
	// (which expects a leading `(`) sees the parameter/return body.
	if strings.HasPrefix(sig, "<") {
		depth := 0
		for i, c := range sig {
			if c == '<' {
				depth++
			} else if c == '>' {
				depth--
				if depth == 0 {
					sig = sig[i+1:]
					break
				}
			}
		}
	}
	_, ret := types.ParseMethodSignature(sig)
	if ret == nil {
		return ""
	}
	pt, ok := types.AsParameterizedType(ret)
	if !ok || pt.RawClassName != fiRawName {
		return ""
	}
	pos, _ := lambdaFIReturnPosition[fiRawName]
	if pos >= len(pt.TypeArgs) {
		return ""
	}
	ta := pt.TypeArgs[pos]
	if ta == nil {
		return ""
	}
	// A bare type variable parses as a *JavaClass whose Name is a single identifier (no dot), e.g. "T".
	// A concrete class arg (Object/String/...) has a dotted FQN and binds directly, so no cast.
	jc, ok := ta.RawType().(*types.JavaClass)
	if !ok || jc == nil || strings.Contains(jc.Name, ".") {
		return ""
	}
	return jc.Name
}

// injectLambdaReturnCast rewrites a statement-lambda body `(...) -> { ... return EXPR; ... }` so the
// body's value `return EXPR;` becomes `return (TYPEVAR) (EXPR);`, re-emitting the source's unchecked
// cast on the erased Object return. The value return is the LAST `return ` token in the rendered arrow
// body (javac's lambda body shape), and its expression is the run of non-`;`/non-`}` characters up to
// the terminating `;`. A bare `return;` (void) is left untouched. Returns the body unchanged if no
// return site matches. Kill-switch: JDEC_LAMBDA_RETURN_TYPEVAR_CAST_OFF.
func injectLambdaReturnCast(body, typevar string) string {
	idx := strings.LastIndex(body, "return ")
	if idx < 0 {
		return body
	}
	exprStart := idx + len("return ")
	// Skip leading whitespace of the return expression; remember how many bytes we trimmed so the
	// tail splice stays byte-aligned.
	skipped := 0
	for exprStart+skipped < len(body) && (body[exprStart+skipped] == ' ' || body[exprStart+skipped] == '\t') {
		skipped++
	}
	rest := body[exprStart+skipped:]
	if strings.HasPrefix(rest, ";") {
		return body // void return; nothing to cast
	}
	end := strings.IndexByte(rest, ';')
	if end < 0 {
		return body
	}
	expr := strings.TrimSpace(rest[:end])
	castStmt := "(" + typevar + ") (" + expr + ");"
	return body[:exprStart] + castStmt + body[exprStart+skipped+end+1:]
}
