package javaclassparser

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestHttpclientBrowserCompatSpecSuperArrayIsLoadBearing(t *testing.T) {
	path := "testdata/regression/BrowserCompatSpec.class"
	descriptor := "([Ljava/lang/String;Lorg/apache/http/impl/cookie/BrowserCompatSpecFactory$SecurityLevel;)V"
	raw, code, _ := reviewedFixtureMethod(t, path, "<init>", descriptor)
	assertReviewedOpcode(t, code, 1, core.OP_BIPUSH, 7)
	assertReviewedOpcode(t, code, 3, core.OP_ANEWARRAY)
	assertReviewedOpcode(t, code, 6, core.OP_DUP)
	for _, pc := range []uint16{15, 25, 52, 62, 72, 82, 110} {
		assertReviewedOpcode(t, code, pc, core.OP_AASTORE)
	}
	assertReviewedTypeVarInvoke(t, path, "<init>", descriptor, 111, core.OP_INVOKESPECIAL, "org/apache/http/impl/cookie/CookieSpecBase", "<init>", "([Lorg/apache/http/cookie/CommonCookieAttributeHandler;)V")
	resolver := reviewedBrowserDelegationMetadata(t)
	var on string
	for _, flag := range []string{"", "1"} {
		t.Setenv("JDEC_HTTPCLIENT_REMAINING_OFF", flag)
		source, err := DecompileWithResolver(raw, resolver)
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedSourceMethod(t, source, `public\s+BrowserCompatSpec\(String\[\]\s+\w+,\s*BrowserCompatSpecFactory\$SecurityLevel\s+\w+\)`)
		pattern := `\)\s*\{\s*super\(new CommonCookieAttributeHandler\[\]\{new BrowserCompatVersionAttributeHandler\(\),\s*new BasicDomainHandler\(\),[^;]*new BrowserCompatSpec\$1\(\)[^;]*new BasicPathHandler\(\)[^;]*,\s*new BasicMaxAgeHandler\(\),\s*new BasicSecureHandler\(\),\s*new BasicCommentHandler\(\),\s*new BasicExpiresHandler\([^;]*\.clone\(\)[^;]*DEFAULT_DATE_PATTERNS[^;]*\}\);\s*\}`
		requireReviewedPattern(t, body, pattern)
		if strings.Count(body, "super(") != 1 || strings.Contains(body, "Object[]") {
			t.Fatal("delegation or runtime array component changed")
		}
		if flag == "" {
			on = source
		} else if source != on {
			t.Fatal("retired spelling patch still changes a proven constructor")
		}
	}
	missing := func(name string) ([]byte, bool) {
		if name == "org/apache/http/impl/cookie/BasicPathHandler" {
			return nil, false
		}
		return resolver(name)
	}
	t.Setenv("JDEC_HTTPCLIENT_REMAINING_OFF", "1")
	var incomplete string
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_CTOR_ARRAY_ARG_INLINE_OFF", setting)
		source, err := DecompileWithResolver(raw, missing)
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedSourceMethod(t, source, `public\s+BrowserCompatSpec\(String\[\]\s+\w+,\s*BrowserCompatSpecFactory\$SecurityLevel\s+\w+\)`)
		if strings.Contains(body, "super(new CommonCookieAttributeHandler[]{") || !strings.Contains(body, "new CommonCookieAttributeHandler[7]") {
			t.Fatal("missing parent proof guessed a folded initializer")
		}
		if setting == "" {
			incomplete = source
		} else if incomplete != source {
			t.Fatal("failed ownership plan mutated later decompilation")
		}
	}
	// The algorithmic negative remains active. Missing private ownership proof
	// leaves the array declaration/stores in place rather than guessing a literal.
	t.Setenv("JDEC_CTOR_ARRAY_ARG_INLINE_OFF", "1")
	off, err := DecompileWithResolver(raw, resolver)
	if err != nil {
		t.Fatal(err)
	}
	body := reviewedSourceMethod(t, off, `public\s+BrowserCompatSpec\(String\[\]\s+\w+,\s*BrowserCompatSpecFactory\$SecurityLevel\s+\w+\)`)
	if strings.Contains(body, "super(new CommonCookieAttributeHandler[]{") || !strings.Contains(body, "new CommonCookieAttributeHandler[7]") {
		t.Fatal("algorithmic negative lost its missing-delegation counterexample")
	}
}

// The structured switch now retains the shared continuation and its actual
// returns. No guessed return before the catch is required (A15 semantic family).
func TestHttpclientAuthenticatorContinuationRetired(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/HttpAuthenticator.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"", "1"} {
		t.Setenv("JDEC_HTTPCLIENT_REMAINING_OFF", flag)
		src, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"var3.select(var6,var1,var2,var5)", "var7.processChallenge(var8)", "var4.update(var8_1)", "return true;", "return false;"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing real control flow %s", want)
			}
		}
		if strings.Contains(src, "Decompilation failed") {
			t.Fatal("stub hid switch failure")
		}
	}
}
