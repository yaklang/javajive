package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestLogbackEchoEncoderGetBytesIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/EchoEncoder.class", "JDEC_LOGBACK_REMAINING_OFF",
		"(String.valueOf(var1) + CoreConstants.LINE_SEPARATOR).getBytes()",
		`"" + String.valueOf(var1) + "" + CoreConstants.LINE_SEPARATOR.getBytes()`)
}

func TestLogbackPutUninterruptiblyIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/AsyncAppenderBase.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_LOGBACK_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "try{\nbreak;\n}catch(InterruptedException") {
		t.Errorf("ON still has empty try/break IE catch")
	}
	if !strings.Contains(on, "this.blockingQueue.put(var1)") {
		t.Errorf("ON missing blockingQueue.put")
	}
	t.Setenv("JDEC_LOGBACK_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
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
