package performance_baseline

import (
	"fmt"

	"github.com/yaklang/javajive"
	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/decompiler"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func utf8At(obj *javaclassparser.ClassObject, idx uint16) string {
	if idx == 0 || int(idx) > len(obj.ConstantPool) {
		return ""
	}
	u, ok := obj.ConstantPool[idx-1].(*javaclassparser.ConstantUtf8Info)
	if !ok {
		return ""
	}
	return u.Value
}

type methodCode struct {
	name, desc string
	access     uint16
	code       *javaclassparser.CodeAttribute
}

func methodsWithCode(obj *javaclassparser.ClassObject) []methodCode {
	var out []methodCode
	for _, m := range obj.Methods {
		for _, attr := range m.Attributes {
			code, ok := attr.(*javaclassparser.CodeAttribute)
			if !ok {
				continue
			}
			out = append(out, methodCode{
				name:   utf8At(obj, m.NameIndex),
				desc:   utf8At(obj, m.DescriptorIndex),
				access: m.AccessFlags,
				code:   code,
			})
		}
	}
	return out
}

func cpValue(obj *javaclassparser.ClassObject, id int) (v values.JavaValue) {
	defer func() {
		if recover() != nil {
			v = javaclassparser.GetLiteralFromCP(obj.ConstantPool, id)
		}
	}()
	return javaclassparser.GetValueFromCP(obj.ConstantPool, id)
}

func buildDecompiler(obj *javaclassparser.ClassObject, m methodCode) (*core.Decompiler, error) {
	parser := core.NewDecompiler(m.code.Code, func(id int) values.JavaValue {
		return cpValue(obj, id)
	})
	parser.ConstantPoolLiteralGetter = func(id int) values.JavaValue {
		return javaclassparser.GetLiteralFromCP(obj.ConstantPool, id)
	}
	mt, err := types.ParseMethodDescriptor(m.desc)
	if err != nil {
		return nil, err
	}
	ft := mt.FunctionType()
	if ft == nil {
		return nil, fmt.Errorf("no function type for %s%s", m.name, m.desc)
	}
	parser.FunctionType = ft
	parser.FunctionContext = &class_context.ClassContext{
		ClassName:    obj.GetClassName(),
		FunctionName: m.name,
		FunctionType: ft,
		IsStatic:     m.access&javaclassparser.StaticFlag != 0,
	}
	for _, entry := range m.code.ExceptionTable {
		parser.ExceptionTable = append(parser.ExceptionTable, &core.ExceptionTableEntry{
			StartPc:   entry.StartPc,
			EndPc:     entry.EndPc,
			HandlerPc: entry.HandlerPc,
			CatchType: entry.CatchType,
		})
	}
	return parser, nil
}

func parseObj(classBytes []byte) (*javaclassparser.ClassObject, []methodCode, error) {
	obj, err := javaclassparser.Parse(classBytes)
	if err != nil {
		return nil, nil, err
	}
	return obj, methodsWithCode(obj), nil
}

func harvestWork(stage string, classBytes []byte) (WorkCounts, error) {
	w := WorkCounts{InputSHA256: sha256Hex(classBytes), PendingT24T25T26: false}
	obj, err := javaclassparser.Parse(classBytes)
	if err != nil {
		w.fillCanonical()
		return w, err
	}
	ms := methodsWithCode(obj)
	w.MethodCount = len(obj.Methods)
	w.CodeMethodCount = len(ms)
	for _, m := range ms {
		w.BytecodeBytes += len(m.code.Code)
		w.ExceptionEntries += len(m.code.ExceptionTable)
	}

	needCFG := stage != StageParse
	if needCFG {
		for _, m := range ms {
			d, err := buildDecompiler(obj, m)
			if err != nil {
				w.fillCanonical()
				return w, err
			}
			if err := d.ParseOpcode(); err != nil {
				w.fillCanonical()
				return w, err
			}
			if stage == StageDecode {
				// opCodes is unexported; ScanJmp is required to walk RootOpCode.Target.
				if err := d.ScanJmp(); err != nil {
					w.fillCanonical()
					return w, err
				}
				acc := countCFG(d)
				w.OpcodeCount += acc.OpcodeCount
				w.StoreCount += acc.StoreCount
				w.LoadCount += acc.LoadCount
				w.IincCount += acc.IincCount
				continue
			}
			if err := d.ScanJmp(); err != nil {
				w.fillCanonical()
				return w, err
			}
			if err := d.DropUnreachableOpcode(); err != nil {
				w.fillCanonical()
				return w, err
			}
			if stage == StageDataflow || stage == StageRegion || stage == StageRender || stage == StageE2E {
				_ = d.CalcOpcodeStackInfo()
			}
			if stage == StageSemanticCFG || stage == StageT25Sparse || stage == StageT26Cache || stage == StageRegion || stage == StageRender || stage == StageE2E {
				g, gerr := d.BuildSemanticCFG()
				if gerr == nil && g != nil {
					w.Nodes += len(g.Nodes)
					w.Edges += len(g.Edges)
					if stage == StageT25Sparse || stage == StageRegion || stage == StageE2E {
						sr := core.NewSparseReaching(g)
						for _, n := range g.Nodes {
							if n == nil || n.Instr == nil {
								continue
							}
							if !core.LocalAccessOf(n.Instr.OpCode).Write {
								continue
							}
							_, _, _ = sr.Definitions(n, core.GetStoreIdx(n))
						}
						w.DefMerge += int(sr.Merges())
					}
					if stage == StageT26Cache || stage == StageE2E {
						_ = g.GetOrCompute(core.AnalysisDominators, core.GraphAnalysisDomain{Roots: []int{0}, IncludeException: true})
						w.RegionRoots += int(g.AnalysisComputeCount())
						_ = g.AnalysisNodesTouched()
					}
				}
			}
			if stage == StageRegion || stage == StageRender || stage == StageE2E {
				pd, err := buildDecompiler(obj, m)
				if err == nil {
					sts, perr := decompiler.ParseBytesCode(pd)
					if perr == nil {
						w.StatementCount += len(sts)
						if pd.RootNode != nil {
							w.RegionRoots++
						}
					}
				}
			}
			acc := countCFG(d)
			w.OpcodeCount += acc.OpcodeCount
			w.CFGNodes += acc.CFGNodes
			w.CFGEdges += acc.CFGEdges
			w.StoreCount += acc.StoreCount
			w.LoadCount += acc.LoadCount
			w.IincCount += acc.IincCount
			w.HandlerPCs += acc.HandlerPCs
			w.DefMergeLoads += acc.DefMergeLoads
			w.DefStoreLoadPairs += acc.DefStoreLoadPairs
		}
	}

	switch stage {
	case StageParse:
		w.Notes = "javajive.ParseClass. Timed region is parse only."
	case StageDecode:
		w.Notes = "Timed: ParseOpcode. Opcode counts harvested via sibling ScanJmp because opCodes is unexported."
	case StageCFG:
		w.Notes = "Timed: ScanJmp+DropUnreachableOpcode after ParseOpcode. Stage id cfg_opcode_only. Production SemanticCFG is timed separately as semantic_cfg_build."
	case StageSemanticCFG:
		w.Notes = "Timed: (*core.Decompiler).BuildSemanticCFG. nodes/edges from production SemanticCFG."
	case StageDataflow:
		w.Notes = "Timed: CalcOpcodeStackInfo only. Stage id dataflow_stacksim_opcode_only."
	case StageT25Sparse:
		w.Notes = "Timed: NewSparseReaching + Definitions. def_merge is sparse Merges on production CFG."
	case StageT26Cache:
		w.Notes = "Timed: SemanticCFG.GetOrCompute(AnalysisDominators). Per-request cache counters."
	case StageRegion:
		w.Notes = "Timed: decompiler.ParseBytesCode (ParseStatement+rewriter). Stage id region_and_statements_combined. region_roots counts statement RootNode values, not SemanticCFG regions."
	case StageRender:
		src, err := obj.Dump()
		if err != nil {
			w.fillCanonical()
			return w, err
		}
		w.OutputBytes = len(src)
		w.OutputSHA256 = sha256Hex([]byte(src))
		w.Notes = "Timed: ClassObject.Dump after Parse. Stage id render_class_dump_reruns_decompile. Dump re-runs method decompile+render (combined). Dump() leaves Mode empty so compatibility source rewrites may run; e2e_kernel uses Precision via DecompileWithOptions and is not the same path."
	case StageE2E:
		res, err := javajive.DecompileWithOptions(classBytes, javajive.DecompileOptions{Mode: javajive.Precision})
		if err != nil {
			w.DecompileStatus = "error"
			w.fillCanonical()
			return w, err
		}
		w.OutputBytes = len(res.Source)
		w.OutputSHA256 = sha256Hex([]byte(res.Source))
		w.DecompileStatus = res.Status
		w.StubMethodCount = len(res.StubMethods)
		w.StubMethods = append([]string(nil), res.StubMethods...)
		w.InputSHA256 = res.InputHash
		w.Notes = "Timed: javajive.DecompileWithOptions Precision. Kernel Parse+Dump. javac/java never added. nodes/edges from production SemanticCFG; def_merge from sparse Merges; region_roots includes T26 compute count."
	}
	w.fillCanonical()
	return w, nil
}

func cacheKey(mode, stage, inputSHA string, reuseParsed bool) string {
	process := "shared"
	if mode == "cold" {
		process = "fresh"
	}
	return fmt.Sprintf("process=%s,input=%s,stage=%s,reuse_classobject=%t,reuse_dumper=false", process, inputSHA, stage, reuseParsed)
}
