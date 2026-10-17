package core

import (
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// A loop's back edge joins its initial and final definitions, while reads
// between consecutive mutations form separate reaching-definition webs.
// These are distinct VALUE webs but each ++/-- must write the same LOCATION.
func TestNumericUpdateLoopPreservesOneSourceLocationAcrossValueWebs(t *testing.T) {
	for _, kind := range []string{types.JavaLong, types.JavaFloat, types.JavaDouble} {
		for _, updates := range []int{2, 3, 4, 7} {
			for _, subtract := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/updates=%d/subtract=%t", kind, updates, subtract), func(t *testing.T) {
					d, firstPath, ref, _ := localPostTestProtocol(kind, subtract, 1, false)
					p := localPostProtocols[kind]
					initialize := &OpCode{Instr: &Instruction{}, CurrentOffset: 0}
					// Fixed slot encodings are separated by four for every numeric category.
					initialize.Instr.OpCode = OP_ISTORE_0 + (p.store-OP_ISTORE)*4 + 1
					zero := values.NewJavaLiteral(0, ref.Type())
					initialize.stackConsumed = []values.JavaValue{zero}
					d.opCodes = []*OpCode{initialize}
					d.opcodeIdToRef = map[*OpCode][][2]any{initialize: {{ref, true}}}
					d.slotStoreValue = map[*OpCode]values.JavaValue{}
					var first, previous *OpCode
					var expressions []*values.JavaExpression
					offset := 10
					for i := 0; i < updates; i++ {
						path := firstPath
						if i > 0 {
							_, path, _, _ = localPostTestProtocol(kind, subtract, 1, false)
						}
						word := values.NewSlotValue(ref, ref.Type())
						path[0].stackProduced = []values.JavaValue{word}
						d.stackValueProducers[word] = path[0]
						for _, node := range path {
							node.CurrentOffset = uint16(offset)
							width, _ := localPostInstructionWidth(node)
							offset += width
						}
						if first == nil {
							first = path[0]
							first.Source = []*OpCode{initialize}
							initialize.Target = []*OpCode{first}
						} else {
							path[0].Source = []*OpCode{previous}
							previous.Target = []*OpCode{path[0]}
						}
						store := path[len(path)-1]
						operation := values.INC
						arithmetic := values.ADD
						if subtract {
							operation = values.DEC
							arithmetic = values.SUB
						}
						rhs := values.NewBinaryExpression(word, values.NewJavaLiteral(1, ref.Type()), arithmetic, ref.Type())
						store.stackConsumed = []values.JavaValue{rhs}
						d.slotStoreValue[store] = rhs
						d.opcodeIdToRef[store] = [][2]any{{ref, false}}
						store.LocalStoreFoldedToPostOp = true
						expression := values.NewBinaryExpression(word, values.NewJavaLiteral(1, ref.Type()), operation, ref.Type())
						d.stackValueProducers[expression] = store
						expressions = append(expressions, expression)
						d.opCodes = append(d.opCodes, path...)
						previous = store
					}
					back := &OpCode{Instr: &Instruction{OpCode: OP_GOTO}, CurrentOffset: uint16(offset), Source: []*OpCode{previous}, Target: []*OpCode{first}}
					previous.Target = []*OpCode{back}
					first.Source = append(first.Source, back)
					d.opCodes = append(d.opCodes, back)
					original := d.slotWebs()
					if original.webOf[first] == original.webOf[d.opCodes[1+len(firstPath)]] {
						t.Fatal("fixture needs independently observed intermediate value definitions")
					}
					d.unifyNumericExitWebs()
					canonical := d.opcodeIdToRef[initialize][0][0].(*values.JavaRef)
					for i, expression := range expressions {
						access := values.InspectAccess(expression)
						if len(access.Writes) != 1 || !access.Writes[canonical] {
							t.Fatalf("update%d writes another location after canonical declaration binding", i)
						}
					}
					if d.slotWebs() != original {
						t.Fatal("source projection replaced original reaching-definition cache")
					}
				})
			}
		}
	}
}
