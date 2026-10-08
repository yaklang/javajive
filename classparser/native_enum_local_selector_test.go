package javaclassparser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Independent oracle: propagate explicit last-writer labels forward over the
// finite graph, retaining the input label on exception edges. Production walks
// backward to a frontier after a separate reachability pass. These abstract
// access graphs test reaching definitions, not JVM operand-stack validity.
func TestNativeEnumLocalSelectorReachingStoreBoundedModel(t *testing.T) {
	type write struct{ opcode, slot, width int }
	writes := []write{{core.OP_ASTORE, 2, 1}, {core.OP_ASTORE, 3, 1}, {core.OP_LSTORE, 1, 2}, {core.OP_LSTORE, 3, 2}, {core.OP_IINC, 2, 1}}
	type edge struct {
		from, to  int
		exception bool
	}
	type state struct{ node, last int }
	count, positive := 0, 0
	digest := sha256.New()
	for n := 1; n <= 3; n++ {
		for mask := 0; mask < 1<<(n*n); mask++ {
			pairs := []edge{}
			for from := 0; from < n; from++ {
				for to := 0; to < n; to++ {
					if mask&(1<<(from*n+to)) != 0 {
						pairs = append(pairs, edge{from: from, to: to})
					}
				}
			}
			variants := 1
			if len(pairs) > 0 {
				variants = 2
			}
			if len(pairs) > 1 {
				variants = 3
			}
			for variant := 0; variant < variants; variant++ {
				edges := append([]edge(nil), pairs...)
				for i := range edges {
					edges[i].exception = variant == 1 && i == 0 || variant == 2
				}
				for readIndex := 0; readIndex < n; readIndex++ {
					for first := -1; first < n; first++ {
						if first == readIndex {
							continue
						}
						for second := -1; second < n; second++ {
							if second == readIndex || second >= 0 && second == first {
								continue
							}
							for k, other := range writes {
								ops := make([]*core.OpCode, n)
								accesses := map[int]write{}
								for i := range ops {
									ops[i] = &core.OpCode{CurrentOffset: uint16(i), Instr: &core.Instruction{OpCode: core.OP_NOP}}
								}
								ops[readIndex].Instr.OpCode = core.OP_ALOAD
								ops[readIndex].Data = []byte{2}
								if first >= 0 {
									accesses[first] = write{core.OP_ASTORE, 2, 1}
								}
								if second >= 0 {
									accesses[second] = other
								}
								for i, w := range accesses {
									ops[i].Instr.OpCode = w.opcode
									ops[i].Data = []byte{byte(w.slot)}
									if w.opcode == core.OP_IINC {
										ops[i].Data = append(ops[i].Data, 1)
									}
								}
								flow := &nativeEnumParameterFlow{entry: ops[0], byPC: map[int]*core.OpCode{}, edges: map[*core.OpCode][]core.SemanticEdge{}}
								for i, op := range ops {
									flow.byPC[i] = op
								}
								for _, e := range edges {
									kind := core.EdgeTaken
									if e.exception {
										kind = core.EdgeException
									}
									flow.edges[ops[e.from]] = append(flow.edges[ops[e.from]], core.SemanticEdge{From: ops[e.from], To: ops[e.to], Kind: kind})
								}
								frontier := map[int]bool{}
								pending := []state{{0, -1}}
								seen := map[state]bool{{0, -1}: true}
								for len(pending) > 0 {
									current := pending[len(pending)-1]
									pending = pending[:len(pending)-1]
									if current.node == readIndex {
										frontier[current.last] = true
									}
									after := current.last
									if w, ok := accesses[current.node]; ok && w.slot < 3 && 2 < w.slot+w.width {
										after = current.node
									}
									for _, e := range edges {
										if e.from != current.node {
											continue
										}
										last := after
										if e.exception {
											last = current.last
										}
										next := state{e.to, last}
										if !seen[next] {
											seen[next] = true
											pending = append(pending, next)
										}
									}
								}
								want := len(frontier) == 1 && !frontier[-1]
								wantPC := -1
								if want {
									for pc := range frontier {
										wantPC = pc
									}
								}
								read := &nativeEnumSelectorProducer{pc: readIndex, opcode: core.OP_ALOAD, slot: 2, result: "Ljava/lang/Object;"}
								pc, got := flow.reachingStore(read, nil)
								if got != want || got && pc != wantPC {
									t.Fatalf("n=%d graph=%d exception=%d read=%d first=%d second=%d/%+v result=(%d,%v) want=(%d,%v)", n, mask, variant, readIndex, first, second, other, pc, got, wantPC, want)
								}
								if got {
									positive++
								}
								count++
								fmt.Fprintf(digest, "%d/%d/%d/%d/%d/%d/%d/%v/%d\n", n, mask, variant, readIndex, first, second, k, want, wantPC)
							}
						}
					}
				}
			}
		}
	}
	if count != 161400 || positive == 0 || positive == count {
		t.Fatalf("model inventory count=%d positive=%d", count, positive)
	}
	t.Logf("complete bounded reaching-definition grammar cases=%d positive=%d sha256=%x", count, positive, digest.Sum(nil))
}

func TestNativeEnumLocalSelectorSourceRequiresExactDecodedDeclaration(t *testing.T) {
	for _, variant := range []string{"original", "renamed identifier", "no declaration", "wrong store pc", "wrong store slot", "same type replaced seed", "parameter relabeled local", "custom value", "stack alias", "wrong read pc", "wrong read opcode", "wrong read slot", "wrong original type", "binding cast", "runtime cast", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaClass("LocalBox")
			seed := values.NewJavaLiteral(nil, typ)
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), seed, typ)
			pc, slot := 11, 3
			if variant == "wrong store pc" {
				pc++
			}
			if variant == "wrong store slot" {
				slot++
			}
			if variant != "no declaration" {
				ref.MarkOriginalLocalDeclaration(pc, slot, seed)
			}
			original := &nativeEnumSelectorProducer{opcode: core.OP_ALOAD, pc: 17, slot: 3, result: "LLocalBox;", local: &nativeEnumLocalRead{pc: 17, opcode: core.OP_ALOAD, slot: 3, storePC: 11, descriptor: "LLocalBox;"}}
			var source values.JavaValue = ref
			var work *workbudget.Budget
			switch variant {
			case "renamed identifier":
				ref.Id = coreutils.NewRootVariableId()
			case "same type replaced seed":
				ref.Val = values.NewJavaLiteral(nil, typ)
			case "parameter relabeled local":
				ref.IsParam = true
			case "custom value":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "other" }, func() types.JavaType { return typ })
			case "stack alias":
				ref.StackVar = seed
			case "wrong read pc":
				original.local.pc++
			case "wrong read opcode":
				original.local.opcode = core.OP_ILOAD
			case "wrong read slot":
				original.local.slot++
			case "wrong original type":
				original.local.descriptor = "LOther;"
			case "binding cast":
				source = &values.CastExpression{Binding: true, TargetType: typ, Value: ref}
			case "runtime cast":
				source = &values.CastExpression{TargetType: typ, Value: ref}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("consume the sole available proof step")
				}
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original" || variant == "renamed identifier" || variant == "binding cast"
			if got := nativeEnumSelectorSource(original, source, &class_context.ClassContext{}, work, 0); got != want {
				t.Fatalf("original local source=%v", got)
			}
		})
	}
}

func TestNativeEnumLocalSelectorRequiresOriginalMethodCodeAndTypedFrames(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class TypedLocalProof {static Object copy(Object input){Object local=input;return local;}}`, "none")
	for _, variant := range []string{"original", "wide local", "nil reader", "nil object", "nil method", "nil code", "nil flow", "foreign flow body", "foreign method pointer", "missing method", "duplicate method", "foreign code pointer", "missing code", "duplicate code", "wrong parameter type", "missing parameter", "extra parameter", "bad descriptor", "small locals", "small stack", "oversize frames", "invalid handler", "uninitialized local", "primitive local", "null local", "two reaching stores", "same origin two stores", "phi value", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["TypedLocalProof.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, candidate := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, candidate.NameIndex)
				if name == "copy" {
					method = candidate
					for _, attribute := range method.Attributes {
						if body, ok := attribute.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("actual original copy method")
			}
			reader := NewClassObjectDumper(obj)
			params := map[int]string{0: "Ljava/lang/Object;"}
			// Straight-line typed controls use the actual parsed declaration and
			// descriptor. Malformed/abstract controls never run on a JVM.
			switch variant {
			case "wide local":
				code.Code = []byte{core.OP_ALOAD_0, core.OP_WIDE, core.OP_ASTORE, 1, 3, core.OP_WIDE, core.OP_ALOAD, 1, 3, core.OP_ARETURN}
				code.MaxLocals = 260
			case "uninitialized local":
				code.Code = []byte{core.OP_ALOAD_1, core.OP_ARETURN}
			case "primitive local":
				code.Code = []byte{core.OP_ICONST_1, core.OP_ISTORE_1, core.OP_ILOAD_1, core.OP_POP, core.OP_ALOAD_0, core.OP_ARETURN}
			case "null local":
				code.Code = []byte{core.OP_ACONST_NULL, core.OP_ASTORE_1, core.OP_ALOAD_1, core.OP_ARETURN}
			case "two reaching stores", "same origin two stores", "phi value":
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).SetString("(Ljava/lang/Object;Ljava/lang/Object;Z)Ljava/lang/Object;")
				params = map[int]string{0: "Ljava/lang/Object;", 1: "Ljava/lang/Object;", 2: "Z"}
				code.MaxLocals, code.MaxStack = 4, 1
				code.Code = []byte{core.OP_ILOAD_2, core.OP_IFEQ, 0, 8, core.OP_ALOAD_0, core.OP_ASTORE_3, core.OP_GOTO, 0, 5, core.OP_ALOAD_1, core.OP_ASTORE_3, core.OP_ALOAD_3, core.OP_ARETURN}
				if variant == "same origin two stores" {
					code.Code[9] = core.OP_ALOAD_0
				}
				if variant == "phi value" {
					code.Code = []byte{core.OP_ILOAD_2, core.OP_IFEQ, 0, 7, core.OP_ALOAD_0, core.OP_GOTO, 0, 4, core.OP_ALOAD_1, core.OP_ASTORE_3, core.OP_ALOAD_3, core.OP_ARETURN}
				}
			case "small locals":
				code.MaxLocals = 1
			case "small stack":
				code.MaxStack = 0
			case "oversize frames":
				code.MaxLocals, code.MaxStack = 65535, 65535
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			flow := nativeEnumParameterOriginalFlow(decoder, code, nil)
			if flow == nil {
				t.Fatal("typed-frame control must reach a constructed original flow")
			}
			switch variant {
			case "nil reader":
				reader = nil
			case "nil object":
				reader.obj = nil
			case "nil method":
				method = nil
			case "nil code":
				code = nil
			case "nil flow":
				flow = nil
			case "foreign flow body":
				copy := *code
				flow.code = &copy
			case "foreign method pointer":
				copy := *method
				method = &copy
			case "missing method":
				obj.Methods = nil
			case "duplicate method":
				obj.Methods = append(obj.Methods, method)
			case "foreign code pointer":
				copy := *code
				code = &copy
				flow.code = code // still not owned by the original method
			case "missing code":
				method.Attributes = nil
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "wrong parameter type":
				params[0] = "Ljava/lang/String;"
			case "missing parameter":
				delete(params, 0)
			case "extra parameter":
				params[1] = "Ljava/lang/Object;"
			case "bad descriptor":
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).SetString("(Ljava/lang/Object;)invalid")
			case "invalid handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code) + 1), HandlerPc: 0}}
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			facts, known := reader.nativeEnumLocalSelectorReads(method, code, flow, params)
			want := variant == "original" || variant == "wide local"
			if got := known && len(facts) == 1; got != want || !want && len(facts) != 0 {
				t.Fatalf("original typed local certificate=%v known=%v facts=%+v", got, known, facts)
			}
			if want {
				for _, fact := range facts {
					wantPC, wantStore, wantSlot := 2, 1, 1
					if variant == "wide local" {
						wantPC, wantSlot = 5, 259
					}
					if fact.pc != wantPC || fact.storePC != wantStore || fact.slot != wantSlot || fact.descriptor != "Ljava/lang/Object;" {
						t.Fatalf("actual LOAD/STORE identity %+v", fact)
					}
				}
			}
		})
	}
}
