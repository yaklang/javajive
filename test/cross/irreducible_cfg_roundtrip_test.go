package cross

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

const irreducibleCorpusDir = "testdata/irreducible"

const irreducibleMatrixDriver = `import java.lang.reflect.*;
public final class IrreducibleMatrixDriver {
  public static void main(String[] args) throws Exception {
    Class<?> owner = Class.forName(args[0]);
    Method run = owner.getMethod("run", int.class, int.class);
    Field effects = null;
    try { effects = owner.getField("effects"); } catch (NoSuchFieldException ignored) {}
    for (int entry = -3; entry <= 5; entry++) {
      for (int budget = -1; budget <= 12; budget++) {
        int value = ((Integer) run.invoke(null, entry, budget)).intValue();
        System.out.print(entry + "," + budget + "=" + value);
        if (effects != null) System.out.print(",effects=" + effects.getInt(null));
        System.out.println();
      }
    }
  }
}`

type irreducibleSourceBaseline struct {
	SHA256 string
	Bytes  int
	Lines  int
}

var irreducibleSourceBaselines = map[classparser.DecompileMode]map[string]irreducibleSourceBaseline{
	classparser.Precision: {
		"NaturalSingleEntry":          {SHA256: "f3ccd6f8dcbcbe15b99ca4641829cfa28de49b801128df8d3c9286cae693ac24", Bytes: 440, Lines: 18},
		"NaturalMultipleLatch":        {SHA256: "b92302247190fbc629d5210ddec27ed6f9a2828b87eefdfde5fd9b829c9e2fd6", Bytes: 552, Lines: 25},
		"DeadSecondaryEntry":          {SHA256: "49928c9f7fb1e59e1dceeb5b40d69ae08ece056eae3ea47e130949c2adce02dd", Bytes: 604, Lines: 29},
		"IrreducibleTwoEntry":         {SHA256: "687821ad001b5bd5edc69a85eef2cd29137668e95e4789a89a65f47b39343baf", Bytes: 1009, Lines: 55},
		"IrreducibleMidBodyPhi":       {SHA256: "5473c20877149d28e21d47fea9d7da71d05bdef7b0960f552fa0ebf0a45a4673", Bytes: 901, Lines: 48},
		"IrreducibleSwitchThreeEntry": {SHA256: "7f9073787d427f02cd8ae4ebe8e42cdfb684b70d9fc2beb3e6e74e359842c5a6", Bytes: 1307, Lines: 75},
		"IrreducibleMultiExitEffects": {SHA256: "ea6cbde2329f9bc99d2253e5716081939f5f3903cd1b99c4b9bb665cb07f3ac8", Bytes: 1674, Lines: 91},
		"RegionalIrreducibility":      {SHA256: "51a8759311cd7660d7cf1b04245f5cc20d043c08535a3d5afd1099ce21113016", Bytes: 1385, Lines: 75},
	},
}

func init() {
	irreducibleSourceBaselines[classparser.Compatibility] = irreducibleSourceBaselines[classparser.Precision]
}

type irreducibleManifest struct {
	SchemaVersion  int                       `json:"schemaVersion"`
	ClassfileMajor int                       `json:"classfileMajor"`
	Generator      string                    `json:"generator"`
	Vectors        []irreducibleVector       `json:"vectors"`
	Cases          []irreducibleManifestCase `json:"cases"`
}

type irreducibleVector struct {
	Entry  int `json:"entry"`
	Budget int `json:"budget"`
}

type irreducibleManifestCase struct {
	Name             string                 `json:"name"`
	Kind             string                 `json:"kind"`
	GraphEntries     []string               `json:"graphEntries"`
	Description      string                 `json:"description"`
	ClassFile        string                 `json:"classFile"`
	EquivalentSource string                 `json:"equivalentSource"`
	BytecodeDoc      string                 `json:"bytecodeDoc"`
	ControlFlow      irreducibleControlFlow `json:"controlFlow"`
	ExpectedOutput   []int                  `json:"expectedOutput"`
	Size             int                    `json:"size"`
	SHA256           string                 `json:"sha256"`
}

type irreducibleControlFlow struct {
	Blocks       []irreducibleBlock `json:"blocks"`
	Edges        []irreducibleEdge  `json:"edges"`
	CyclicSCC    []string           `json:"cyclicScc"`
	EntryTargets []string           `json:"entryTargets"`
	Reducibility string             `json:"reducibility"`
}

type irreducibleBlock struct {
	Name string `json:"name"`
	PC   int    `json:"pc"`
}

type irreducibleEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

func TestRoundTripIrreducibleCFGCorpus(t *testing.T) {
	manifest, manifestBytes := readIrreducibleManifest(t)
	assertIrreducibleCorpusInventory(t, manifest)
	assertIrreducibleChecksums(t, manifest, manifestBytes)
	assertIrreducibleGeneratorDeterminism(t, manifest)

	classes := make(map[string][]byte, len(manifest.Cases))
	for _, fixture := range manifest.Cases {
		fixture := fixture
		t.Run("classfile/"+fixture.Name, func(t *testing.T) {
			raw := mustReadIrreducibleFile(t, fixture.ClassFile)
			classes[fixture.Name] = raw
			assertIrreducibleClassfile(t, manifest, fixture, raw)
		})
	}
	if len(classes) != len(manifest.Cases) {
		t.Fatalf("classfile validation did not load the complete corpus: got %d classes, want %d", len(classes), len(manifest.Cases))
	}

	java := auditTool(t, "java")
	javac := auditTool(t, "javac")
	driverRoot := compileIrreducibleMatrixDriver(t, javac)
	originalOutputs := make(map[string]string, len(manifest.Cases))
	originalMatrix := make(map[string]string, len(manifest.Cases))
	for _, fixture := range manifest.Cases {
		fixture := fixture
		t.Run("original/"+fixture.Name, func(t *testing.T) {
			got := normalizeIrreducibleOutput(auditCommand(t, filepath.Join(irreducibleCorpusDir, "original"), java,
				"-Xverify:all", "-cp", ".", fixture.Name))
			want := irreducibleExpectedOutput(fixture.ExpectedOutput)
			if got != want {
				t.Fatalf("verified original behavior changed: got %q, manifest pins %q", got, want)
			}
			originalOutputs[fixture.Name] = got
			originalMatrix[fixture.Name] = runIrreducibleMatrix(t, java, driverRoot,
				filepath.Join(irreducibleCorpusDir, "original"), fixture.Name)
		})
	}

	equivalentRoot := t.TempDir()
	equivalentSources := make([]string, 0, len(manifest.Cases))
	for _, fixture := range manifest.Cases {
		path, err := filepath.Abs(filepath.Join(irreducibleCorpusDir, filepath.FromSlash(fixture.EquivalentSource)))
		if err != nil {
			t.Fatal(err)
		}
		equivalentSources = append(equivalentSources, path)
	}
	sort.Strings(equivalentSources)
	equivalentCompileArgs := []string{"-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-d", equivalentRoot}
	auditCommand(t, equivalentRoot, javac, append(equivalentCompileArgs, equivalentSources...)...)
	for _, fixture := range manifest.Cases {
		fixture := fixture
		t.Run("equivalent/"+fixture.Name, func(t *testing.T) {
			got := normalizeIrreducibleOutput(auditCommand(t, equivalentRoot, java,
				"-Xverify:all", "-cp", equivalentRoot, fixture.Name))
			if want := originalOutputs[fixture.Name]; got != want {
				t.Fatalf("verified equivalent Java behavior differs from original: got %q, want %q", got, want)
			}
			if got, want := runIrreducibleMatrix(t, java, driverRoot, equivalentRoot, fixture.Name), originalMatrix[fixture.Name]; got != want {
				t.Fatalf("equivalent Java parameter matrix differs from original:\n got %q\nwant %q", got, want)
			}
		})
	}

	resolver := func(internalName string) ([]byte, bool) {
		data, ok := classes[internalName]
		return data, ok
	}
	for _, mode := range []classparser.DecompileMode{classparser.Precision, classparser.Compatibility} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			runIrreducibleRoundTripMode(t, java, javac, driverRoot, manifest, classes, resolver, originalOutputs, originalMatrix, mode)
		})
	}
}

func readIrreducibleManifest(t *testing.T) (irreducibleManifest, []byte) {
	t.Helper()
	path := filepath.Join(irreducibleCorpusDir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read irreducible manifest %q: %v", path, err)
	}
	var manifest irreducibleManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("strictly decode irreducible manifest %q: %v", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("irreducible manifest %q has trailing JSON: %v", path, err)
	}
	if manifest.SchemaVersion != 1 || manifest.ClassfileMajor != 49 || manifest.Generator != "generate/main.go" {
		t.Fatalf("unsupported irreducible manifest header: schema=%d classfileMajor=%d generator=%q",
			manifest.SchemaVersion, manifest.ClassfileMajor, manifest.Generator)
	}
	if len(manifest.Vectors) == 0 || len(manifest.Cases) == 0 {
		t.Fatalf("irreducible manifest must declare vectors and cases: vectors=%d cases=%d", len(manifest.Vectors), len(manifest.Cases))
	}
	vectorSet := map[irreducibleVector]bool{}
	for _, vector := range manifest.Vectors {
		if vectorSet[vector] {
			t.Fatalf("irreducible manifest repeats vector %+v", vector)
		}
		vectorSet[vector] = true
	}
	return manifest, data
}

func assertIrreducibleCorpusInventory(t *testing.T, manifest irreducibleManifest) {
	t.Helper()
	expected := map[string]bool{
		"README.md": true, "SHA256SUMS": true, "manifest.json": true, manifest.Generator: true,
	}
	names := map[string]bool{}
	for _, fixture := range manifest.Cases {
		if fixture.Name == "" || names[fixture.Name] {
			t.Fatalf("irreducible manifest has empty or duplicate case name %q", fixture.Name)
		}
		names[fixture.Name] = true
		if fixture.Kind == "" || fixture.Description == "" || len(fixture.GraphEntries) == 0 ||
			fixture.ControlFlow.Reducibility == "" || len(fixture.ControlFlow.Blocks) == 0 ||
			len(fixture.ControlFlow.Edges) == 0 || len(fixture.ControlFlow.CyclicSCC) == 0 ||
			len(fixture.ControlFlow.EntryTargets) == 0 || len(fixture.ExpectedOutput) != len(manifest.Vectors) ||
			fixture.Size <= 0 || len(fixture.SHA256) != sha256.Size*2 {
			t.Fatalf("irreducible manifest case %q is incomplete: %+v", fixture.Name, fixture)
		}
		paths := map[string]string{
			fixture.ClassFile:        "original/" + fixture.Name + ".class",
			fixture.EquivalentSource: "equivalent/" + fixture.Name + ".java",
			fixture.BytecodeDoc:      "bytecode/" + fixture.Name + ".md",
		}
		for got, want := range paths {
			if got != want || !fs.ValidPath(got) {
				t.Fatalf("irreducible case %q artifact path = %q, want %q", fixture.Name, got, want)
			}
			expected[got] = true
		}
		blockNames := map[string]bool{}
		blockPCs := map[int]bool{}
		for _, block := range fixture.ControlFlow.Blocks {
			if block.Name == "" || block.PC < 0 || blockNames[block.Name] || blockPCs[block.PC] {
				t.Fatalf("irreducible case %q has invalid or duplicate block %+v", fixture.Name, block)
			}
			blockNames[block.Name], blockPCs[block.PC] = true, true
		}
		for _, edge := range fixture.ControlFlow.Edges {
			if !blockNames[edge.From] || !blockNames[edge.To] || edge.Kind == "" {
				t.Fatalf("irreducible case %q has invalid edge %+v", fixture.Name, edge)
			}
		}
		for _, block := range append(append([]string(nil), fixture.ControlFlow.CyclicSCC...), fixture.ControlFlow.EntryTargets...) {
			if !blockNames[block] {
				t.Fatalf("irreducible case %q topology names unknown block %q", fixture.Name, block)
			}
		}
	}

	var actual []string
	err := filepath.WalkDir(irreducibleCorpusDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular corpus artifact %q", path)
		}
		rel, err := filepath.Rel(irreducibleCorpusDir, path)
		if err != nil {
			return err
		}
		actual = append(actual, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("inventory irreducible corpus: %v", err)
	}
	sort.Strings(actual)
	want := sortedIrreducibleSet(expected)
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("irreducible corpus inventory changed:\n got %q\nwant %q", actual, want)
	}
}

func assertIrreducibleChecksums(t *testing.T, manifest irreducibleManifest, manifestBytes []byte) {
	t.Helper()
	checksumBytes := mustReadIrreducibleFile(t, "SHA256SUMS")
	lines := strings.Split(strings.TrimSuffix(string(checksumBytes), "\n"), "\n")
	registered := make(map[string]string, len(lines))
	pathsInOrder := make([]string, 0, len(lines))
	for lineNumber, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 || !fs.ValidPath(parts[1]) || strings.ToLower(parts[0]) != parts[0] {
			t.Fatalf("SHA256SUMS line %d is not canonical: %q", lineNumber+1, line)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			t.Fatalf("SHA256SUMS line %d has invalid digest: %v", lineNumber+1, err)
		}
		if _, exists := registered[parts[1]]; exists {
			t.Fatalf("SHA256SUMS repeats %q", parts[1])
		}
		registered[parts[1]] = parts[0]
		pathsInOrder = append(pathsInOrder, parts[1])
	}
	wantPaths := map[string]bool{"README.md": true, "manifest.json": true}
	for _, fixture := range manifest.Cases {
		wantPaths[fixture.ClassFile] = true
		wantPaths[fixture.EquivalentSource] = true
		wantPaths[fixture.BytecodeDoc] = true
	}
	if got, want := sortedIrreducibleMapKeys(registered), sortedIrreducibleSet(wantPaths); !reflect.DeepEqual(got, want) {
		t.Fatalf("SHA256SUMS inventory changed:\n got %q\nwant %q", got, want)
	}
	if !sort.StringsAreSorted(pathsInOrder) {
		t.Fatal("SHA256SUMS entries are not sorted by artifact path")
	}
	for path, want := range registered {
		data := mustReadIrreducibleFile(t, path)
		if got := irreducibleHash(data); got != want {
			t.Fatalf("corpus artifact %q SHA-256 changed: got %s, checksum file pins %s", path, got, want)
		}
	}
	if got := registered["manifest.json"]; got != irreducibleHash(manifestBytes) {
		t.Fatalf("manifest checksum mismatch: got %s, want %s", got, irreducibleHash(manifestBytes))
	}
	for _, fixture := range manifest.Cases {
		data := mustReadIrreducibleFile(t, fixture.ClassFile)
		if len(data) != fixture.Size || irreducibleHash(data) != fixture.SHA256 {
			t.Fatalf("case %q class artifact drifted: size=%d want=%d sha256=%s want=%s",
				fixture.Name, len(data), fixture.Size, irreducibleHash(data), fixture.SHA256)
		}
		if registered[fixture.ClassFile] != fixture.SHA256 {
			t.Fatalf("case %q manifest and SHA256SUMS disagree: manifest=%s checksums=%s",
				fixture.Name, fixture.SHA256, registered[fixture.ClassFile])
		}
	}
}

func assertIrreducibleGeneratorDeterminism(t *testing.T, manifest irreducibleManifest) {
	t.Helper()
	t.Run("generator-determinism", func(t *testing.T) {
		repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		moduleRoot := t.TempDir()
		moduleText := fmt.Sprintf("module irreducible-corpus-gate\n\ngo 1.22.0\n\nrequire github.com/yaklang/javajive v0.0.0\n\nreplace github.com/yaklang/javajive => %s\n", strconv.Quote(repositoryRoot))
		if err := os.WriteFile(filepath.Join(moduleRoot, "go.mod"), []byte(moduleText), 0o644); err != nil {
			t.Fatal(err)
		}
		copyNames := []string{"README.md", manifest.Generator}
		for _, fixture := range manifest.Cases {
			copyNames = append(copyNames, fixture.EquivalentSource)
		}
		tempCorpus := filepath.Join(moduleRoot, filepath.FromSlash("test/cross/testdata/irreducible"))
		for _, name := range copyNames {
			copyIrreducibleFile(t, filepath.Join(irreducibleCorpusDir, filepath.FromSlash(name)), filepath.Join(tempCorpus, filepath.FromSlash(name)))
		}
		goTool := auditTool(t, "go")
		auditCommand(t, moduleRoot, goTool, "run", "./test/cross/testdata/irreducible/generate")

		var checkedIn []string
		err = filepath.WalkDir(irreducibleCorpusDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, err := filepath.Rel(irreducibleCorpusDir, path)
			if err != nil {
				return err
			}
			checkedIn = append(checkedIn, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(checkedIn)
		var generated []string
		err = filepath.WalkDir(tempCorpus, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, err := filepath.Rel(tempCorpus, path)
			if err != nil {
				return err
			}
			generated = append(generated, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(generated)
		if !reflect.DeepEqual(generated, checkedIn) {
			t.Fatalf("isolated generator inventory differs:\n got %q\nwant %q", generated, checkedIn)
		}
		for _, name := range checkedIn {
			got, err := os.ReadFile(filepath.Join(tempCorpus, filepath.FromSlash(name)))
			if err != nil {
				t.Fatal(err)
			}
			want := mustReadIrreducibleFile(t, name)
			if !bytes.Equal(got, want) {
				t.Fatalf("isolated generator output %q differs: got sha256=%s, checked-in sha256=%s", name, irreducibleHash(got), irreducibleHash(want))
			}
		}
	})
}

func assertIrreducibleClassfile(t *testing.T, manifest irreducibleManifest, fixture irreducibleManifestCase, raw []byte) {
	t.Helper()
	object, err := classparser.Parse(raw)
	if err != nil {
		t.Fatalf("classparser.Parse: %v", err)
	}
	if object.GetClassName() != fixture.Name || int(object.MajorVersion) != manifest.ClassfileMajor || object.MinorVersion != 0 {
		t.Fatalf("parsed identity/version = %q %d.%d, want %q %d.0", object.GetClassName(), object.MajorVersion, object.MinorVersion, fixture.Name, manifest.ClassfileMajor)
	}
	var runCode *classparser.CodeAttribute
	runCount := 0
	for _, method := range object.Methods {
		nameInfo := object.ConstantPoolManager.GetUtf8(int(method.NameIndex))
		descriptorInfo := object.ConstantPoolManager.GetUtf8(int(method.DescriptorIndex))
		if nameInfo == nil || descriptorInfo == nil || nameInfo.Value != "run" || descriptorInfo.Value != "(II)I" {
			continue
		}
		runCount++
		for _, attribute := range method.Attributes {
			if code, ok := attribute.(*classparser.CodeAttribute); ok {
				if runCode != nil {
					t.Fatal("run(II)I has more than one Code attribute")
				}
				runCode = code
			}
		}
	}
	if runCount != 1 || runCode == nil {
		t.Fatalf("run(II)I extraction failed: declarations=%d code=%v", runCount, runCode != nil)
	}
	oracle, err := parseCodeAttributeCFGOracle(runCode)
	if err != nil {
		t.Fatalf("independent bytecode CFG oracle: %v", err)
	}
	assertIrreducibleTopology(t, fixture, oracle)
}

func assertIrreducibleTopology(t *testing.T, fixture irreducibleManifestCase, oracle *bytecodeCFGOracle) {
	t.Helper()
	pcByName := make(map[string]int, len(fixture.ControlFlow.Blocks))
	var wantBlocks []int
	for _, block := range fixture.ControlFlow.Blocks {
		pcByName[block.Name] = block.PC
		wantBlocks = append(wantBlocks, block.PC)
	}
	if got := oracleBlockStarts(oracle.Blocks); !reflect.DeepEqual(got, wantBlocks) {
		t.Fatalf("oracle block PCs differ from manifest: got %v, want %v", got, wantBlocks)
	}
	gotEdges := make([]string, 0, len(oracle.Edges))
	for _, edge := range oracle.Edges {
		gotEdges = append(gotEdges, irreducibleOracleEdgeKey(edge))
	}
	wantEdges := make([]string, 0, len(fixture.ControlFlow.Edges))
	for _, edge := range fixture.ControlFlow.Edges {
		keys, err := irreducibleManifestEdgeKeys(edge, pcByName)
		if err != nil {
			t.Fatalf("manifest edge %+v: %v", edge, err)
		}
		wantEdges = append(wantEdges, keys...)
	}
	sort.Strings(gotEdges)
	sort.Strings(wantEdges)
	if !reflect.DeepEqual(gotEdges, wantEdges) {
		t.Fatalf("oracle edges differ from manifest:\n got %v\nwant %v", gotEdges, wantEdges)
	}
	wantSCC := make([]int, 0, len(fixture.ControlFlow.CyclicSCC))
	for _, name := range fixture.ControlFlow.CyclicSCC {
		wantSCC = append(wantSCC, pcByName[name])
	}
	sort.Ints(wantSCC)
	if len(oracle.CyclicSCCs) != 1 || !reflect.DeepEqual(oracle.CyclicSCCs[0].PCs, wantSCC) {
		t.Fatalf("oracle cyclic SCC differs from manifest: got %#v, want PCs %v", oracle.CyclicSCCs, wantSCC)
	}
	wantEntries := make([]int, 0, len(fixture.ControlFlow.EntryTargets))
	for _, name := range fixture.ControlFlow.EntryTargets {
		wantEntries = append(wantEntries, pcByName[name])
	}
	sort.Ints(wantEntries)
	if got := oracle.CyclicSCCs[0].ReachableEntryTargets; !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("oracle reachable SCC entries differ from manifest: got %v, want %v (encoded entries %v)", got, wantEntries, oracle.CyclicSCCs[0].EntryTargets)
	}
	wantReducible := fixture.Kind == "reducible" || fixture.Kind == "reachable-reducible"
	if oracle.Reducible != wantReducible {
		t.Fatalf("oracle reducibility=%v, want %v for kind %q; witness=%v", oracle.Reducible, wantReducible, fixture.Kind, oracle.IrreducibleCyclePC)
	}
	assertIrreducibleSpecialTopology(t, fixture, oracle, pcByName)
}

func assertIrreducibleSpecialTopology(t *testing.T, fixture irreducibleManifestCase, oracle *bytecodeCFGOracle, pcByName map[string]int) {
	t.Helper()
	region := oracle.CyclicSCCs[0]
	switch fixture.Name {
	case "DeadSecondaryEntry":
		wantEncoded := []int{pcByName["A"], pcByName["B"]}
		sort.Ints(wantEncoded)
		if !reflect.DeepEqual(region.EntryTargets, wantEncoded) || !reflect.DeepEqual(region.ReachableEntryTargets, []int{pcByName["A"]}) {
			t.Fatalf("dead secondary entry contract changed: encoded=%v reachable=%v, want encoded=%v reachable=[%d]",
				region.EntryTargets, region.ReachableEntryTargets, wantEncoded, pcByName["A"])
		}
	case "RegionalIrreducibility":
		allowed := map[int]bool{}
		for _, name := range []string{"A", "Adispatch", "AtoB", "B", "Bdispatch"} {
			allowed[pcByName[name]] = true
		}
		seenA, seenB := false, false
		for _, pc := range oracle.IrreducibleCyclePC {
			if !allowed[pc] {
				t.Fatalf("regional irreducibility witness escaped the A/B kernel: witness=%v unexpected pc=%d", oracle.IrreducibleCyclePC, pc)
			}
			seenA = seenA || pc == pcByName["A"]
			seenB = seenB || pc == pcByName["B"]
		}
		if len(oracle.IrreducibleCyclePC) == 0 || !seenA || !seenB {
			t.Fatalf("regional irreducibility witness does not cover both A and B: %v", oracle.IrreducibleCyclePC)
		}
	}
}

func irreducibleManifestEdgeKeys(edge irreducibleEdge, pcByName map[string]int) ([]string, error) {
	from, to := pcByName[edge.From], pcByName[edge.To]
	kind := edge.Kind
	switch {
	case strings.HasPrefix(kind, "tableswitch "):
		var keys []string
		if strings.Contains(kind, "default") {
			keys = append(keys, fmt.Sprintf("%d->%d:%s", from, to, bytecodeSwitchDefault))
		}
		caseAt := strings.Index(kind, "case ")
		if caseAt >= 0 {
			word := strings.Fields(kind[caseAt+len("case "):])[0]
			value, err := strconv.ParseInt(strings.TrimSuffix(word, "/default"), 10, 32)
			if err != nil {
				return nil, err
			}
			keys = append(keys, fmt.Sprintf("%d->%d:%s:%d", from, to, bytecodeSwitchCase, value))
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("unrecognized tableswitch kind %q", kind)
		}
		return keys, nil
	case kind == "fallthrough" || strings.HasSuffix(kind, "/fallthrough"):
		return []string{fmt.Sprintf("%d->%d:%s", from, to, bytecodeFallthrough)}, nil
	case strings.HasSuffix(kind, "/taken"):
		return []string{fmt.Sprintf("%d->%d:%s", from, to, bytecodeConditionalTaken)}, nil
	case strings.HasPrefix(kind, "goto"):
		return []string{fmt.Sprintf("%d->%d:%s", from, to, bytecodeGoto)}, nil
	default:
		return nil, fmt.Errorf("unrecognized edge kind %q", kind)
	}
}

func irreducibleOracleEdgeKey(edge bytecodeOracleEdge) string {
	key := fmt.Sprintf("%d->%d:%s", edge.FromPC, edge.ToPC, edge.Kind)
	if edge.Case != nil {
		key += fmt.Sprintf(":%d", *edge.Case)
	}
	return key
}

func runIrreducibleRoundTripMode(
	t *testing.T,
	java, javac, driverRoot string,
	manifest irreducibleManifest,
	classes map[string][]byte,
	resolver func(string) ([]byte, bool),
	originalOutputs map[string]string,
	originalMatrix map[string]string,
	mode classparser.DecompileMode,
) {
	t.Helper()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sources := make([]string, 0, len(manifest.Cases))
	for _, fixture := range manifest.Cases {
		fixture := fixture
		t.Run("decompile/"+fixture.Name, func(t *testing.T) {
			var firstResult, firstSource []byte
			for run := 0; run < 6; run++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				result, err := classparser.DecompileWithOptions(classes[fixture.Name], classparser.DecompileOptions{
					Mode: mode, Resolve: resolver, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Context: ctx,
					Limits: classparser.Limits{
						MaxInputBytes: 1 << 20, MaxReadBytes: 1 << 20, MaxParseItems: 1_000_000,
						MaxGraphScans: 1_000_000, MaxGraphEdges: 1_000_000, MaxSetElementWork: 10_000_000,
						MaxRequestWork: 100_000_000, MaxOutputBytes: 1 << 20, MaxASTDepth: 10_000,
						MaxDiagnostics: 64, MaxNodeCopies: 1_000_000,
					},
				})
				cancel()
				if err != nil {
					t.Fatalf("decompile run %d/6: status=%q diagnostics=%+v error=%v", run+1, result.Status, result.Diagnostics, err)
				}
				assertHealthyIrreducibleDecompile(t, fixture.Name, mode, run, result)
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatalf("marshal decompile result run %d/6: %v", run+1, err)
				}
				if run == 0 {
					firstResult = encoded
					firstSource = []byte(result.Source)
					assertIrreducibleSourceBaseline(t, fixture.Name, mode, firstSource)
					continue
				}
				if !bytes.Equal(encoded, firstResult) || !bytes.Equal([]byte(result.Source), firstSource) {
					t.Fatalf("nondeterministic decompile run %d/6: result sha256=%s want=%s source sha256=%s want=%s",
						run+1, irreducibleHash(encoded), irreducibleHash(firstResult), irreducibleHash([]byte(result.Source)), irreducibleHash(firstSource))
				}
			}
			path := filepath.Join(sourceRoot, fixture.Name+".java")
			if err := os.WriteFile(path, firstSource, 0o644); err != nil {
				t.Fatal(err)
			}
			sources = append(sources, path)
		})
	}
	if len(sources) != len(manifest.Cases) {
		t.Fatalf("mode %s cannot run whole-tree rebuild: healthy sources=%d want=%d", mode, len(sources), len(manifest.Cases))
	}
	sort.Strings(sources)
	rebuiltRoot := filepath.Join(t.TempDir(), "rebuilt-only")
	if err := os.MkdirAll(rebuiltRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	compileArgs := []string{"-proc:none", "-encoding", "UTF-8", "-Xlint:-options", "--release", "8", "-classpath", rebuiltRoot, "-d", rebuiltRoot}
	auditCommand(t, rebuiltRoot, javac, append(compileArgs, sources...)...)
	rebuiltClasses := readIrreducibleClassTree(t, rebuiltRoot)
	if len(rebuiltClasses) != len(manifest.Cases) {
		t.Fatalf("whole-tree rebuilt-only compile emitted %d classes, want exactly %d: %v", len(rebuiltClasses), len(manifest.Cases), sortedIrreducibleMapKeys(rebuiltClasses))
	}
	fqnBySimple := make(map[string]string, len(rebuiltClasses))
	for internalName, data := range rebuiltClasses {
		object, err := classparser.Parse(data)
		if err != nil {
			t.Fatalf("rebuilt class %q does not parse: %v", internalName, err)
		}
		if object.GetClassName() != internalName {
			t.Fatalf("rebuilt class path/identity mismatch: path=%q identity=%q", internalName, object.GetClassName())
		}
		simple := internalName[strings.LastIndex(internalName, "/")+1:]
		if previous := fqnBySimple[simple]; previous != "" {
			t.Fatalf("rebuilt-only tree repeats simple class %q as %q and %q", simple, previous, internalName)
		}
		fqnBySimple[simple] = strings.ReplaceAll(internalName, "/", ".")
	}
	for _, fixture := range manifest.Cases {
		fixture := fixture
		t.Run("rebuilt/"+fixture.Name, func(t *testing.T) {
			fqn := fqnBySimple[fixture.Name]
			if fqn == "" {
				t.Fatalf("whole-tree rebuilt-only compile did not emit %q; emitted %v", fixture.Name, sortedIrreducibleMapKeys(rebuiltClasses))
			}
			got := normalizeIrreducibleOutput(auditCommand(t, rebuiltRoot, java,
				"-Xverify:all", "-cp", rebuiltRoot, fqn))
			if want := originalOutputs[fixture.Name]; got != want {
				t.Fatalf("verified rebuilt behavior differs from original: got %q, want %q", got, want)
			}
			if got, want := runIrreducibleMatrix(t, java, driverRoot, rebuiltRoot, fqn), originalMatrix[fixture.Name]; got != want {
				t.Fatalf("rebuilt parameter matrix differs from original:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func assertHealthyIrreducibleDecompile(t *testing.T, fixture string, mode classparser.DecompileMode, run int, result classparser.DecompileResult) {
	t.Helper()
	wantDiagnostics := []classparser.DecompileDiagnostic(nil)
	if mode == classparser.Precision {
		wantDiagnostics = []classparser.DecompileDiagnostic{{
			Code:    "legacy_source_recovery_disabled",
			Message: "Unproven class-source repairs are disabled. Core structuring and type recovery still require round-trip validation.",
		}}
	}
	if result.Status != "complete" || len(result.StubMethods) != 0 || strings.TrimSpace(result.Source) == "" || !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("decompile run %d/6 is incomplete or violates the exact diagnostic contract: status=%q stubs=%q diagnostics=%+v want=%+v source_bytes=%d",
			run+1, result.Status, result.StubMethods, result.Diagnostics, wantDiagnostics, len(result.Source))
	}
	for _, member := range result.Members {
		if member.State != "preserved" && member.State != "regenerated" {
			t.Fatalf("decompile run %d/6 has incomplete member %+v", run+1, member)
		}
	}
	lower := strings.ToLower(result.Source)
	for _, marker := range []string{
		"yak-decompiler:", "decompile dump failed", "decompile parse failed",
		"undecompilable method body", "unsupported bootstrap", "unsupportedoperationexception",
	} {
		if strings.Contains(lower, marker) {
			t.Fatalf("fixture %q mode %s decompile run %d/6 contains forbidden degradation marker %q:\n%s", fixture, mode, run+1, marker, result.Source)
		}
	}
}

func compileIrreducibleMatrixDriver(t *testing.T, javac string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "IrreducibleMatrixDriver.java")
	if err := os.WriteFile(path, []byte(irreducibleMatrixDriver), 0o644); err != nil {
		t.Fatal(err)
	}
	auditCommand(t, root, javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", root, path)
	return root
}

func runIrreducibleMatrix(t *testing.T, java, driverRoot, targetRoot, className string) string {
	t.Helper()
	targetRoot, err := filepath.Abs(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	classPath := driverRoot + string(os.PathListSeparator) + targetRoot
	return normalizeIrreducibleOutput(auditCommand(t, driverRoot, java,
		"-Xverify:all", "-cp", classPath, "IrreducibleMatrixDriver", className))
}

func assertIrreducibleSourceBaseline(t *testing.T, fixture string, mode classparser.DecompileMode, source []byte) {
	t.Helper()
	got := irreducibleSourceBaseline{
		SHA256: irreducibleHash(source),
		Bytes:  len(source),
		Lines:  bytes.Count(source, []byte{'\n'}),
	}
	want, ok := irreducibleSourceBaselines[mode][fixture]
	if !ok || got != want {
		t.Fatalf("source quality baseline changed for %s/%s: got %+v, want %+v", mode, fixture, got, want)
	}
}

func readIrreducibleClassTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	classes := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			return err
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular compiler output %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		classes[strings.TrimSuffix(filepath.ToSlash(rel), ".class")] = data
		return nil
	})
	if err != nil {
		t.Fatalf("read rebuilt class tree: %v", err)
	}
	return classes
}

func mustReadIrreducibleFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(irreducibleCorpusDir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read irreducible corpus artifact %q: %v", name, err)
	}
	return data
}

func copyIrreducibleFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func irreducibleExpectedOutput(values []int) string {
	var output strings.Builder
	for _, value := range values {
		fmt.Fprintf(&output, "%d\n", value)
	}
	return output.String()
}

func normalizeIrreducibleOutput(output string) string {
	return strings.ReplaceAll(output, "\r\n", "\n")
}

func irreducibleHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func sortedIrreducibleSet(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedIrreducibleMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
