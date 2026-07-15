package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestTryBreakReturnIsLoadBearing pins the fixTryBreakReturn post-processing fix. A do-while(true)
// loop containing a `try { return expr; } catch(CancellationException) {...} catch(ExecutionException)
// {...}` is structured by the do-while rewriter as `try { break; } catch(...) {...}` + a hoisted
// `return expr;` after the loop. But the `try { break; }` throws no checked exception, so the
// `catch(ExecutionException)` fails "exception is never thrown in body of corresponding try
// statement". The fix moves the hoisted `return <expr>;` back into the `try { break; }` block,
// replacing `break;` with `return <expr>;` and removing the post-loop return. Kill-switch:
// JDEC_FIX_TRY_BREAK_RETURN_OFF. Real hit: commons-lang3 Memoizer.compute.
//
// This seed verifies the ON path: the return must be inside the try block (not after the loop),
// so the catch clauses are valid (the return's expression can throw the caught exceptions).
func TestTryBreakReturnIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/TryBreakReturnSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_FIX_TRY_BREAK_RETURN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	// The return should be INSIDE the try block (before } while), making the catch valid.
	tryIdx := strings.Index(on, "try{")
	if tryIdx < 0 {
		t.Fatalf("fix ON: no try block found:\n%s", on)
	}
	whileIdx := strings.Index(on, "} while (true);")
	if whileIdx < 0 {
		t.Fatalf("fix ON: no do-while found:\n%s", on)
	}
	returnInTry := strings.Contains(on[tryIdx:whileIdx], "return ")
	if !returnInTry {
		t.Errorf("fix ON: return should be INSIDE the try block (before } while), got:\n%s", on)
	}
	// The output should recompile (no break-in-try + hoisted return pattern).
	if strings.Contains(on[whileIdx:], "return ") {
		// A return after the while loop means the return was NOT moved into the try.
		afterWhile := strings.TrimSpace(on[whileIdx+len("} while (true);"):])
		if strings.HasPrefix(afterWhile, "return ") {
			t.Errorf("fix ON: return should not be hoisted after the do-while, got:\n%s", on)
		}
	}
}