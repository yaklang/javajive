// Command requiredci owns the reviewed, fail-closed test inventory for required CI.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type suite struct {
	Group    string
	Package  string
	Tests    int
	TestHash string
	Include  []string
	Exclude  []string
}

var requiredSuites = []suite{
	{Group: "corpus", Package: "./ci/requiredci", Tests: 2, TestHash: "e6aed0096f27de8745dd8495190f7eb24ba7de64b0eafa759767e45b988ae479"},
	{Group: "corpus", Package: "./test/cross", Tests: 104, TestHash: "17c48c808ac80dbb2055c86a65896b22997f04f8ad72643a76cb91025eb2ff43", Include: []string{"TestFixedIrreducibleCFGCorpusPureGo"}},
	{Group: "corpus", Package: "./classparser/classfilebuilder", Tests: 26, TestHash: "d26b13298f017bd8077df6e8910e78f6542e5ffcfd2dae7a36260ab17e61847f", Exclude: []string{"TestJava8InterfaceStaticMethodVerifies", "TestJavaVerifyAll", "TestLongDoubleAndFloatConstantsExecute"}},
	{Group: "corpus", Package: "./classparser/jarwar", Tests: 18, TestHash: "b1195066bc29f6a35391695fb815b4eef9eb42a55ef53f7ba9786375a27afc25"},
	{Group: "corpus", Package: "./classparser/decompiler/jar", Tests: 1, TestHash: "e84c19ac824483c172a763d735732079fb808d6dd0153c53e573b18513d23553"},
	{Group: "corpus", Package: "./jimage", Tests: 14, TestHash: "7c6617c457475dff8be3a1eb712d11488dae8fd3201994abdecc98b1e7c16758", Exclude: []string{"TestJLinkBigEndianImage", "TestJLinkCompactCPImage", "TestOpenJDKImage"}},
	{Group: "corpus", Package: "./classparser", Tests: 4171, TestHash: "5598a3042c01d9c22b1472992e8aa5b7a427eacd44a4030fed865ec37fb6a93f", Include: []string{"TestDumpMethodsPointerFastPathKeepsDescriptor", "TestJoinRuleChainExactOutput", "TestSourceRewritePrecisionOuterGateSemantics", "TestRewriteSchedulerOrderMatchesDumperCallSites", "TestRewriteSchedulerPositiveAndNearMissExactOutput", "TestRewriteSchedulerPreservesRecordsDiagnosticsAndHashes", "TestRewriteSchedulerDecompileEquivalence", "TestJDKObjectMetadataMatchesCatalog", "TestAdversarialPlatformConstructorCodeIdentityAndEffectProof"}},

	{Group: "algorithm", Package: "./classparser/decompiler/core", Tests: 409, TestHash: "1eec9b33c385716c4911830d60edf9d9669e4044285f808723639119164d8971"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/callbinding", Tests: 17, TestHash: "e7eafd33ec36c2219f362b5c45872f126dde5a81dab84d931023fd1abc1836cc"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/class_context", Tests: 26, TestHash: "254c1ad1ea4a5da5d4bc6849e1067c11ef1953a2c171436e59d8b18627811c40"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/fieldbinding", Tests: 1, TestHash: "6ec3bd03aab62727a5d86c38f16a3e99142866db2c91b5d2872616162fc2b06e"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/frametransfer", Tests: 24, TestHash: "9898ba6045bd07fcbf98bbf469f099570072c73edaed0dff76d9857aea6f60f5"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/methodir", Tests: 13, TestHash: "95c6ac735fb6ab912cbc0bfeff4fdbb9a2611afda63de0360fb146cfda552655"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/ssabuild", Tests: 29, TestHash: "ea3b73bdf4252370e93bbb6b07d64ed5a954cd11cfbfd49c2a2a55692c7b27ba"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/ssalower", Tests: 21, TestHash: "ba60145bafd5fda5be48298b5524097b255e2a1edf1226ab0c8153879ce3685d"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/statements", Tests: 47, TestHash: "9f3bcb9c125d2d07a5d09bf4a4f4c119f5e9055cfd55db989869e96b22e574aa"},
	{Group: "algorithm", Package: "./classparser/decompiler/core/values", Tests: 283, TestHash: "1ef92dd8999fa01ec4702b98c9172efbe714dcdd6b35d8d13a8daa71363efb64", Exclude: []string{"TestJavaUnitsExhaustiveJVMCodeUnitOracle", "TestTaskT03C03_CharLiterals"}},
	{Group: "algorithm", Package: "./classparser/decompiler/core/values/types", Tests: 71, TestHash: "03a89ddb73bfdfbb8de135dd84fc6af8ebdb054662e7126d92a58889b0055773"},
	{Group: "algorithm", Package: "./classparser/decompiler/rewriter", Tests: 262, TestHash: "21381e2810021266ba9957a2d0320640f3b61688d7d9a76cece737ae3fb7dd7a"},
	{Group: "algorithm", Package: "./internal/workbudget", Tests: 17, TestHash: "1f53605cb2386de80f254918d30db00ff803eed986bbc7087d0d42488f6dea8b"},
	{Group: "algorithm", Package: "./internal/jdecenv", Tests: 13, TestHash: "51ee508a5994d7763fb87e1d2d9cf78a345237af650479b650dbf055625082ad"},
	{Group: "algorithm", Package: "./internal/filesys", Tests: 9, TestHash: "f6485392fe079ab59aafa635aca0d9bbe00b7af50334a7c1362396d883d82e31"},

	{Group: "dex", Package: "./dex2jar", Tests: 34, TestHash: "256aabae495907857f59e2a4960fd3ea0404cce6455ba30a6f69a1d4f0acf942", Exclude: []string{"TestAcceptedCorpusJVMOracle", "TestCoverageBaselineContract", "TestStrictAcceptedDEXRoundtripGate"}},
	{Group: "dex", Package: "./dex2jar/input", Tests: 24, TestHash: "ef67f45d9b6d13e74806d6bd48a369073f355c14754ca6f93a01b03f3fe63d7a"},
	{Group: "dex", Package: "./dex2jar/ir", Tests: 42, TestHash: "d858db8c7d5f33673d8fd0e3275298cf229ddb4a94046f7d54241b869872c06f"},
	{Group: "dex", Package: "./dex2jar/lower", Tests: 53, TestHash: "cd86a4630c8e95271b56ca6eba7f64052ff4ec8473c8388d3069659ca1a83c80", Exclude: []string{
		"TestClassParsesVerifiesAndExecutes", "TestIntArrayPathVerifiesAndExecutes", "TestUnsupportedStubsVerifyAndThrow", "TestThisDelegationVerifiesAndExecutes",
		"TestLowerEmitsMetadataAttributes", "TestLowerFilledNewArrayAndNarrowArrayElements", "TestLowerEmitsAnnotationDefaultForReflection", "TestLowerEmitsClassMetadataAndArrayOwnerInvoke", "TestLowerMovesPreDelegationFieldStoreAfterSuper", "TestLowerInterleavedAllocationKeepsArgumentOrder", "TestLowerCapturesDeferredStoreValueBeforeReassignment",
		"TestRecoverFailsClosedForUnsupportedStaticInitializer", "TestRecoverClassVerifiesWithoutInitialization", "TestRecoverChildFirstInterfaceShellsVerifyOnJVM",
		"TestInvokeResultUsesMoveResultCategory", "TestLowerWideValuesVerifyAndExecute", "TestLowerMonitorsVerifyAndExecute", "TestLowerTryCatchVerifiesAndExecutes", "TestLowerCatchAllHandlerVerifiesAndExecutes", "TestHandlerWithoutMoveExceptionDiscardsCaughtValue", "TestLowerInterfaceStaticMethodsVerifyAndExecute", "TestLowerSwitchesVerifyAndExecute", "TestLowerFillArrayDataVerifyAndExecute",
	}},
	{Group: "dex", Package: "./dex2jar/report", Tests: 7, TestHash: "6121738f00eca84495e4795ebdd9887db39a8ff30f761bd307bfb7bf0eed2081"},
	{Group: "dex", Package: "./inspect", Tests: 13, TestHash: "3b55a9b12c378a02fd0259f86e3dac3dda338090d3f5af2d06b161fc4065bb23"},
	{Group: "dex", Package: "./internal/dexparser", Tests: 4, TestHash: "982c4f5860610cd86fc786e1a2a8721108f134f1bf6ebbe657f2085e30400c3c"},
	{Group: "dex", Package: "./internal/inputkind", Tests: 5, TestHash: "42170b3c19333161c0213dec5d9b4f8b1862596cffaac4ddb0314460d0d52d47"},
	{Group: "dex", Package: "./internal/mutf8", Tests: 6, TestHash: "f176fe43927c9dff4e9f378419d80c0d889d67e93aa568f3250e7edc8dd3ad72"},
	{Group: "dex", Package: "./internal/zipx", Tests: 2, TestHash: "99bdca5967a4f3f345a748cfc3c8317ea9cb7055ae7fadcaca08ff4fc67abdd0"},

	{Group: "contract", Package: "./tools/requiredci", Tests: 5, TestHash: "6b6056b6358c2517caed772186e1ce80c1609cc0c635ae08e76c83ae641560a6"},
}

var forbiddenWorkflowWord = regexp.MustCompile(`(?i)\b(?:java|javac|javap|jlink|jmod|jimage|python(?:2|3)?)\b`)

func main() {
	check := flag.Bool("check", false, "validate the required-CI contract")
	run := flag.String("run", "", "validate and execute one inventory group")
	flag.Parse()
	if *check == (*run != "") || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/requiredci -check | -run corpus|algorithm|dex|contract")
		os.Exit(2)
	}
	root, err := repositoryRoot()
	if err == nil {
		err = validate(root)
	}
	if err == nil && *run != "" {
		err = runSuites(root, *run)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "required CI:", err)
		os.Exit(1)
	}
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if data, readErr := os.ReadFile(filepath.Join(dir, "go.mod")); readErr == nil && bytes.HasPrefix(data, []byte("module github.com/yaklang/javajive\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository go.mod not found")
		}
		dir = parent
	}
}

func validate(root string) error {
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("read required workflow: %w", err)
	}
	if err := validateWorkflow(string(data)); err != nil {
		return err
	}
	seen := map[string]bool{}
	groups := map[string]bool{}
	for _, item := range requiredSuites {
		key := item.Group + "\x00" + item.Package
		if seen[key] || item.Group == "" || !strings.HasPrefix(item.Package, "./") || len(item.Include) > 0 && len(item.Exclude) > 0 {
			return fmt.Errorf("invalid inventory row: %+v", item)
		}
		seen[key] = true
		groups[item.Group] = true
		all, err := listTests(root, item.Package)
		if err != nil {
			return err
		}
		if item.TestHash != "" && (len(all) != item.Tests || testHash(all) != item.TestHash) {
			return fmt.Errorf("%s inventory changed: tests=%d hash=%s; review and update the sealed row", item.Package, len(all), testHash(all))
		}
		if _, err := selectedTests(item, all); err != nil {
			return err
		}
	}
	for _, group := range []string{"corpus", "algorithm", "dex", "contract"} {
		if !groups[group] {
			return fmt.Errorf("required group %q has no inventory", group)
		}
	}
	return nil
}

func validateWorkflow(text string) error {
	if !strings.Contains(text, "pull_request:") || !strings.Contains(text, "push:") {
		return errors.New("ci.yml must remain the push and pull-request workflow")
	}
	if forbiddenWorkflowWord.MatchString(text) {
		return fmt.Errorf("ci.yml contains forbidden external-tool word %q", forbiddenWorkflowWord.FindString(text))
	}
	allowedActions := map[string]bool{
		"actions/checkout@11d5960a326750d5838078e36cf38b85af677262": true,
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff": true,
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "uses:") && !strings.HasPrefix(line, "- uses:") {
			continue
		}
		spec := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "uses:"))
		if !allowedActions[spec] {
			return fmt.Errorf("ci.yml uses unapproved action %q", spec)
		}
	}
	for _, required := range []string{"go run ./tools/requiredci -check", "CGO_ENABLED=0 go build ./...", "-run corpus", "-run algorithm", "-run dex", "-run contract"} {
		if strings.Count(text, required) != 1 {
			return fmt.Errorf("ci.yml must contain exactly one %q", required)
		}
	}
	if strings.Contains(text, "go test ./...") {
		return errors.New("ci.yml may not bypass the committed test inventory")
	}
	return nil
}

func listTests(root, pkg string) ([]string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}
	command := exec.Command(goPath, "test", pkg, "-list", "^Test")
	command.Dir = root
	command.Env = sanitizedEnvironment("")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list %s: %w\n%s", pkg, err, output)
	}
	var tests []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Test") {
			tests = append(tests, line)
		}
	}
	sort.Strings(tests)
	return tests, nil
}

func selectedTests(item suite, all []string) ([]string, error) {
	known := make(map[string]bool, len(all))
	for _, name := range all {
		known[name] = true
	}
	if len(item.Include) > 0 {
		for _, name := range item.Include {
			if !known[name] {
				return nil, fmt.Errorf("%s includes missing test %s", item.Package, name)
			}
		}
		return append([]string(nil), item.Include...), nil
	}
	excluded := map[string]bool{}
	for _, name := range item.Exclude {
		if !known[name] {
			return nil, fmt.Errorf("%s excludes missing test %s", item.Package, name)
		}
		excluded[name] = true
	}
	selected := make([]string, 0, len(all)-len(excluded))
	for _, name := range all {
		if !excluded[name] {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("%s selects no required tests", item.Package)
	}
	return selected, nil
}

func testHash(tests []string) string {
	sum := sha256.Sum256([]byte(strings.Join(tests, "\n") + "\n"))
	return hex.EncodeToString(sum[:])
}

type testEvent struct {
	Action string
	Test   string
}

func runSuites(root, group string) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return err
	}
	emptyPath, err := os.MkdirTemp("", "javajive-required-path-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(emptyPath)
	work, err := os.MkdirTemp("", "javajive-required-work-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	var coverageProfiles []string
	ran := false
	for suiteIndex, item := range requiredSuites {
		if item.Group != group {
			continue
		}
		ran = true
		all, err := listTests(root, item.Package)
		if err != nil {
			return err
		}
		tests, err := selectedTests(item, all)
		if err != nil {
			return err
		}
		parts := make([]string, len(tests))
		for index, name := range tests {
			parts[index] = regexp.QuoteMeta(name)
		}
		selector := "^(" + strings.Join(parts, "|") + ")$"
		fmt.Printf("required CI: running %s (%d tests)\n", item.Package, len(tests))
		var output []byte
		var runErr error
		if group == "algorithm" || group == "dex" {
			binary := filepath.Join(work, fmt.Sprintf("suite-%d.test", suiteIndex))
			compileArgs := []string{"test", "-c", "-race", "-o", binary}
			if group == "dex" {
				compileArgs = append(compileArgs, "-covermode=atomic", "-coverpkg=./dex2jar/...,./internal/dexparser")
			}
			compileArgs = append(compileArgs, item.Package)
			compile := exec.Command(goPath, compileArgs...)
			compile.Dir = root
			compile.Env = compileEnvironment()
			if compiled, compileErr := compile.CombinedOutput(); compileErr != nil {
				return fmt.Errorf("compile %s: %w\n%s", item.Package, compileErr, compiled)
			}
			runArgs := []string{"tool", "test2json", "-t", "-p", item.Package, binary,
				"-test.v=test2json", "-test.count=1", "-test.timeout=5m", "-test.run=" + selector}
			if group == "dex" {
				profile := filepath.Join(work, fmt.Sprintf("coverage-%d.out", suiteIndex))
				coverageProfiles = append(coverageProfiles, profile)
				runArgs = append(runArgs, "-test.coverprofile="+profile)
			}
			command := exec.Command(goPath, runArgs...)
			command.Dir = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(item.Package, "./")))
			command.Env = sanitizedEnvironment(emptyPath)
			output, runErr = command.CombinedOutput()
		} else {
			command := exec.Command(goPath, "test", "-json", "-count=1", "-timeout=5m", "-run", selector, item.Package)
			command.Dir = root
			command.Env = sanitizedEnvironment(emptyPath)
			output, runErr = command.CombinedOutput()
		}
		_, _ = os.Stdout.Write(output)
		if err := verifyEvents(item.Package, tests, output); err != nil {
			return err
		}
		if runErr != nil {
			return fmt.Errorf("%s failed: %w", item.Package, runErr)
		}
	}
	if !ran {
		return fmt.Errorf("unknown or empty inventory group %q", group)
	}
	if group == "dex" {
		if err := checkCoverage(root, goPath, coverageProfiles); err != nil {
			return err
		}
	}
	return nil
}

func verifyEvents(pkg string, expected []string, output []byte) error {
	terminal := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	buffer := make([]byte, 64<<10)
	scanner.Buffer(buffer, 4<<20)
	for scanner.Scan() {
		var event testEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Test == "" {
			continue
		}
		if event.Action != "pass" && event.Action != "fail" && event.Action != "skip" {
			continue
		}
		if event.Action != "pass" {
			return fmt.Errorf("%s required test %s ended with %s", pkg, event.Test, event.Action)
		}
		if !strings.Contains(event.Test, "/") {
			if previous := terminal[event.Test]; previous != "" {
				return fmt.Errorf("%s test %s has duplicate terminal events", pkg, event.Test)
			}
			terminal[event.Test] = event.Action
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: decode go test output: %w", pkg, err)
	}
	for _, name := range expected {
		if terminal[name] != "pass" {
			return fmt.Errorf("%s required test %s did not pass", pkg, name)
		}
	}
	if len(terminal) != len(expected) {
		return fmt.Errorf("%s terminal inventory mismatch: got %d, want %d", pkg, len(terminal), len(expected))
	}
	return nil
}

type coverageBaseline struct {
	Module               string                   `json:"module"`
	Total                coverageCount            `json:"total"`
	Packages             map[string]coverageCount `json:"packages"`
	ZeroCoveredFunctions []string                 `json:"zero_covered_functions"`
}

type coverageCount struct {
	Covered    int `json:"covered"`
	Statements int `json:"statements"`
}

type coverageBlock struct {
	Statements int
	Count      int64
}

func checkCoverage(root, goPath string, profiles []string) error {
	baselinePath := filepath.Join(root, "dex2jar", "coverage_baseline.json")
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return err
	}
	var baseline coverageBaseline
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&baseline); err != nil {
		return fmt.Errorf("decode coverage baseline: %w", err)
	}
	blocks := map[string]coverageBlock{}
	for _, profile := range profiles {
		profileData, err := os.ReadFile(profile)
		if err != nil {
			return fmt.Errorf("read coverage profile %s: %w", profile, err)
		}
		lines := strings.Split(strings.TrimSpace(string(profileData)), "\n")
		if len(lines) == 0 || lines[0] != "mode: atomic" {
			return fmt.Errorf("coverage profile %s has invalid mode", profile)
		}
		for lineNumber, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) != 3 {
				return fmt.Errorf("coverage profile %s line %d is malformed", profile, lineNumber+2)
			}
			statements, statErr := strconv.Atoi(fields[1])
			count, countErr := strconv.ParseInt(fields[2], 10, 64)
			if statErr != nil || countErr != nil || statements < 0 || count < 0 {
				return fmt.Errorf("coverage profile %s line %d has invalid counts", profile, lineNumber+2)
			}
			block := blocks[fields[0]]
			if block.Statements != 0 && block.Statements != statements {
				return fmt.Errorf("coverage block %s changed statement count", fields[0])
			}
			block.Statements = statements
			if count > block.Count {
				block.Count = count
			}
			blocks[fields[0]] = block
		}
	}
	actual := map[string]coverageCount{}
	prefix := baseline.Module + "/"
	for location, block := range blocks {
		filename := strings.SplitN(location, ":", 2)[0]
		if !strings.HasPrefix(filename, prefix) {
			return fmt.Errorf("coverage file is outside module: %s", filename)
		}
		pkg := filename[:strings.LastIndex(filename, "/")]
		value := actual[pkg]
		value.Statements += block.Statements
		if block.Count != 0 {
			value.Covered += block.Statements
		}
		actual[pkg] = value
	}
	for pkg, want := range baseline.Packages {
		got, ok := actual[pkg]
		if !ok {
			return fmt.Errorf("missing package coverage: %s", pkg)
		}
		if !ratioAtLeast(got, want) {
			return fmt.Errorf("coverage regression %s: %d/%d < %d/%d", pkg, got.Covered, got.Statements, want.Covered, want.Statements)
		}
	}
	for pkg := range actual {
		if _, ok := baseline.Packages[pkg]; !ok {
			return fmt.Errorf("unbaselined package coverage: %s", pkg)
		}
	}
	total := coverageCount{}
	for _, value := range actual {
		total.Covered += value.Covered
		total.Statements += value.Statements
	}
	if !ratioAtLeast(total, baseline.Total) {
		return fmt.Errorf("total coverage regression: %d/%d < %d/%d", total.Covered, total.Statements, baseline.Total.Covered, baseline.Total.Statements)
	}
	merged := filepath.Join(filepath.Dir(profiles[0]), "coverage-merged.out")
	if err := writeCoverageProfile(merged, blocks); err != nil {
		return err
	}
	command := exec.Command(goPath, "tool", "cover", "-func="+merged)
	command.Dir = root
	command.Env = compileEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect coverage functions: %w\n%s", err, output)
	}
	allowed := map[string]bool{}
	for _, name := range baseline.ZeroCoveredFunctions {
		allowed[name] = true
	}
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, "\t0.0%") || strings.HasPrefix(line, "total:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return fmt.Errorf("unrecognized coverage function line %q", line)
		}
		fileAndLine := fields[0]
		goEnd := strings.Index(fileAndLine, ".go:")
		if goEnd < 0 {
			return fmt.Errorf("unrecognized coverage location %q", fileAndLine)
		}
		filename := fileAndLine[:goEnd+len(".go")]
		if !strings.HasPrefix(filename, prefix) {
			return fmt.Errorf("coverage function is outside module: %s", filename)
		}
		identity := strings.TrimPrefix(filename, prefix) + ":" + fields[1]
		if !allowed[identity] {
			return fmt.Errorf("new 0%%-covered function: %s", identity)
		}
	}
	fmt.Printf("required CI: DEX coverage %d/%d statements\n", total.Covered, total.Statements)
	return nil
}

func ratioAtLeast(actual, baseline coverageCount) bool {
	if actual.Statements == 0 || baseline.Statements == 0 {
		return actual.Statements == baseline.Statements
	}
	return actual.Covered*baseline.Statements >= baseline.Covered*actual.Statements
}

func writeCoverageProfile(path string, blocks map[string]coverageBlock) error {
	keys := make([]string, 0, len(blocks))
	for key := range blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	output.WriteString("mode: atomic\n")
	for _, key := range keys {
		block := blocks[key]
		fmt.Fprintf(&output, "%s %d %d\n", key, block.Statements, block.Count)
	}
	return os.WriteFile(path, []byte(output.String()), 0o600)
}

func compileEnvironment() []string {
	forbiddenPrefixes := []string{"JAVA_HOME=", "JDK_HOME=", "JRE_HOME=", "PYTHONHOME=", "PYTHONPATH="}
	result := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		drop := false
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(value, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			result = append(result, value)
		}
	}
	return append(result, "CGO_ENABLED=1", "GOTOOLCHAIN=local")
}

func sanitizedEnvironment(path string) []string {
	forbiddenPrefixes := []string{"JAVA_HOME=", "JDK_HOME=", "JRE_HOME=", "PYTHONHOME=", "PYTHONPATH=", "PATH="}
	result := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		drop := false
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(value, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			result = append(result, value)
		}
	}
	if path != "" {
		result = append(result, "PATH="+path)
	}
	return append(result, "CGO_ENABLED=0", "GOTOOLCHAIN=local")
}
