package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type CustomValue struct {
	Flag           string
	NoOuterCapture bool
	// IsMethodRef distinguishes a method reference (`Type::method`, `receiver::method`, `Type::new`)
	// from an inlined lambda body (`(x) -> ...`). Both carry Flag=="lambda" (so receiver/call-site
	// functional-interface cast logic fires for both), but a method reference binds NATURALLY to a
	// raw SAM (it has no explicit parameter types to mismatch), whereas an explicitly-typed lambda
	// needs the cast to bind. A parameterized FI cast on a method reference is therefore unnecessary
	// and, when the target FI's SAM mentions nested wildcards (e.g. Stream.flatMap's
	// `Function<? super T, ? extends Stream<? extends R>>`), the cast pins a concrete parameterization
	// that defeats javac's poly inference ("method flatMap cannot be applied"). The cast helpers use
	// this flag to skip method references (fastjson2 ObjectReaderCreator.toFieldReaderArray
	// `flatMap(Collection::stream)`).
	IsMethodRef bool
	// InstantiatedMtdDesc carries a lambda/method-reference's invokedynamic instantiatedMethodType
	// descriptor (3rd LambdaMetafactory bootstrap arg), e.g. "(Ljava/lang/Throwable;[Ljava/lang/StackTraceElement;)V".
	// For a method reference passed to a constructor whose formal is a RAW functional interface (raw
	// BiConsumer.accept(Object,Object)), the bare method ref fails to bind ("invalid method reference")
	// because the SAM arity erases to (Object,Object) while the impl method is (Throwable,StackTraceElement[]).
	// The source's `(BiConsumer<Throwable,StackTraceElement[]>) Type::method` cast -- recoverable from
	// this descriptor -- re-targets the SAM so the method ref binds. Set only on the bootstrap method-ref
	// branch; consumed by ctorRawFISAMMethodRefCast (renderArgAt). Empty/unused for lambdas and non-FI uses.
	InstantiatedMtdDesc string
	StringFunc          func(funcCtx *class_context.ClassContext) string
	TypeFunc            func() types.JavaType
	ReplaceFunc         func(oldId *utils.VariableId, newId *utils.VariableId)
}

// ReplaceVar implements JavaValue.
func (v *CustomValue) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if v.ReplaceFunc != nil {
		v.ReplaceFunc(oldId, newId)
	}
}

func (v *CustomValue) Type() types.JavaType {
	return v.TypeFunc()
}
func (v *CustomValue) String(funcCtx *class_context.ClassContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomValue(stringFun func(funcCtx *class_context.ClassContext) string, typeFunc func() types.JavaType, replaceFunc ...func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomValue {
	var rf func(oldId *utils.VariableId, newId *utils.VariableId)
	if len(replaceFunc) > 0 {
		rf = replaceFunc[0]
	}
	return &CustomValue{
		StringFunc:  stringFun,
		TypeFunc:    typeFunc,
		ReplaceFunc: rf,
	}
}
