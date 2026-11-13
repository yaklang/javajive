package dex2jar

import "testing"

func TestAppendConversionDiagnosticIsBounded(t *testing.T) {
	detailed := DetailedReport{}
	for index := 0; index < maxConversionDiagnostics*4; index++ {
		appendConversionDiagnostic(&detailed, ConversionDiagnostic{Message: "diagnostic"})
	}
	if got := len(detailed.Diagnostics); got != maxConversionDiagnostics {
		t.Fatalf("diagnostic count = %d, want %d", got, maxConversionDiagnostics)
	}
	last := detailed.Diagnostics[len(detailed.Diagnostics)-1]
	if last.Code != "diagnostics.truncated" {
		t.Fatalf("last diagnostic = %#v", last)
	}
}

func BenchmarkAppendConversionDiagnosticBounded(b *testing.B) {
	diagnostic := ConversionDiagnostic{Message: "diagnostic"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		detailed := DetailedReport{}
		for index := 0; index < maxConversionDiagnostics*4; index++ {
			appendConversionDiagnostic(&detailed, diagnostic)
		}
	}
}
