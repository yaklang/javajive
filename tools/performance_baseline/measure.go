package performance_baseline

import (
	"runtime"
	"time"

	"github.com/yaklang/javajive"
	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/decompiler"
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func readMem() (totalAlloc, mallocs uint64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc, m.Mallocs
}

func measureOnce(stage string, classBytes []byte) (Sample, error) {
	var s Sample
	runtime.GC()
	prefix, timed, err := exclusiveFns(stage, classBytes)
	if err != nil {
		s.OK = false
		s.Error = err.Error()
		return s, err
	}
	if prefix != nil {
		if err := prefix(); err != nil {
			s.OK = false
			s.Error = "prefix: " + err.Error()
			return s, err
		}
	}
	a0, m0 := readMem()
	start := time.Now()
	err = timed()
	ns := time.Since(start).Nanoseconds()
	a1, m1 := readMem()
	s.NS = ns
	s.BOp = int64(a1 - a0)
	s.Allocs = int64(m1 - m0)
	s.OK = err == nil
	if err != nil {
		s.Error = err.Error()
	}
	return s, err
}

// exclusiveFns splits untimed prefix (prior stages / parse) from the timed kernel.
// Prefix always builds fresh objects so warm repeats do not reuse a mutated CFG.
func exclusiveFns(stage string, classBytes []byte) (prefix, timed func() error, err error) {
	switch stage {
	case StageParse:
		return nil, func() error {
			_, err := javajive.ParseClass(classBytes)
			return err
		}, nil
	case StageDecode:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var ds []*core.Decompiler
		prefix = func() error {
			ds = ds[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				ds = append(ds, d)
			}
			return nil
		}
		timed = func() error {
			for _, d := range ds {
				if err := d.ParseOpcode(); err != nil {
					return err
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageCFG:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var ds []*core.Decompiler
		prefix = func() error {
			ds = ds[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				if err := d.ParseOpcode(); err != nil {
					return err
				}
				ds = append(ds, d)
			}
			return nil
		}
		timed = func() error {
			for _, d := range ds {
				if err := d.ScanJmp(); err != nil {
					return err
				}
				if err := d.DropUnreachableOpcode(); err != nil {
					return err
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageSemanticCFG:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var ds []*core.Decompiler
		prefix = func() error {
			ds = ds[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				if err := d.ParseOpcode(); err != nil {
					return err
				}
				ds = append(ds, d)
			}
			return nil
		}
		timed = func() error {
			for _, d := range ds {
				if _, err := d.BuildSemanticCFG(); err != nil {
					return err
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageDataflow:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var ds []*core.Decompiler
		prefix = func() error {
			ds = ds[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				if err := d.ParseOpcode(); err != nil {
					return err
				}
				if err := d.ScanJmp(); err != nil {
					return err
				}
				if err := d.DropUnreachableOpcode(); err != nil {
					return err
				}
				ds = append(ds, d)
			}
			return nil
		}
		timed = func() error {
			for _, d := range ds {
				if err := d.CalcOpcodeStackInfo(); err != nil {
					return err
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageT25Sparse:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var graphs []*core.SemanticCFG
		prefix = func() error {
			graphs = graphs[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				if err := d.ParseOpcode(); err != nil {
					return err
				}
				g, err := d.BuildSemanticCFG()
				if err != nil {
					return err
				}
				graphs = append(graphs, g)
			}
			return nil
		}
		timed = func() error {
			for _, g := range graphs {
				if g == nil {
					continue
				}
				sr := core.NewSparseReaching(g)
				for _, n := range g.Nodes {
					if n == nil || n.Instr == nil || !core.LocalAccessOf(n.Instr.OpCode).Write {
						continue
					}
					if _, _, err := sr.Definitions(n, core.GetStoreIdx(n)); err != nil {
						return err
					}
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageT26Cache:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var graphs []*core.SemanticCFG
		prefix = func() error {
			graphs = graphs[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				if err := d.ParseOpcode(); err != nil {
					return err
				}
				g, err := d.BuildSemanticCFG()
				if err != nil {
					return err
				}
				graphs = append(graphs, g)
			}
			return nil
		}
		timed = func() error {
			for _, g := range graphs {
				if g == nil {
					continue
				}
				_ = g.GetOrCompute(core.AnalysisDominators, core.GraphAnalysisDomain{Roots: []int{0}, IncludeException: true})
			}
			return nil
		}
		return prefix, timed, nil
	case StageRegion:
		obj, ms, err := parseObj(classBytes)
		if err != nil {
			return nil, nil, err
		}
		var ds []*core.Decompiler
		prefix = func() error {
			ds = ds[:0]
			for _, m := range ms {
				d, err := buildDecompiler(obj, m)
				if err != nil {
					return err
				}
				ds = append(ds, d)
			}
			return nil
		}
		timed = func() error {
			for _, d := range ds {
				if _, err := decompiler.ParseBytesCode(d); err != nil {
					return err
				}
			}
			return nil
		}
		return prefix, timed, nil
	case StageRender:
		obj, err := javaclassparser.Parse(classBytes)
		if err != nil {
			return nil, nil, err
		}
		return nil, func() error {
			_, err := obj.Dump()
			return err
		}, nil
	case StageE2E:
		return nil, func() error {
			_, err := javajive.DecompileWithOptions(classBytes, javajive.DecompileOptions{Mode: javajive.Precision})
			return err
		}, nil
	default:
		return nil, nil, errUnknownStage(stage)
	}
}

func errUnknownStage(stage string) error {
	return &stageError{stage}
}

type stageError struct{ s string }

func (e *stageError) Error() string { return "unknown stage " + e.s }
