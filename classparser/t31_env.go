package javaclassparser

import (
	"os"
	"sort"
	"strings"
)

// snapshotJDECEnv copies process JDEC_* values once. The returned map is
// request-local; callers must not mutate the process environment.
func snapshotJDECEnv() map[string]string {
	out := make(map[string]string)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "JDEC_") {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// lookupJDEC reads a kill-switch from a request snapshot. A non-nil snapshot
// is closed: missing keys are unset even if the live process environment
// changed after request entry. A nil snapshot is the legacy Decompile path
// and still reads os.Getenv.
func lookupJDEC(snapshot map[string]string, key string) string {
	if snapshot != nil {
		return snapshot[key]
	}
	return os.Getenv(key)
}

func snapshotDigest(snapshot map[string]string) []string {
	if len(snapshot) == 0 {
		return nil
	}
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+snapshot[k])
	}
	return out
}

func (c *ClassObjectDumper) getenv(key string) string {
	if c == nil {
		return os.Getenv(key)
	}
	return lookupJDEC(c.options.EnvSnapshot, key)
}

func effectiveConfigOf(options DecompileOptions) *EffectiveConfig {
	return &EffectiveConfig{
		Mode:               options.Mode,
		MaxAnalysisUpdates: options.MaxAnalysisUpdates,
		TargetRelease:      options.TargetRelease,
		Limits:             options.Limits,
		Env:                snapshotDigest(options.EnvSnapshot),
		Resolver:           options.Resolve != nil,
	}
}
