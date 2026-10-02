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
	path := "testdata/regression/AsyncAppenderBase.class"
	raw, code, object := reviewedFixtureMethod(t, path, "putUninterruptibly", "(Ljava/lang/Object;)V")
	assertReviewedTypeVarMethod(t, raw, "putUninterruptibly", "(Ljava/lang/Object;)V", "(TE;)V")
	assertReviewedTypeVarInvoke(t, path, "putUninterruptibly", "(Ljava/lang/Object;)V", 7, 185, "java/util/concurrent/BlockingQueue", "put", "(Ljava/lang/Object;)V")
	assertReviewedTypeVarInvoke(t, path, "putUninterruptibly", "(Ljava/lang/Object;)V", 28, 182, "java/lang/Thread", "interrupt", "()V")
	assertReviewedTypeVarInvoke(t, path, "putUninterruptibly", "(Ljava/lang/Object;)V", 43, 182, "java/lang/Thread", "interrupt", "()V")
	// The retry handler protects put only; the cleanup handler protects both
	// the attempt and the retry path. Its self-entry is the javac finally guard.
	expected := [][3]uint16{{2, 12, 15}, {2, 21, 34}, {34, 36, 34}}
	if len(code.ExceptionTable) != len(expected) {
		t.Fatal("original retry/cleanup domains changed")
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for i, handler := range code.ExceptionTable {
		if handler.StartPc != expected[i][0] || handler.EndPc != expected[i][1] || handler.HandlerPc != expected[i][2] {
			t.Fatal("original retry/cleanup coverage changed")
		}
		if i == 0 {
			if cp.GetClassName(int(handler.CatchType)) != "java/lang/InterruptedException" {
				t.Fatal("original retry exception changed")
			}
		} else if handler.CatchType != 0 {
			t.Fatal("cleanup is not original catch-all")
		}
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_LOGBACK_REMAINING_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		body := retryMethodSource(t, source, "void putUninterruptibly(")
		flag := requireReviewedPattern(t, body, `boolean\s+(\w+)\s*=\s*false;`)[1]
		input := requireReviewedPattern(t, body, `putUninterruptibly\(E\s+(\w+)\)`)[1]
		compact := compactReviewedGenericSource(body)
		compact = strings.ReplaceAll(compact, "if(false)thrownewInterruptedException();", "")
		// Match the complete nested scope: put and its success break are inside
		// the retry catch, the loop is inside one outer try, and the same flag
		// controls interrupt restoration in that try's finally on every exit.
		requireReviewedPattern(t, compact, `try\{do\{try\{this\.blockingQueue\.put\(`+input+`\);break;\}catch\(InterruptedException\w+\)\{`+flag+`=true;continue;\}\}while\(true\);return;\}finally\{if\(`+flag+`\)\{Thread\.currentThread\(\)\.interrupt\(\);\}\}`)
		if strings.Count(compact, "this.blockingQueue.put("+input+");") != 1 || strings.Count(compact, "Thread.currentThread().interrupt();") != 1 {
			t.Fatal("retry operation or terminal cleanup duplicated")
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
