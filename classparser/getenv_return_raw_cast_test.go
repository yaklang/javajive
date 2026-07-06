package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestGetenvReturnRawCastIsLoadBearing pins concreteParamReturnSubtypeRawCast Shape 3. `GetenvReturnSeed.env`
// declares `Map<String,Object>` and returns `System.getenv()`, whose JDK signature is the FIXED
// `Map<String,String>` -- same erasure, different args, so the bare return never converts and the direct
// parameterized cast is inconvertible. The source carried the raw `(Map)` cast the bytecode dropped; the fix
// re-inserts it. `envExact` declares `Map<String,String>` (getenv's exact parameterization) and must stay
// cast-free. The kill-switch drops the cast, proving it load-bearing. Real hit: spring
// AbstractEnvironment.getSystemEnvironment().
func TestGetenvReturnRawCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GetenvReturnSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): exactly ONE `(Map)` raw cast (env), none on the exact-match control (envExact).
	os.Unsetenv("JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if got := strings.Count(on, "(Map) (System.getenv())"); got != 1 {
		t.Errorf("fix ON: expected exactly 1 raw `(Map)` cast on System.getenv() (env yes, envExact no), got %d:\n%s", got, on)
	}

	// Fix OFF: no cast anywhere (the uncompilable bare return), proving it is load-bearing.
	t.Setenv("JDEC_CONCRETE_PARAM_RET_SUBTYPE_RAW_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(Map) (System.getenv())") {
		t.Errorf("fix OFF: expected NO raw cast on System.getenv() (kill-switch load-bearing), got:\n%s", off)
	}
}
