// Command generate builds the irreducible-CFG classfile corpus.
//
// Run from any directory with:
//
//	go run ./test/cross/testdata/irreducible/generate
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	cf "github.com/yaklang/javajive/classparser/classfilebuilder"
)

type vector struct {
	Entry  int `json:"entry"`
	Budget int `json:"budget"`
}

type caseSpec struct {
	Name         string
	Kind         string
	GraphEntries []string
	Description  string
	Build        func(*cf.Class) error
	Evaluate     func(int, int) int
}

type manifest struct {
	SchemaVersion  int            `json:"schemaVersion"`
	ClassfileMajor int            `json:"classfileMajor"`
	Generator      string         `json:"generator"`
	Vectors        []vector       `json:"vectors"`
	Cases          []manifestCase `json:"cases"`
}

type manifestCase struct {
	Name             string      `json:"name"`
	Kind             string      `json:"kind"`
	GraphEntries     []string    `json:"graphEntries"`
	Description      string      `json:"description"`
	ClassFile        string      `json:"classFile"`
	EquivalentSource string      `json:"equivalentSource"`
	BytecodeDoc      string      `json:"bytecodeDoc"`
	ControlFlow      controlFlow `json:"controlFlow"`
	ExpectedOutput   []int       `json:"expectedOutput"`
	Size             int         `json:"size"`
	SHA256           string      `json:"sha256"`
}

type controlFlow struct {
	Blocks       []cfgBlock `json:"blocks"`
	Edges        []cfgEdge  `json:"edges"`
	CyclicSCC    []string   `json:"cyclicScc"`
	EntryTargets []string   `json:"entryTargets"`
	Reducibility string     `json:"reducibility"`
}

type cfgBlock struct {
	Name string `json:"name"`
	PC   int    `json:"pc"`
}

type cfgEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

var vectors = []vector{
	{Entry: 0, Budget: 0},
	{Entry: 0, Budget: 1},
	{Entry: 1, Budget: 2},
	{Entry: 2, Budget: 3},
	{Entry: 3, Budget: 5},
	{Entry: -1, Budget: 8},
}

func main() {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		fatalf("locate generator source")
	}
	root := filepath.Dir(filepath.Dir(source))
	for _, dir := range []string{"original", "equivalent", "bytecode"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			fatalf("create %s: %v", dir, err)
		}
	}

	m := manifest{
		SchemaVersion:  1,
		ClassfileMajor: 49,
		Generator:      "generate/main.go",
		Vectors:        vectors,
	}
	generated := make(map[string][]byte)
	for _, spec := range cases() {
		class := cf.NewClass(spec.Name, "java/lang/Object", cf.ACC_PUBLIC|cf.ACC_SUPER)
		if err := addConstructor(class); err != nil {
			fatalf("%s constructor: %v", spec.Name, err)
		}
		if err := spec.Build(class); err != nil {
			fatalf("%s run: %v", spec.Name, err)
		}
		if err := addMain(class, spec.Name); err != nil {
			fatalf("%s main: %v", spec.Name, err)
		}
		raw, err := class.Bytes()
		if err != nil {
			fatalf("assemble %s: %v", spec.Name, err)
		}
		name := spec.Name + ".class"
		generated["original/"+name] = raw
		generated["bytecode/"+spec.Name+".md"] = bytecodeDoc(spec)
		digest := sha256.Sum256(raw)
		expected := make([]int, len(vectors))
		for i, v := range vectors {
			expected[i] = spec.Evaluate(v.Entry, v.Budget)
		}
		m.Cases = append(m.Cases, manifestCase{
			Name: spec.Name, Kind: spec.Kind, GraphEntries: spec.GraphEntries,
			Description: spec.Description, ClassFile: "original/" + name,
			EquivalentSource: "equivalent/" + spec.Name + ".java",
			BytecodeDoc:      "bytecode/" + spec.Name + ".md",
			ControlFlow:      controlFlowFor(spec.Name),
			ExpectedOutput:   expected, Size: len(raw), SHA256: hex.EncodeToString(digest[:]),
		})
	}

	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fatalf("encode manifest: %v", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	generated["manifest.json"] = manifestBytes
	for _, spec := range cases() {
		name := "equivalent/" + spec.Name + ".java"
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			fatalf("read %s: %v", name, err)
		}
		generated[name] = data
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		fatalf("read README.md: %v", err)
	}
	generated["README.md"] = readme

	for name, data := range generated {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			fatalf("write %s: %v", name, err)
		}
	}
	names := make([]string, 0, len(generated))
	for name := range generated {
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		digest := sha256.Sum256(generated[name])
		fmt.Fprintf(&sums, "%x  %s\n", digest, name)
	}
	if err := os.WriteFile(filepath.Join(root, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		fatalf("write SHA256SUMS: %v", err)
	}
}

func bytecodeDoc(spec caseSpec) []byte {
	cfg := controlFlowFor(spec.Name)
	var blocks, edges strings.Builder
	for _, block := range cfg.Blocks {
		fmt.Fprintf(&blocks, "- `%s`: pc %d\n", block.Name, block.PC)
	}
	for _, edge := range cfg.Edges {
		fmt.Fprintf(&edges, "- `%s -> %s` (%s)\n", edge.From, edge.To, edge.Kind)
	}
	return []byte(fmt.Sprintf(`# %s

Classification: **%s**

%s Every reachable cycle consumes budget, and all control-flow edges cross stack-empty block boundaries.

## Control-flow graph

`+"```mermaid"+`
%s
`+"```"+`

## Exact blocks

%s
## Typed edges

%s
Cyclic SCC: `+"`%s`"+`

SCC entry targets: `+"`%s`"+`

## Symbolic bytecode

`+"```text"+`
%s
`+"```"+`
`, spec.Name, cfg.Reducibility, spec.Description, mermaidFor(spec.Name), blocks.String(), edges.String(), strings.Join(cfg.CyclicSCC, ", "), strings.Join(cfg.EntryTargets, ", "), symbolicFor(spec.Name)))
}

func controlFlowFor(name string) controlFlow {
	block := func(name string, pc int) cfgBlock { return cfgBlock{Name: name, PC: pc} }
	edge := func(from, to, kind string) cfgEdge { return cfgEdge{From: from, To: to, Kind: kind} }
	switch name {
	case "NaturalSingleEntry":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("H", 4), block("body", 8), block("exit", 18)},
			[]cfgEdge{edge("entry", "H", "fallthrough"), edge("H", "exit", "ifle/taken"), edge("H", "body", "ifle/fallthrough"), edge("body", "H", "goto/backedge")},
			[]string{"H", "body"}, []string{"H"}, "reducible",
		}
	case "NaturalMultipleLatch":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("H", 4), block("dispatch", 8), block("oddLatch", 14), block("evenLatch", 24), block("exit", 34)},
			[]cfgEdge{edge("entry", "H", "fallthrough"), edge("H", "exit", "ifle/taken"), edge("H", "dispatch", "ifle/fallthrough"), edge("dispatch", "evenLatch", "ifeq/taken"), edge("dispatch", "oddLatch", "ifeq/fallthrough"), edge("oddLatch", "H", "goto/backedge"), edge("evenLatch", "H", "goto/backedge")},
			[]string{"H", "dispatch", "oddLatch", "evenLatch"}, []string{"H"}, "reducible",
		}
	case "DeadSecondaryEntry":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("toA", 8), block("dead", 11), block("A", 19), block("AtoB", 30), block("B", 33), block("BtoA", 44), block("exit", 47)},
			[]cfgEdge{edge("entry", "exit", "ifle/taken"), edge("entry", "toA", "ifle/fallthrough"), edge("toA", "A", "goto"), edge("dead", "B", "goto/unreachable"), edge("A", "exit", "ifle/taken"), edge("A", "AtoB", "ifle/fallthrough"), edge("AtoB", "B", "goto"), edge("B", "exit", "ifle/taken"), edge("B", "BtoA", "ifle/fallthrough"), edge("BtoA", "A", "goto/backedge")},
			[]string{"A", "AtoB", "B", "BtoA"}, []string{"A"}, "reachable-reducible (dead syntactic entry to B)",
		}
	case "IrreducibleTwoEntry":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("select", 8), block("toB", 12), block("A", 15), block("AtoB", 26), block("B", 29), block("BtoA", 40), block("exit", 43)},
			[]cfgEdge{edge("entry", "exit", "ifle/taken"), edge("entry", "select", "ifle/fallthrough"), edge("select", "A", "ifeq/taken"), edge("select", "toB", "ifeq/fallthrough"), edge("toB", "B", "goto/entry"), edge("A", "exit", "ifle/taken"), edge("A", "AtoB", "ifle/fallthrough"), edge("AtoB", "B", "goto"), edge("B", "exit", "ifle/taken"), edge("B", "BtoA", "ifle/fallthrough"), edge("BtoA", "A", "goto/backedge")},
			[]string{"A", "AtoB", "B", "BtoA"}, []string{"A", "B"}, "irreducible",
		}
	case "IrreducibleMidBodyPhi":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("select", 11), block("toM", 15), block("H", 18), block("M", 26), block("MtoH", 38), block("exit", 41)},
			[]cfgEdge{edge("entry", "exit", "ifle/taken"), edge("entry", "select", "ifle/fallthrough"), edge("select", "H", "ifeq/taken"), edge("select", "toM", "ifeq/fallthrough"), edge("toM", "M", "goto/entry"), edge("H", "M", "goto"), edge("M", "exit", "ifle/taken"), edge("M", "MtoH", "ifle/fallthrough"), edge("MtoH", "H", "goto/backedge")},
			[]string{"H", "M", "MtoH"}, []string{"H", "M"}, "irreducible",
		}
	case "IrreducibleSwitchThreeEntry":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("switch", 8), block("A", 36), block("AtoB", 47), block("B", 50), block("BtoC", 62), block("C", 65), block("CtoA", 77), block("exit", 80)},
			[]cfgEdge{edge("entry", "exit", "ifle/taken"), edge("entry", "switch", "ifle/fallthrough"), edge("switch", "A", "tableswitch case 0/default"), edge("switch", "B", "tableswitch case 1"), edge("switch", "C", "tableswitch case 2"), edge("A", "exit", "ifle/taken"), edge("A", "AtoB", "ifle/fallthrough"), edge("AtoB", "B", "goto"), edge("B", "exit", "ifle/taken"), edge("B", "BtoC", "ifle/fallthrough"), edge("BtoC", "C", "goto"), edge("C", "exit", "ifle/taken"), edge("C", "CtoA", "ifle/fallthrough"), edge("CtoA", "A", "goto/backedge")},
			[]string{"A", "AtoB", "B", "BtoC", "C", "CtoA"}, []string{"A", "B", "C"}, "irreducible",
		}
	case "IrreducibleMultiExitEffects":
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("select", 12), block("toB", 16), block("A", 19), block("Atest", 38), block("AtoB", 45), block("B", 48), block("Btest", 68), block("BtoA", 74), block("exitA", 77), block("exitB", 87), block("budgetExit", 97)},
			[]cfgEdge{edge("entry", "budgetExit", "ifle/taken"), edge("entry", "select", "ifle/fallthrough"), edge("select", "A", "ifeq/taken"), edge("select", "toB", "ifeq/fallthrough"), edge("toB", "B", "goto/entry"), edge("A", "budgetExit", "ifle/taken"), edge("A", "Atest", "ifle/fallthrough"), edge("Atest", "exitA", "ifeq/taken"), edge("Atest", "AtoB", "ifeq/fallthrough"), edge("AtoB", "B", "goto"), edge("B", "budgetExit", "ifle/taken"), edge("B", "Btest", "ifle/fallthrough"), edge("Btest", "exitB", "ifeq/taken"), edge("Btest", "BtoA", "ifeq/fallthrough"), edge("BtoA", "A", "goto/backedge")},
			[]string{"A", "Atest", "AtoB", "B", "Btest", "BtoA"}, []string{"A", "B"}, "irreducible",
		}
	default:
		return controlFlow{
			[]cfgBlock{block("entry", 0), block("toH", 8), block("H", 11), block("HtoB", 17), block("A", 20), block("Adispatch", 31), block("AtoB", 37), block("B", 40), block("Bdispatch", 51), block("BtoH", 57), block("exit", 60)},
			[]cfgEdge{edge("entry", "exit", "ifle/taken"), edge("entry", "toH", "ifle/fallthrough"), edge("toH", "H", "goto"), edge("H", "A", "ifeq/taken"), edge("H", "HtoB", "ifeq/fallthrough"), edge("HtoB", "B", "goto"), edge("A", "exit", "ifle/taken"), edge("A", "Adispatch", "ifle/fallthrough"), edge("Adispatch", "H", "ifeq/taken"), edge("Adispatch", "AtoB", "ifeq/fallthrough"), edge("AtoB", "B", "goto"), edge("B", "exit", "ifle/taken"), edge("B", "Bdispatch", "ifle/fallthrough"), edge("Bdispatch", "A", "ifeq/taken"), edge("Bdispatch", "BtoH", "ifeq/fallthrough"), edge("BtoH", "H", "goto/backedge")},
			[]string{"H", "HtoB", "A", "Adispatch", "AtoB", "B", "Bdispatch", "BtoH"}, []string{"H"}, "regionally-irreducible (whole SCC has outer header H; cyclic subregion A/B has entry targets A and B)",
		}
	}
}

func mermaidFor(name string) string {
	switch name {
	case "NaturalSingleEntry":
		return "flowchart TD\n  E[entry pc 0] --> H[H pc 4]\n  H -->|budget > 0| H\n  H --> X[exit pc 18]"
	case "NaturalMultipleLatch":
		return "flowchart TD\n  E[entry pc 0] --> H[H pc 4]\n  H --> O[odd latch pc 14]\n  H --> V[even latch pc 24]\n  O --> H\n  V --> H\n  H --> X[exit pc 34]"
	case "DeadSecondaryEntry":
		return "flowchart TD\n  E[entry pc 0] --> A[A pc 19]\n  D[dead D pc 11] -. unreachable .-> B[B pc 33]\n  A --> B\n  B --> A\n  A --> X[exit pc 47]\n  B --> X"
	case "IrreducibleTwoEntry":
		return "flowchart TD\n  E[entry pc 0] --> A[A pc 15]\n  E --> B[B pc 29]\n  A --> B\n  B --> A\n  A --> X[exit pc 43]\n  B --> X"
	case "IrreducibleMidBodyPhi":
		return "flowchart TD\n  E[entry pc 0] --> H[H pc 18]\n  E --> M[M pc 26]\n  H --> M\n  M --> H\n  M --> X[exit pc 41]"
	case "IrreducibleSwitchThreeEntry":
		return "flowchart TD\n  E[switch block pc 8] --> A[A pc 36]\n  E --> B[B pc 50]\n  E --> C[C pc 65]\n  A --> B\n  B --> C\n  C --> A\n  A --> X[exit pc 80]\n  B --> X\n  C --> X"
	case "IrreducibleMultiExitEffects":
		return "flowchart TD\n  E[entry pc 0] --> A[A pc 19]\n  E --> B[B pc 48]\n  A --> B\n  B --> A\n  A --> XA[exit A pc 77]\n  B --> XB[exit B pc 87]\n  A --> BX[budget exit pc 97]\n  B --> BX"
	default:
		return "flowchart TD\n  E[entry pc 0] --> H[outer H pc 11]\n  H --> A[A pc 20]\n  H --> B[B pc 40]\n  A --> B\n  B --> A\n  A --> H\n  B --> H\n  A --> X[exit pc 60]\n  B --> X"
	}
}

func symbolicFor(name string) string {
	switch name {
	case "NaturalSingleEntry":
		return "entry: x=budget; acc=entry\nH: if x<=0 goto exit; acc+=x; x--; goto H\nexit: return acc"
	case "NaturalMultipleLatch":
		return "entry: x=budget; acc=entry\nH: if x<=0 goto exit; if (x&1)==0 goto even\nodd: acc+=3; x--; goto H\neven: acc+=2; x--; goto H\nexit: return acc"
	case "DeadSecondaryEntry":
		return "entry: x=budget; acc=entry; if x<=0 goto exit; goto A\ndead D: acc+=99; goto B\nA: acc+=2; x--; if x<=0 goto exit; goto B\nB: acc+=3; x--; if x<=0 goto exit; goto A\nexit: return acc"
	case "IrreducibleTwoEntry":
		return "entry: x=budget; acc=entry; if x<=0 exit; if entry==0 goto A else B\nA: acc+=3; x--; if x<=0 exit; goto B\nB: acc+=5; x--; if x<=0 exit; goto A\nexit: return acc"
	case "IrreducibleMidBodyPhi":
		return "entry: x=budget; acc=entry; value=entry; if entry==0 goto H else M\nH: value=acc+2; goto M\nM: acc+=value; x--; if x<=0 exit; goto H\nexit: return acc"
	case "IrreducibleSwitchThreeEntry":
		return "entry: x=budget; acc=entry; tableswitch entry {0:A,1:B,2:C,default:A}\nA: acc+=1; x--; if x<=0 exit; goto B\nB: acc+=10; x--; if x<=0 exit; goto C\nC: acc+=100; x--; if x<=0 exit; goto A\nexit: return acc"
	case "IrreducibleMultiExitEffects":
		return "entry: effects=0; x=budget; acc=entry; choose A/B\nA: effects+=1; acc+=2; x--; budget/early exit or B\nB: effects+=10; acc+=3; x--; budget/early exit or A\nexitA/exitB/budgetExit: return acc plus selected effects"
	default:
		return "entry: x=budget; acc=entry; goto H\nH: choose A/B by acc parity\nA: acc+=2; x--; exit, H, or B\nB: acc+=5; x--; exit, A, or H\nexit: return acc"
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "generate irreducible corpus: "+format+"\n", args...)
	os.Exit(1)
}

func addConstructor(class *cf.Class) error {
	initRef, err := class.Pool().Methodref("java/lang/Object", "<init>", "()V")
	if err != nil {
		return err
	}
	class.AddMethod(cf.ACC_PUBLIC, "<init>", "()V").Code().
		Load(cf.RefLocal, 0).CP(cf.INVOKESPECIAL, initRef).Emit(cf.RETURN)
	return nil
}

func addMain(class *cf.Class, name string) error {
	out, err := class.Pool().Fieldref("java/lang/System", "out", "Ljava/io/PrintStream;")
	if err != nil {
		return err
	}
	println, err := class.Pool().Methodref("java/io/PrintStream", "println", "(I)V")
	if err != nil {
		return err
	}
	run, err := class.Pool().Methodref(name, "run", "(II)I")
	if err != nil {
		return err
	}
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "main", "([Ljava/lang/String;)V").Code()
	for _, v := range vectors {
		code.CP(cf.GETSTATIC, out)
		pushInt(class, code, v.Entry)
		pushInt(class, code, v.Budget)
		code.CP(cf.INVOKESTATIC, run).CP(cf.INVOKEVIRTUAL, println)
	}
	code.Emit(cf.RETURN)
	return nil
}

func pushInt(class *cf.Class, code *cf.Code, value int) {
	switch value {
	case -1:
		code.Emit(cf.ICONST_M1)
	case 0:
		code.Emit(cf.ICONST_0)
	case 1:
		code.Emit(cf.ICONST_1)
	case 2:
		code.Emit(cf.ICONST_2)
	case 3:
		code.Emit(cf.ICONST_3)
	case 4:
		code.Emit(cf.ICONST_4)
	case 5:
		code.Emit(cf.ICONST_5)
	default:
		if value >= -128 && value <= 127 {
			code.Byte(cf.BIPUSH, value)
		} else if value >= -32768 && value <= 32767 {
			code.Short(cf.SIPUSH, value)
		} else {
			index, err := class.Pool().Integer(int32(value))
			if err != nil {
				fatalf("integer constant %d: %v", value, err)
			}
			code.LDC(index)
		}
	}
}

func initialize(class *cf.Class, code *cf.Code) {
	code.Load(cf.IntLocal, 1).Store(cf.IntLocal, 2)
	code.Load(cf.IntLocal, 0).Store(cf.IntLocal, 3)
}

func addConst(class *cf.Class, code *cf.Code, local uint16, value int) {
	code.Load(cf.IntLocal, local)
	pushInt(class, code, value)
	code.Emit(cf.IADD).Store(cf.IntLocal, local)
}

func returnLocal(code *cf.Code, local uint16) {
	code.Load(cf.IntLocal, local).Emit(cf.IRETURN)
}

func cases() []caseSpec {
	return []caseSpec{
		{"NaturalSingleEntry", "reducible", []string{"H"}, "One natural loop with one header and one latch.", buildNaturalSingleEntry, evalNaturalSingleEntry},
		{"NaturalMultipleLatch", "reducible", []string{"H"}, "One natural loop with two distinct latch blocks.", buildNaturalMultipleLatch, evalNaturalMultipleLatch},
		{"DeadSecondaryEntry", "reachable-reducible", []string{"A", "dead D -> B"}, "A dead block supplies a syntactic secondary edge into an otherwise natural A/B loop.", buildDeadSecondaryEntry, evalDeadSecondaryEntry},
		{"IrreducibleTwoEntry", "irreducible", []string{"A", "B"}, "A two-node strongly connected component entered independently at both nodes.", buildIrreducibleTwoEntry, evalIrreducibleTwoEntry},
		{"IrreducibleMidBodyPhi", "irreducible", []string{"H", "M"}, "A secondary entry reaches the loop body at a local-value merge.", buildIrreducibleMidBodyPhi, evalIrreducibleMidBodyPhi},
		{"IrreducibleSwitchThreeEntry", "irreducible", []string{"A", "B", "C"}, "A tableswitch enters each node of a three-node cycle.", buildIrreducibleSwitchThreeEntry, evalIrreducibleSwitchThreeEntry},
		{"IrreducibleMultiExitEffects", "irreducible", []string{"A", "B"}, "A two-entry cycle has distinct effectful early exits and a budget exit.", buildIrreducibleMultiExitEffects, evalIrreducibleMultiExitEffects},
		{"RegionalIrreducibility", "regionally-irreducible", []string{"H -> A", "H -> B"}, "A natural outer loop contains an A/B subregion with two entries.", buildRegionalIrreducibility, evalRegionalIrreducibility},
	}
}

func buildNaturalSingleEntry(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	header, exit := code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Mark(header).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 2).Emit(cf.IADD).Store(cf.IntLocal, 3)
	code.IInc(2, -1).Branch(cf.GOTO, header)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalNaturalSingleEntry(entry, budget int) int {
	acc := entry
	for x := budget; x > 0; x-- {
		acc += x
	}
	return acc
}

func buildNaturalMultipleLatch(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	header, even, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Mark(header).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 2).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, even)
	addConst(class, code, 3, 3)
	code.IInc(2, -1).Branch(cf.GOTO, header)
	code.Mark(even)
	addConst(class, code, 3, 2)
	code.IInc(2, -1).Branch(cf.GOTO, header)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalNaturalMultipleLatch(entry, budget int) int {
	acc := entry
	for x := budget; x > 0; x-- {
		if x&1 == 0 {
			acc += 2
		} else {
			acc += 3
		}
	}
	return acc
}

func buildDeadSecondaryEntry(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	dead, a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
	code.Mark(dead)
	addConst(class, code, 3, 99)
	code.Branch(cf.GOTO, b)
	code.Mark(a)
	addConst(class, code, 3, 2)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
	code.Mark(b)
	addConst(class, code, 3, 3)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalDeadSecondaryEntry(entry, budget int) int {
	acc, x, atA := entry, budget, true
	for x > 0 {
		if atA {
			acc += 2
		} else {
			acc += 3
		}
		x--
		atA = !atA
	}
	return acc
}

func buildIrreducibleTwoEntry(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
	code.Mark(a)
	addConst(class, code, 3, 3)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
	code.Mark(b)
	addConst(class, code, 3, 5)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalIrreducibleTwoEntry(entry, budget int) int {
	acc, x, atA := entry, budget, entry == 0
	for x > 0 {
		if atA {
			acc += 3
		} else {
			acc += 5
		}
		x--
		atA = !atA
	}
	return acc
}

func buildIrreducibleMidBodyPhi(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	header, middle, exit := code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Load(cf.IntLocal, 0).Store(cf.IntLocal, 4)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, header).Branch(cf.GOTO, middle)
	code.Mark(header)
	code.Load(cf.IntLocal, 3).Emit(cf.ICONST_2).Emit(cf.IADD).Store(cf.IntLocal, 4)
	code.Branch(cf.GOTO, middle)
	code.Mark(middle)
	code.Load(cf.IntLocal, 3).Load(cf.IntLocal, 4).Emit(cf.IADD).Store(cf.IntLocal, 3)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, header)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalIrreducibleMidBodyPhi(entry, budget int) int {
	acc, value := entry, entry
	if budget <= 0 {
		return acc
	}
	atHeader := entry == 0
	for x := budget; x > 0; x-- {
		if atHeader {
			value = acc + 2
		}
		acc += value
		atHeader = true
	}
	return acc
}

func buildIrreducibleSwitchThreeEntry(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	a, b, c, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 0).TableSwitch(0, a, a, b, c)
	code.Mark(a)
	addConst(class, code, 3, 1)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, b)
	code.Mark(b)
	addConst(class, code, 3, 10)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, c)
	code.Mark(c)
	addConst(class, code, 3, 100)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, a)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalIrreducibleSwitchThreeEntry(entry, budget int) int {
	acc, x, state := entry, budget, 0
	if entry == 1 {
		state = 1
	} else if entry == 2 {
		state = 2
	}
	for x > 0 {
		switch state {
		case 0:
			acc++
		case 1:
			acc += 10
		case 2:
			acc += 100
		}
		x--
		state = (state + 1) % 3
	}
	return acc
}

func buildIrreducibleMultiExitEffects(class *cf.Class) error {
	class.AddField(cf.ACC_PUBLIC|cf.ACC_STATIC, "effects", "I")
	effects, err := class.Pool().Fieldref("IrreducibleMultiExitEffects", "effects", "I")
	if err != nil {
		return err
	}
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	a, b := code.NewLabel(), code.NewLabel()
	exitA, exitB, budgetExit := code.NewLabel(), code.NewLabel(), code.NewLabel()
	code.Emit(cf.ICONST_0).CP(cf.PUTSTATIC, effects)
	initialize(class, code)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, budgetExit)
	code.Load(cf.IntLocal, 0).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
	code.Mark(a)
	code.CP(cf.GETSTATIC, effects).Emit(cf.ICONST_1).Emit(cf.IADD).CP(cf.PUTSTATIC, effects)
	addConst(class, code, 3, 2)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, budgetExit)
	code.Load(cf.IntLocal, 3).Byte(cf.BIPUSH, 7).Emit(cf.IAND).Branch(cf.IFEQ, exitA).Branch(cf.GOTO, b)
	code.Mark(b)
	code.CP(cf.GETSTATIC, effects).Byte(cf.BIPUSH, 10).Emit(cf.IADD).CP(cf.PUTSTATIC, effects)
	addConst(class, code, 3, 3)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, budgetExit)
	code.Load(cf.IntLocal, 3).Emit(cf.ICONST_3).Emit(cf.IAND).Branch(cf.IFEQ, exitB).Branch(cf.GOTO, a)
	code.Mark(exitA)
	code.Load(cf.IntLocal, 3).Short(cf.SIPUSH, 1000).Emit(cf.IADD).CP(cf.GETSTATIC, effects).Emit(cf.IADD).Emit(cf.IRETURN)
	code.Mark(exitB)
	code.Load(cf.IntLocal, 3).Short(cf.SIPUSH, 2000).Emit(cf.IADD).CP(cf.GETSTATIC, effects).Emit(cf.IADD).Emit(cf.IRETURN)
	code.Mark(budgetExit)
	code.Load(cf.IntLocal, 3).CP(cf.GETSTATIC, effects).Emit(cf.IADD).Emit(cf.IRETURN)
	return nil
}

func evalIrreducibleMultiExitEffects(entry, budget int) int {
	acc, x, effects, atA := entry, budget, 0, entry == 0
	for x > 0 {
		if atA {
			effects++
			acc += 2
			x--
			if x <= 0 {
				return acc + effects
			}
			if acc&7 == 0 {
				return acc + 1000 + effects
			}
		} else {
			effects += 10
			acc += 3
			x--
			if x <= 0 {
				return acc + effects
			}
			if acc&3 == 0 {
				return acc + 2000 + effects
			}
		}
		atA = !atA
	}
	return acc + effects
}

func buildRegionalIrreducibility(class *cf.Class) error {
	code := class.AddMethod(cf.ACC_PUBLIC|cf.ACC_STATIC, "run", "(II)I").Code()
	header, a, b, exit := code.NewLabel(), code.NewLabel(), code.NewLabel(), code.NewLabel()
	initialize(class, code)
	code.Load(cf.IntLocal, 2).Branch(cf.IFLE, exit).Branch(cf.GOTO, header)
	code.Mark(header)
	code.Load(cf.IntLocal, 3).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, a).Branch(cf.GOTO, b)
	code.Mark(a)
	addConst(class, code, 3, 2)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 2).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, header).Branch(cf.GOTO, b)
	code.Mark(b)
	addConst(class, code, 3, 5)
	code.IInc(2, -1).Load(cf.IntLocal, 2).Branch(cf.IFLE, exit)
	code.Load(cf.IntLocal, 2).Emit(cf.ICONST_1).Emit(cf.IAND).Branch(cf.IFEQ, a).Branch(cf.GOTO, header)
	code.Mark(exit)
	returnLocal(code, 3)
	return nil
}

func evalRegionalIrreducibility(entry, budget int) int {
	acc, x, state := entry, budget, 0 // 0=H, 1=A, 2=B
	for x > 0 {
		switch state {
		case 0:
			if acc&1 == 0 {
				state = 1
			} else {
				state = 2
			}
		case 1:
			acc += 2
			x--
			if x&1 == 0 {
				state = 0
			} else {
				state = 2
			}
		case 2:
			acc += 5
			x--
			if x&1 == 0 {
				state = 1
			} else {
				state = 0
			}
		}
	}
	return acc
}
