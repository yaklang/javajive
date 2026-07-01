package cross

// JavaJive decompile-throughput benchmark (SELF metric; no external tool). It answers "how fast does
// the pure-Go decompiler core turn class bytes into Java source", end-to-end through the production
// JarFS.ReadFile path (zip read + full decompile + dump) over the benchmark jars.
//
// This measures the SINGLE-THREAD (sequential) rate, which is exactly the production archive path:
// javajive.DecompileArchive -> jarwar.DumpToLocalFileSystem walks the archive and decompiles one
// class at a time (see classparser/jarwar/jarwar.go). Concurrent decompile (as quoted on the site's
// "proven at scale" section) is an external harness that fans the per-class Decompile out over a
// worker pool; on an N-core box it scales this single-thread rate by roughly N. We report the honest,
// reproducible single-thread number here; multiply by the core count for the concurrent ballpark.
//
// A warm-up pass decompiles the first jar once before timing so the one-time lazy decompression of the
// embedded JDK stdlib (classparser/classes, gzip-embed) is not charged to the first measured jar.
//
// Opt-in with BENCHMARK=1 (needs ~/.m2):
//
//	BENCHMARK=1 go test -run TestBenchmarkSelfThroughput -v ./test/cross/
//	BENCHMARK=1 BENCHMARK_JARS=guava,fastjson2 go test -run TestBenchmarkSelfThroughput -v ./test/cross/

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

func TestBenchmarkSelfThroughput(t *testing.T) {
	if os.Getenv("BENCHMARK") == "" {
		t.Skip("set BENCHMARK=1 to run the JavaJive decompile-throughput benchmark")
	}

	jars := benchmarkJars
	if sel := os.Getenv("BENCHMARK_JARS"); sel != "" {
		want := map[string]bool{}
		for _, k := range strings.Split(sel, ",") {
			want[strings.TrimSpace(k)] = true
		}
		var filtered []benchJar
		for _, j := range benchmarkJars {
			if want[j.key] {
				filtered = append(filtered, j)
			}
		}
		jars = filtered
	}

	// Warm up the lazily-decompressed embedded JDK stdlib once so it is not charged to the first jar.
	warmed := false
	for _, j := range jars {
		if jarPath := resolveJar(j.relPath); jarPath != "" {
			if fsw, err := classparser.NewJarFSFromLocal(jarPath); err == nil {
				for i, e := range classEntries(t, jarPath) {
					if i >= 30 {
						break
					}
					_, _ = fsw.ReadFile(e)
				}
				warmed = true
				break
			}
		}
	}
	if !warmed {
		t.Skip("no benchmark jars found under ~/.m2")
	}

	type row struct {
		jar     string
		classes int
		dur     time.Duration
	}
	var rows []row
	var totClasses int
	var totDur time.Duration

	for _, j := range jars {
		jarPath := resolveJar(j.relPath)
		if jarPath == "" {
			t.Logf("[skip] %s not found under %s", j.key, m2Repo())
			continue
		}
		entries := classEntries(t, jarPath)
		if len(entries) == 0 {
			continue
		}
		jfs, err := classparser.NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatalf("NewJarFSFromLocal %s: %v", jarPath, err)
		}
		start := time.Now()
		for _, e := range entries {
			if _, err := jfs.ReadFile(e); err != nil {
				t.Fatalf("decompile %s!%s: %v", j.key, e, err)
			}
		}
		dur := time.Since(start)

		rows = append(rows, row{jar: j.key, classes: len(entries), dur: dur})
		totClasses += len(entries)
		totDur += dur
		t.Logf("[%s] classes=%d | %.3fs | %.0f classes/s (single-thread)",
			j.key, len(entries), dur.Seconds(), float64(len(entries))/dur.Seconds())
	}

	perSec := func(n int, d time.Duration) float64 {
		if d <= 0 {
			return 0
		}
		return float64(n) / d.Seconds()
	}

	var b strings.Builder
	b.WriteString("\n#### Decompile throughput (SELF metric; end-to-end JarFS.ReadFile: zip read + decompile + dump; single-thread)\n\n")
	fmt.Fprintf(&b, "Machine: %s/%s, %d logical CPUs (Go %s).\n\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	b.WriteString("| jar | classes | seconds | classes/s (single-thread) |\n")
	b.WriteString("|---|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %d | %.2f | %.0f |\n", r.jar, r.classes, r.dur.Seconds(), perSec(r.classes, r.dur))
	}
	fmt.Fprintf(&b, "| **total** | **%d** | **%.2f** | **%.0f** |\n", totClasses, totDur.Seconds(), perSec(totClasses, totDur))
	t.Logf("JavaJive decompile throughput:\n%s", b.String())
}
