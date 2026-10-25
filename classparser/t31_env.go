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

// jdecPolicyMap is the native-member cache identity. A class dump binds the
// whole process environment so later lookups avoid the env lock; only JDEC_*
// keys belong in that identity. A map that is already only JDEC_* is returned
// unchanged, including its nil/empty distinction.
func jdecPolicyMap(snap map[string]string) map[string]string {
	if snap == nil {
		return nil
	}
	extra := false
	for k := range snap {
		if !strings.HasPrefix(k, "JDEC_") {
			extra = true
			break
		}
	}
	if !extra {
		return snap
	}
	out := make(map[string]string)
	for k, v := range snap {
		if strings.HasPrefix(k, "JDEC_") {
			out[k] = v
		}
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
		SourceCompiler:     options.SourceCompiler,
	}
}
