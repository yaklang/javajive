package values

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestErasedIntersectionOverloadKeepsAllConstraintsWithoutNewChecks(t *testing.T) {
	for _, variant := range []string{"original", "unknown bound", "private bound", "second class", "duplicate bound", "missing assignability", "competing interface", "competing generic", "original check"} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, meta, sig := boundedOverloadFixture()
			*sig = "<A::Ljava/lang/Appendable;:Ljava/io/Serializable;>(TA;Ljava/util/Iterator<*>;)TA;^Ljava/io/IOException;"
			meta["java/lang/Appendable"] = callbinding.Class{Name: "java/lang/Appendable", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true}
			meta["java/io/Serializable"] = callbinding.Class{Name: "java/io/Serializable", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true}
			m := meta["java/lang/StringBuilder"]
			m.Parents = append(m.Parents, "java/io/Serializable")
			meta[m.Name] = m
			owner := meta["probe/Owner"]
			switch variant {
			case "unknown bound":
				delete(meta, "java/io/Serializable")
			case "private bound":
				m = meta["java/io/Serializable"]
				m.Public = false
				meta[m.Name] = m
			case "second class":
				m = meta["java/io/Serializable"]
				m.IsInterface = false
				meta[m.Name] = m
			case "duplicate bound":
				*sig = strings.Replace(*sig, "Ljava/io/Serializable;", "Ljava/lang/Appendable;", 1)
			case "missing assignability":
				m = meta["java/lang/StringBuilder"]
				m.Parents = m.Parents[:2]
				meta[m.Name] = m
			case "competing interface", "competing generic":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "apply", Desc: "(Ljava/io/Serializable;Ljava/util/Iterator;)Ljava/lang/Object;", Generic: variant == "competing generic", Public: true})
			case "original check":
				f.Arguments[0] = NewOriginalCheckCast(NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object")), types.NewJavaClass("java.lang.StringBuilder"), 19)
			}
			meta[owner.Name] = owner
			original := append([]JavaValue(nil), f.Arguments...)
			out, ok := f.PlanErasedDiscardedMethodInput(ctx)
			want := variant == "original" || variant == "original check"
			if ok != want {
				t.Fatalf("plan=%v", ok)
			}
			if !ok {
				return
			}
			cast, valid := out.Arguments[0].(*CastExpression)
			if !valid || len(cast.bindingIntersection) != 2 || cast.Value != original[0] || cast.OriginPC != 37 || cast.OriginalCheckCast || !cast.Binding || out.Witness() != f.Witness() || out.Object != f.Object || bindingType(cast.Type()) != "Ljava/lang/Appendable;" || !reflect.DeepEqual(f.Arguments, original) {
				t.Fatal("lost call, operand, check position or JVM erasure")
			}
			if _, allowed := f.planErasedMethodInput(ctx); allowed {
				t.Fatal("discarded generic result escaped its use site")
			}
			if argument := out.ArgumentStrings(ctx)[0]; !strings.Contains(argument, "java.lang.Appendable & java.io.Serializable") {
				t.Fatalf("final call renderer lost constraints: %s", argument)
			}
			if variant == "original check" {
				pc, descriptor, known := cast.Value.(*CastExpression).OriginalCheckCastWitness(ctx)
				if !known || pc != 19 || descriptor != "Ljava/lang/StringBuilder;" {
					t.Fatal("original check witness changed")
				}
			}
		})
	}
}
