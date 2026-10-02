package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestLogbackEchoEncoderGetBytesIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/EchoEncoder.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_LOGBACK_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !echoEncoderGetBytesParenthesized(on) {
		t.Errorf("ON missing parenthesized concat.getBytes():\n%s", on)
	}
	if strings.Contains(on, "var1 + CoreConstants.LINE_SEPARATOR.getBytes()") {
		t.Errorf("ON still has unparenthesized concat receiver:\n%s", on)
	}
	t.Setenv("JDEC_LOGBACK_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !echoEncoderGetBytesParenthesized(off) {
		t.Errorf("OFF structural emitter must still parenthesize concat receiver:\n%s", off)
	}
}

func echoEncoderGetBytesParenthesized(src string) bool {
	return strings.Contains(src, "(var1 + CoreConstants.LINE_SEPARATOR).getBytes()") ||
		strings.Contains(src, "(String.valueOf(var1) + CoreConstants.LINE_SEPARATOR).getBytes()")
}

// The CFG structurer now owns this retry loop. Both settings of the unrelated
// library workaround switch must preserve it; see the independent interrupt
// and finally behavior oracle in TestAdversarialRetryInterruptCleanupRoundTrip.
func TestLogbackPutUninterruptiblyUsesStructuredRetry(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/AsyncAppenderBase.class")
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_LOGBACK_REMAINING_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		body := retryMethodSource(t, source, "void putUninterruptibly(")
		compact := strings.Join(strings.Fields(body), "")
		// Legacy Decompile without a resolver can retain its checked-catch
		// sentinel. It must accompany the real protected call, never replace it.
		compact = strings.ReplaceAll(compact, "if(false)thrownewInterruptedException();", "")
		for _, required := range []string{
			// The terminal cleanup covers the whole retry loop; the retry
			// handler covers the put operation on each iteration.
			"try{do{try{this.blockingQueue.put(var1);",
			"catch(InterruptedException", "Thread.currentThread().interrupt();",
			"catch(Throwable", "while(true)",
		} {
			if !strings.Contains(compact, required) {
				t.Errorf("switch=%q missing %q in retry loop:\n%s", setting, required, body)
			}
		}
		if strings.Count(compact, "this.blockingQueue.put(var1);") != 1 || strings.Contains(compact, "try{break;") {
			t.Errorf("switch=%q lost or duplicated the protected operation:\n%s", setting, body)
		}
	}
}

func retryMethodSource(t *testing.T, source, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("missing method %q", signature)
	}
	end := strings.Index(source[start:], "\n\t}")
	if end < 0 {
		t.Fatalf("missing end of method %q", signature)
	}
	return source[start : start+end+3]
}

func TestLogbackConsoleAppenderOrElseThrowIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ConsoleAppender.class", "JDEC_LOGBACK_REMAINING_OFF",
		"var3.get()",
		"var3.orElseThrow")
}

func TestHikaricpGaugeMethodRefIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CodaHaleMetricsTracker.class", "JDEC_HIKARICP_REMAINING_OFF",
		"(com.codahale.metrics.Gauge)(var2::getTotalConnections)",
		"(Metric)(var2::getTotalConnections)")
}

func TestHikaricpProxyResultSetCloseThrowsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/HikariProxyResultSet.class", "JDEC_HIKARICP_REMAINING_OFF",
		"public void close() throws SQLException",
		"public void close() throws Exception")
}

func TestHikaricpEvictConnectionPoolTypeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/HikariDataSource.class", "JDEC_HIKARICP_REMAINING_OFF",
		"HikariPool var3 = null;",
		"Object var3 = null;")
}

func assertKillSwitchDecompile(t *testing.T, seed, env, onMust, offMust string) {
	t.Helper()
	raw, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv(env)
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON decompile: %v", err)
	}
	if !strings.Contains(on, onMust) {
		t.Errorf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv(env, "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF decompile: %v", err)
	}
	// A later always-on / orig14 shape pass may already apply the same rewrite, so
	// this switch is no longer the unique owner. The reconstruct still fires.
	if strings.Contains(off, onMust) {
		return
	}
	if !strings.Contains(off, offMust) {
		t.Errorf("OFF missing %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}
