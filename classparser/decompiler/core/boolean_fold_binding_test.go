package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestNativeBooleanFoldPreservesDeclarationViewAndForwarding(t *testing.T) {
	for _, variant := range []string{"integer word", "conditional", "boolean alias", "numeric declaration", "unsolved declaration", "reference", "cyclic", "nil definition"} {
		t.Run(variant, func(t *testing.T) {
			integer, boolean := types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaBoolean)
			word := values.NewJavaLiteral(1, integer)
			var original values.JavaValue = word
			if variant == "conditional" {
				original = values.NewTernaryExpression(values.NewJavaLiteral(true, boolean), word, values.NewJavaLiteral(0, integer))
			}
			if variant == "boolean alias" {
				original = values.NewJavaRef(utils.NewRootVariableId(), values.NewJavaLiteral(true, boolean), boolean)
			}
			if variant == "reference" {
				original = values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))
			}
			cell := values.NewSlotValue(original, original.Type())

			ref := values.NewJavaRef(utils.NewRootVariableId(), cell, boolean)
			ref.WebDeclType = boolean
			if variant == "cyclic" {
				cycle := values.NewTernaryExpression(values.NewJavaLiteral(true, boolean), word, word)
				cycle.TrueValue = cycle
				ref.Val = cycle
			}
			if variant == "numeric declaration" {
				ref.WebDeclType = integer
			}
			if variant == "unsolved declaration" {
				ref.WebDeclType = nil
			}
			if variant == "nil definition" {
				ref.Val = nil
			}
			got := foldDefinitionValue(ref)
			rejected := variant == "reference" || variant == "cyclic" || variant == "nil definition"
			if rejected {
				if got != ref {
					t.Fatal("unproved definition allowed folding")
				}
				return
			}
			if variant == "numeric declaration" || variant == "unsolved declaration" {
				if got != cell {
					t.Fatal("changed unsolved/numeric declaration")
				}
				return
			}
			if !isExactPrimer(got.Type(), types.JavaBoolean) || got == cell || ref.Val != cell || !isExactPrimer(word.Type(), types.JavaInteger) {
				t.Fatal("lost declared domain or mutated shared word")
			}
			children, known := values.Children(got)
			if !known || len(children) != 1 || children[0] != cell {
				t.Fatal("forwarding dependency lost")
			}
			replacement := values.NewJavaRef(utils.NewRootVariableId(), nil, boolean)
			replacement.Id.SetName("changed")
			cell.ResetValue(replacement)
			if !strings.Contains(got.String(&class_context.ClassContext{}), "changed") {
				t.Fatal("late producer replacement lost")
			}
		})
	}
}
