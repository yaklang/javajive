package core

import (
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// InlineEnumLambdaArgumentTemps restores a compiler-spilled enum argument in
// the value tree before constants are lifted out of <clinit>. Printed local
// names and lambda bodies are not dependency evidence. Require one direct use,
// adjacent statements, no captures, and the original allocation/lambda/invoke
// order in one handler domain. Moving across an effectful earlier argument or
// an opaque use is unsupported, so the original statements remain visible.
func (d *Decompiler) InlineEnumLambdaArgumentTemps(root *[]statements.Statement, constants map[string]int) int {
	if d == nil || root == nil || d.FunctionContext == nil || d.FunctionContext.FunctionName != "<clinit>" ||
		len(constants) == 0 || len(*root) > 512 {
		return 0
	}
	owner := strings.ReplaceAll(d.FunctionContext.ClassName, "/", ".")
	changed := 0
	for i := len(*root) - 2; i >= 0; i-- {
		producer, ok := (*root)[i].(*statements.AssignStatement)
		if !ok || producer.ArrayMember != nil || !producer.HasOriginPC {
			continue
		}
		temp, ok := values.UnpackSoltValue(producer.LeftValue).(*values.JavaRef)
		if !ok || temp == nil || temp.IsThis {
			continue
		}
		lambda, ok := values.UnpackSoltValue(producer.JavaValue).(*values.CustomValue)
		if !ok || lambda == nil || lambda.Flag != "lambda" || !lambda.CapturesKnown || !lambda.NoOuterCapture ||
			len(lambda.Captures) != 0 || !lambda.HasOriginPC {
			continue
		}
		consumer, ok := (*root)[i+1].(*statements.AssignStatement)
		if !ok || consumer.ArrayMember != nil || !consumer.HasOriginPC {
			continue
		}
		field, ok := values.UnpackSoltValue(consumer.LeftValue).(*values.JavaClassMember)
		if !ok || field == nil || strings.ReplaceAll(field.Name, "/", ".") != owner {
			continue
		}
		ordinal, constant := constants[field.Member]
		allocation, ok := values.UnpackSoltValue(consumer.JavaValue).(*values.NewExpression)
		if !constant || !ok || allocation == nil || allocation.IsArray() || !allocation.HasOriginPC ||
			allocation.ConstructorCall == nil || allocation.Type() == nil {
			continue
		}
		call := allocation.ConstructorCall
		if values.UnpackSoltValue(call.Object) != allocation || strings.ReplaceAll(call.ClassName, "/", ".") != owner || call.FunctionName != "<init>" ||
			len(call.Arguments) < 3 || !enumSyntheticName(call.Arguments[0], field.Member) {
			continue
		}
		index, ok := values.UnpackSoltValue(call.Arguments[1]).(*values.JavaLiteral)
		if !ok || index.Data != ordinal || !(allocation.OriginPC < lambda.OriginPC &&
			lambda.OriginPC <= producer.OriginPC && producer.OriginPC < call.OriginPC && call.OriginPC <= consumer.OriginPC) {
			continue
		}
		handlers := d.handlersAtPC(lambda.OriginPC)
		if !slices.Equal(handlers, d.handlersAtPC(allocation.OriginPC)) ||
			!slices.Equal(handlers, d.handlersAtPC(call.OriginPC)) ||
			!slices.Equal(handlers, d.handlersAtPC(consumer.OriginPC)) ||
			statementTreeReferencesLocal(*root, temp, producer, consumer) {
			continue
		}
		position, valid := -1, true
		for j, arg := range call.Arguments {
			if ref, ok := values.UnpackSoltValue(arg).(*values.JavaRef); ok && values.SameLocal(ref, temp) {
				if position != -1 || j < 2 {
					valid = false
				}
				position = j
				continue
			}
			effects, refs := values.InspectValue(arg)
			if effects&values.EffectOpaque != 0 {
				valid = false
			}
			for ref := range refs {
				if values.SameLocal(ref, temp) {
					valid = false
				}
			}
		}
		if !valid || position < 2 {
			continue
		}
		for _, prefix := range call.Arguments[:position] {
			if effects, _ := values.InspectValue(prefix); effects != 0 {
				valid = false
			}
		}
		if !valid {
			continue
		}
		call.Arguments[position] = producer.JavaValue
		copy((*root)[i:], (*root)[i+1:])
		(*root)[len(*root)-1] = nil
		*root = (*root)[:len(*root)-1]
		changed++
	}
	return changed
}

func enumSyntheticName(value values.JavaValue, name string) bool {
	literal, ok := values.UnpackSoltValue(value).(*values.JavaLiteral)
	if !ok {
		return false
	}
	if literal.Units != nil {
		return string(utf16.Decode(literal.Units)) == name
	}
	switch data := literal.Data.(type) {
	case string:
		return data == name
	case []uint16:
		return string(utf16.Decode(data)) == name
	}
	return false
}
