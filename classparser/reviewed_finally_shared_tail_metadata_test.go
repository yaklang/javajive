package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Parse/decompile only: the original normal and typed-handler paths join one
// inert RETURN after their separate cleanup copies. Runtime behavior is covered
// by the independent authored identity/effect oracle.
func TestOriginalNettyFinallySharedVoidReturnMetadata(t *testing.T) {
	const path = "testdata/regression/SslHandler.class"
	_, code, _ := reviewedFixtureMethod(t, path, "handshake", "(Z)V")
	assertReviewedOpcode(t, code, 91, 177)
	if len(code.ExceptionTable) != 4 {
		t.Fatal("original handshake handler count changed")
	}
	first := code.ExceptionTable[0]
	if first.StartPc != 32 || first.EndPc != 46 || first.HandlerPc != 58 || first.CatchType == 0 {
		t.Fatal("original typed handshake domain changed")
	}
	if code.ExceptionTable[1].StartPc != 32 || code.ExceptionTable[1].EndPc != 46 || code.ExceptionTable[1].HandlerPc != 77 || code.ExceptionTable[1].CatchType != 0 || code.ExceptionTable[2].StartPc != 58 || code.ExceptionTable[2].EndPc != 65 || code.ExceptionTable[2].HandlerPc != 77 || code.ExceptionTable[2].CatchType != 0 {
		t.Fatal("cleanup must protect body and typed failure handler")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
		var source string
		if mode == "legacy" {
			source, err = Decompile(raw)
		} else {
			var result DecompileResult
			result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode})
			source = result.Source
		}
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedControlBody(t, source, `private void handshake\(boolean \w+\)`)
		requireReviewedPattern(t, body, `(?s)\.beginHandshake\(\).*?\.wrapNonAppData\(.*?catch\(Throwable \w+\).*?\.setHandshakeFailure\(.*?finally\{.*?\.forceFlush\(`)
		if strings.Contains(body, DecompileStubMarker) {
			t.Fatal("handshake cannot be a stub")
		}
		if regexp.MustCompile(`(?s)finally\{.*\}\s*return;\s*$`).MatchString(body) {
			t.Fatalf("shared inert RETURN duplicated after terminal recovered finally in %s:\n%s", mode, body)
		}
	}
}
