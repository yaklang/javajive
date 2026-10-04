package javaclassparser

// 承重测试: okhttp PublicSuffixDatabase.findMatchingRule 空 sync 缺 return, 以及
// readTheListUninterruptibly 的 readTheList() 落到 IOException try 外。
// kill-switch: JDEC_PUBLIC_SUFFIX_OFF。

import (
	"os"
	"strings"
	"testing"
)

// Disabling the library-specific findMatchingRule workaround must not disable
// generic retry reconstruction. The operation stays inside its protected loop
// with either setting; successful and failing retries also have a JVM oracle.
func TestPublicSuffixDatabaseUsesStructuredRetry(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/PublicSuffixDatabase.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_PUBLIC_SUFFIX_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		body := retryMethodSource(t, source, "void readTheListUninterruptibly(")
		compact := strings.Join(strings.Fields(body), "")
		// The legacy no-resolver entry point can retain this checked-catch
		// sentinel; it must not stand in for the real read operation.
		compact = strings.ReplaceAll(compact, "if(false)thrownewIOException();", "")
		for _, required := range []string{
			"do{try{this.readTheList();", "catch(InterruptedIOException",
			"Thread.interrupted();", "catch(IOException", "catch(Throwable",
			"Thread.currentThread().interrupt();", "while(true)",
		} {
			if !strings.Contains(compact, required) {
				t.Errorf("switch=%q missing %q in retry loop:\n%s", setting, required, body)
			}
		}
		if strings.Count(compact, "this.readTheList();") != 1 || strings.Contains(compact, "try{break;") {
			t.Errorf("switch=%q lost or duplicated the protected operation:\n%s", setting, body)
		}
	}
}
