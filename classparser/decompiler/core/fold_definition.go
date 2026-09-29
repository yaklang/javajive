package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// foldDefinitionValue returns only this variable's defining expression. An
// alias is a read of the already computed value. Never follow an intervening
// variable back to its initializer: its evaluation count, identity and current
// state belong to that variable's definition, not to this copy.
func foldDefinitionValue(ref *values.JavaRef) values.JavaValue {
	if ref == nil {
		return nil
	}
	if ref.Val == nil {
		return ref
	}
	if cv, ok := ref.Val.(*values.CustomValue); ok && cv.Flag == "param_placeholder" {
		return ref
	}
	return ref.Val
}
