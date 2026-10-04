package javaclassparser

import "testing"

func TestSentinelsIgnoreSourceData(t *testing.T) {
	for _, s := range []string{`System.out.println("= Exception;");`, `String s="/* yak-decompiler: only data */";`, `/* x = Exception; */ int x=1;`, `String s="\\\"= Exception;";`} {
		if hasExceptionSentinel(s) {
			t.Fatalf("literal/comment treated as code: %s", s)
		}
		if got := fixLeakedExceptionSentinel(s); got != s {
			t.Fatalf("changed source data: %q -> %q", s, got)
		}
	}
	if !hasExceptionSentinel(`Object x = Exception;`) {
		t.Fatal("missed actual sentinel")
	}
}
