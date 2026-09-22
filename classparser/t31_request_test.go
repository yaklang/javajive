package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
)

func TestT31C01OpposingRequests(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	const n = 100
	var envWrites atomic.Int32
	var wg sync.WaitGroup
	errCh := make(chan string, n*2)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if os.Getenv("JDEC_T31_SHOULD_NOT_EXIST") != "" {
				envWrites.Add(1)
			}
			r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, MaxAnalysisUpdates: 1})
			if r.Mode != Precision {
				errCh <- "precision mode leaked"
			}
			if len(r.RulesApplied) != 0 {
				errCh <- "precision applied compatibility rules"
			}
			if r.Status == "complete" && len(r.StubMethods) == 0 {
				errCh <- "low budget reported complete"
			}
			if r.Status == "unsupported" {
				errCh <- "budget disguised as unsupported"
			}
			if err != nil && r.Status != "resource_limit" && r.Status != "partial" && r.Status != "canceled" {
				errCh <- err.Error()
			}
		}()
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility})
			if err != nil {
				errCh <- err.Error()
				return
			}
			if r.Mode != Compatibility {
				errCh <- "compatibility mode leaked"
			}
			if r.Status == "resource_limit" {
				errCh <- "high-budget compatibility hit resource_limit"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var msgs []string
	for m := range errCh {
		msgs = append(msgs, m)
	}
	if len(msgs) > 0 {
		t.Fatalf("T31-C01 opposing request isolation failed: %s", strings.Join(msgs, "; "))
	}
	if envWrites.Load() != 0 {
		t.Fatalf("T31-C01 observed unexpected process env")
	}
}

func TestT31C02ResolverIsolation(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	emptySuper := func(string) ([]byte, bool) { return nil, false }
	realSuper := func(internalName string) ([]byte, bool) {
		if internalName == "SuperSeed" {
			return superBytes, true
		}
		return nil, false
	}
	const n = 40
	var wg sync.WaitGroup
	errCh := make(chan string, n*2)
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(implBytes, DecompileOptions{Mode: Precision, Resolve: realSuper})
			if err != nil {
				errCh <- err.Error()
				return
			}
			if !inheritedThisMethodArgCastRe.MatchString(r.Source) {
				errCh <- "real resolver lost SuperSeed identity"
			}
		}()
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(implBytes, DecompileOptions{Mode: Precision, Resolve: emptySuper})
			if err != nil {
				errCh <- err.Error()
				return
			}
			if inheritedThisMethodArgCastRe.MatchString(r.Source) {
				errCh <- "empty resolver mixed SuperSeed cache"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var msgs []string
	for m := range errCh {
		msgs = append(msgs, m)
	}
	if len(msgs) > 0 {
		t.Fatalf("T31-C02 resolver isolation failed: %s", strings.Join(msgs, "; "))
	}
}

func TestT31C03InvalidOptions(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	r, err := DecompileWithOptions(raw, DecompileOptions{Mode: "nope"})
	if err == nil || r.Status != "invalid_input" {
		t.Fatalf("T31-C03 unknown mode: %+v %v", r, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "invalid class") {
		t.Fatalf("T31-C03 unknown mode reported as invalid class: %v", err)
	}
	r, err = DecompileWithOptions(raw, DecompileOptions{MaxAnalysisUpdates: -3})
	if err == nil || r.Status != "invalid_input" {
		t.Fatalf("T31-C03 negative budget: %+v %v", r, err)
	}
	zero, err := DecompileWithOptions(raw, DecompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if zero.Mode != Precision {
		t.Fatalf("T31-C03 zero-value mode default: %q", zero.Mode)
	}
	if zero.Status == "invalid_input" {
		t.Fatalf("T31-C03 zero-value treated as invalid class: %+v", zero)
	}
}

func TestT31C05LegacyDecompileEntry(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	precise, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	compat, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility})
	if err != nil {
		t.Fatal(err)
	}
	if precise.Status != "complete" && precise.Status != "partial" {
		t.Fatalf("T31-C05 precision status: %+v", precise)
	}
	if legacy == "" {
		t.Fatal("T31-C05 legacy Decompile empty")
	}
	// Compatibility may rewrite; Precision must not apply those rules.
	if len(precise.RulesApplied) != 0 {
		t.Fatalf("T31-C05 precision applied rules: %+v", precise.RulesApplied)
	}
	_ = compat
}

func TestT31C06EnvSnapshotHelper(t *testing.T) {
	t.Setenv("JDEC_T31_PROBE", "before")
	snap := snapshotJDECEnv()
	if snap["JDEC_T31_PROBE"] != "before" {
		t.Fatalf("T31-C06 snapshot missed live value: %+v", snap)
	}
	t.Setenv("JDEC_T31_PROBE", "after")
	if lookupJDEC(snap, "JDEC_T31_PROBE") != "before" {
		t.Fatalf("T31-C06 snapshot followed live env")
	}
	if lookupJDEC(nil, "JDEC_T31_PROBE") != "after" {
		t.Fatalf("T31-C06 nil snapshot should read live env for legacy Decompile")
	}
}

func TestT31C04StatusTruth(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("invalid_class", func(t *testing.T) {
		r, err := DecompileWithOptions([]byte{0xca, 0xfe, 0xba}, DecompileOptions{Mode: Precision})
		if err == nil || r.Status != "invalid_input" {
			t.Fatalf("T31-C04 invalid class: %+v %v", r, err)
		}
		if r.EffectiveConfig == nil || r.EffectiveConfig.Mode != Precision {
			t.Fatalf("T31-C04 missing effective config: %+v", r.EffectiveConfig)
		}
	})

	t.Run("budget_exhausted", func(t *testing.T) {
		r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, Limits: Limits{MaxRequestWork: 1}})
		if r.Status != "resource_limit" {
			t.Fatalf("T31-C04 budget status=%s err=%v", r.Status, err)
		}
		if err == nil || (!isBudgetErr(err) && r.Status != "resource_limit") {
			t.Fatalf("T31-C04 budget err missing: %v", err)
		}
		if r.Status == "unsupported" {
			t.Fatal("T31-C04 budget disguised as unsupported")
		}
	})

	t.Run("method_local_partial", func(t *testing.T) {
		r, err := DecompileWithOptions(raw, DecompileOptions{MaxAnalysisUpdates: 1})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != "partial" || len(r.StubMethods) == 0 {
			t.Fatalf("T31-C04 method-local budget: %+v", r)
		}
	})

	t.Run("complete_sample", func(t *testing.T) {
		r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != "complete" {
			t.Fatalf("T31-C04 complete sample: %+v", r)
		}
		if r.EffectiveConfig == nil {
			t.Fatal("T31-C04 complete missing EffectiveConfig")
		}
	})

	t.Run("unknown_bootstrap", func(t *testing.T) {
		src, err := os.ReadFile(filepath.Join("testdata", "task_contracts", "ConcatProbe.java"))
		if err != nil {
			t.Fatal(err)
		}
		classBytes := compileJavaClassRelease(t, "ConcatProbe", string(src), "17")
		old := []byte("StringConcatFactory")
		repl := []byte("XxxxxxConcatFactory")
		if len(old) != len(repl) || bytes.Count(classBytes, old) == 0 {
			t.Fatalf("T31-C04 cannot rewrite bootstrap utf8")
		}
		mutated := bytes.ReplaceAll(classBytes, old, repl)
		r, err := DecompileWithOptions(mutated, DecompileOptions{Mode: Precision})
		if r.Status == "complete" {
			t.Fatalf("T31-C04 unknown bootstrap counted complete: %+v err=%v", r, err)
		}
		if r.Status != "unsupported" && r.Status != "partial" {
			t.Fatalf("T31-C04 unknown bootstrap status=%s err=%v", r.Status, err)
		}
		found := false
		for _, d := range r.Diagnostics {
			if d.Code == "unsupported_bootstrap" || strings.Contains(d.Message, "unsupported_bootstrap") {
				found = true
			}
		}
		if r.Status == "unsupported" && !found {
			t.Fatalf("T31-C04 unsupported without bootstrap diagnostic: %+v", r.Diagnostics)
		}
		if r.Status == "complete" {
			t.Fatal("stub/unknown counted as round-trip")
		}
	})
}

func TestT31C06RequestSnapshotStable(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	realSuper := func(internalName string) ([]byte, bool) {
		if internalName == "SuperSeed" {
			return superBytes, true
		}
		return nil, false
	}

	snapOn := map[string]string{}
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "1")
	on, err := DecompileWithOptions(implBytes, DecompileOptions{Mode: Precision, Resolve: realSuper, EnvSnapshot: snapOn})
	if err != nil {
		t.Fatal(err)
	}
	if !inheritedThisMethodArgCastRe.MatchString(on.Source) {
		t.Fatalf("T31-C06 snapshot without kill-switch lost (K) cast after live env change:\n%s", on.Source)
	}
	if on.EffectiveConfig == nil {
		t.Fatal("T31-C06 missing EffectiveConfig")
	}
	for _, e := range on.EffectiveConfig.Env {
		if strings.HasPrefix(e, "JDEC_GENERIC_SUPER_METHOD_OFF=") && !strings.HasSuffix(e, "=") && strings.Contains(e, "=1") {
			t.Fatalf("T31-C06 EffectiveConfig followed live env: %q", e)
		}
	}

	os.Unsetenv("JDEC_GENERIC_SUPER_METHOD_OFF")
	off, err := DecompileWithOptions(implBytes, DecompileOptions{
		Mode:        Precision,
		Resolve:     realSuper,
		EnvSnapshot: map[string]string{"JDEC_GENERIC_SUPER_METHOD_OFF": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inheritedThisMethodArgCastRe.MatchString(off.Source) {
		t.Fatalf("T31-C06 snapshot kill-switch ignored; live env leaked into request:\n%s", off.Source)
	}
}

func TestT31C01OpposingLoadBearingFlagsRace(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	longRaw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(internalName string) ([]byte, bool) {
		if internalName == "SuperSeed" {
			return superBytes, true
		}
		return nil, false
	}
	const n = 40
	var wg sync.WaitGroup
	errCh := make(chan string, n*6)
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "1")
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	t.Setenv("JDEC_NO_ENUM_FOLD", "1")
	for i := 0; i < n; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(implBytes, DecompileOptions{
				Mode:        Precision,
				Resolve:     resolve,
				EnvSnapshot: map[string]string{}, // flag off in snapshot
			})
			if err != nil {
				errCh <- "on: " + err.Error()
				return
			}
			if !inheritedThisMethodArgCastRe.MatchString(r.Source) {
				errCh <- "snapshot empty lost (K) while live env has OFF=1"
			}
		}()
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(implBytes, DecompileOptions{
				Mode:        Precision,
				Resolve:     resolve,
				EnvSnapshot: map[string]string{"JDEC_GENERIC_SUPER_METHOD_OFF": "1"},
			})
			if err != nil {
				errCh <- "off: " + err.Error()
				return
			}
			if inheritedThisMethodArgCastRe.MatchString(r.Source) {
				errCh <- "snapshot OFF=1 mixed with empty snapshot"
			}
		}()
		go func() {
			defer wg.Done()
			r, err := DecompileWithOptions(longRaw, DecompileOptions{
				Mode:        Precision,
				EnvSnapshot: map[string]string{"JDEC_IDENT_SELF_CAST_OFF": "1", "JDEC_NO_ENUM_FOLD": "1"},
			})
			if err != nil {
				errCh <- "long: " + err.Error()
				return
			}
			if r.Mode != Precision {
				errCh <- "long mode leaked"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	var msgs []string
	for m := range errCh {
		msgs = append(msgs, m)
	}
	if len(msgs) > 0 {
		t.Fatalf("T31 opposing flags race: %s", strings.Join(msgs, "; "))
	}
}

func TestT31NestedDecompileRestoresOuterFlags(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "host")
	var innerStatus string
	var innerHasCast bool
	resolve := func(internalName string) ([]byte, bool) {
		if internalName != "SuperSeed" {
			return nil, false
		}
		inner, err := DecompileWithOptions(implBytes, DecompileOptions{
			Mode: Precision,
			Resolve: func(internal string) ([]byte, bool) {
				if internal == "SuperSeed" {
					return superBytes, true
				}
				return nil, false
			},
			EnvSnapshot: map[string]string{"JDEC_GENERIC_SUPER_METHOD_OFF": "1"},
		})
		if err == nil {
			innerStatus = inner.Status
			innerHasCast = inheritedThisMethodArgCastRe.MatchString(inner.Source)
		}
		return superBytes, true
	}
	outer, err := DecompileWithOptions(implBytes, DecompileOptions{
		Mode:        Precision,
		Resolve:     resolve,
		EnvSnapshot: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if innerHasCast {
		t.Fatalf("inner snapshot OFF=1 still emitted (K) cast; status=%s", innerStatus)
	}
	if !inheritedThisMethodArgCastRe.MatchString(outer.Source) {
		t.Fatalf("outer snapshot lost after nested decompile (host/inner leak):\n%s", outer.Source)
	}
}

func TestT31NestedLegacyDumpRestoresOuter(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "1")
	var inner string
	resolve := func(internalName string) ([]byte, bool) {
		if internalName != "SuperSeed" {
			return nil, false
		}
		// Nested legacy Dump: no EnvSnapshot, no Setenv inside the request.
		src, err := Decompile(implBytes)
		if err != nil {
			t.Errorf("legacy inner Dump: %v", err)
		}
		inner = src
		return superBytes, true
	}
	outer, err := DecompileWithOptions(implBytes, DecompileOptions{
		Mode:        Precision,
		Resolve:     resolve,
		EnvSnapshot: map[string]string{}, // flag unset for outer
	})
	if err != nil {
		t.Fatal(err)
	}
	if inheritedThisMethodArgCastRe.MatchString(inner) {
		t.Fatalf("legacy inner Dump should follow live env OFF=1, still had (K) cast:\n%s", inner)
	}
	if !inheritedThisMethodArgCastRe.MatchString(outer.Source) {
		t.Fatalf("outer snapshot not restored after nested legacy Dump:\n%s", outer.Source)
	}
}

func TestT31NestedDecompileFlagsRace(t *testing.T) {
	implBytes, err := os.ReadFile("testdata/regression/InheritedThisSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	superBytes, err := os.ReadFile("testdata/regression/SuperSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_GENERIC_SUPER_METHOD_OFF", "host")
	var wg sync.WaitGroup
	errCh := make(chan string, 80)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolve := func(internalName string) ([]byte, bool) {
				if internalName != "SuperSeed" {
					return nil, false
				}
				inner, _ := DecompileWithOptions(implBytes, DecompileOptions{
					Mode: Precision,
					Resolve: func(internal string) ([]byte, bool) {
						if internal == "SuperSeed" {
							return superBytes, true
						}
						return nil, false
					},
					EnvSnapshot: map[string]string{"JDEC_GENERIC_SUPER_METHOD_OFF": "1"},
				})
				if inheritedThisMethodArgCastRe.MatchString(inner.Source) {
					errCh <- "inner mixed outer/host"
				}
				return superBytes, true
			}
			outer, err := DecompileWithOptions(implBytes, DecompileOptions{
				Mode:        Precision,
				Resolve:     resolve,
				EnvSnapshot: map[string]string{},
			})
			if err != nil {
				errCh <- err.Error()
				return
			}
			if !inheritedThisMethodArgCastRe.MatchString(outer.Source) {
				errCh <- "outer lost (K) after nested"
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for m := range errCh {
		t.Error(m)
	}
}

func compileJavaClassRelease(t *testing.T, className, src, release string) []byte {
	t.Helper()
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Fatalf("T31-C04 javac required: %v", err)
	}
	dir := t.TempDir()
	srcPath := filepath.Join(dir, className+".java")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(javac, "--release", release, "-d", dir, srcPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("javac %s: %v\n%s", className, err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, className+".class"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
