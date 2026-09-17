package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestHttpclientBrowserCompatSpecSuperArrayIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BrowserCompatSpec.class", "JDEC_HTTPCLIENT_REMAINING_OFF",
		"super(new CommonCookieAttributeHandler[]{",
		"super(var3);")
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
