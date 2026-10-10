package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestNativeMethodLocalProducerRuntimeCheckCannotBecomeBindingCast(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class CheckedFactOwner{static Object first(Object a,Object b){return a;}Object make(Object a,Object b){final CharSequence made=(CharSequence)first(a,b);class Entry{CharSequence get(){return made;}}return new Entry();}}`, debug)
			for _, variant := range []string{"original", "binding cast", "missing runtime witness", "changed target", "changed cast PC", "missing producer", "swapped equal typed arguments", "changed producer descriptor", "changed SSA cast operand", "non invocation origin", "deep cast chain", "cyclic SSA cast chain", "cyclic source cast"} {
				t.Run(variant, func(t *testing.T) {
					d, local, slots, body := nativeMethodLocalProducerTestBody(t, files, "CheckedFactOwner")
					var assign *statements.AssignStatement
					var cast *values.CastExpression
					for _, st := range body {
						if a, ok := st.(*statements.AssignStatement); ok {
							v, _ := nativeMemberEnclosingUnpack(a.JavaValue, nil)
							if c, ok := v.(*values.CastExpression); ok {
								assign = a
								cast = c
							}
						}
					}
					if assign == nil || cast == nil {
						t.Fatal("actual retained runtime check")
					}
					v, _ := nativeMemberEnclosingUnpack(cast.Value, nil)
					call, ok := v.(*values.FunctionCallExpression)
					if !ok || call.FunctionName != "first" {
						t.Fatal("actual cast operand invocation")
					}
					var origin ssabuild.Origin
					var site nativeMethodLocalAllocation
					for _, s := range local.allocations {
						site = s
						origin = s.origins[local.constructor.captures["val$made"]]
						break
					}
					if site.producers[int(origin.PC)].instruction.Opcode != core.OP_CHECKCAST {
						t.Fatal("original SSA origin not CHECKCAST")
					}
					switch variant {
					case "binding cast":
						cast.Binding = true
					case "missing runtime witness":
						cast.OriginalCheckCast = false
					case "changed target":
						cast.TargetType = types.NewJavaClass("java.lang.String")
					case "changed cast PC":
						cast.OriginPC++
					case "missing producer":
						cast.Value = slots[1]
					case "swapped equal typed arguments":
						call.Arguments[0], call.Arguments[1] = call.Arguments[1], call.Arguments[0]
					case "changed producer descriptor":
						call.Descriptor = "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/CharSequence;"
					case "changed SSA cast operand":
						r := site.producers[int(origin.PC)]
						r.record.Uses = append([]ssabuild.Origin(nil), r.record.Uses...)
						r.record.Uses[0] = ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 1}
						site.producers[int(origin.PC)] = r
					case "non invocation origin":
						r := site.producers[int(origin.PC)]
						r.instruction.Opcode = core.OP_GETFIELD
						site.producers[int(origin.PC)] = r
					case "deep cast chain":
						for i := 0; i < 40; i++ {
							cast.Value = values.NewOriginalCheckCast(cast.Value, cast.TargetType, cast.OriginPC)
						}
					case "cyclic SSA cast chain":
						r := site.producers[int(origin.PC)]
						r.record.Uses = []ssabuild.Origin{origin}
						site.producers[int(origin.PC)] = r
					case "cyclic source cast":
						cast.Value = cast
					}
					_, _, admitted := d.nativeMethodLocalProducerBindings(local, body, slots)
					if admitted != (variant == "original") {
						t.Fatalf("admitted=%v", admitted)
					}

				})
			}
		})
	}
}
