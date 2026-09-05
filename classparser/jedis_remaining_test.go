package javaclassparser

// 承重测试: jedis 剩余 dump 重构 (getKeys 数组造型、JedisByteHashMap put、catch 重复声明).
// kill-switch: JDEC_JEDIS_REMAINING_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestJedisRemainingReconstructsAreLoadBearing(t *testing.T) {
	in := strings.Join([]string{
		"return ((List)(new BinaryJedisCluster$240(this,this.connectionHandler,this.maxAttempts,this.maxTotalRetriesDuration,var1,var2).runBinary(var2.length,getKeys((Map.Entry)(var2)))));",
		"return ((List)(new BinaryJedisCluster$247(this,this.connectionHandler,this.maxAttempts,this.maxTotalRetriesDuration,var1,var2,var3,var4).runBinary(var4.length,getKeys((Map.Entry)(var4)))));",
		"JedisByteHashMap var3 = new JedisByteHashMap();",
		"var3.put(var4.next(),var4.next());",
		"}catch(JedisConnectionException var4){",
		"			JedisConnectionException var4 = null;",
		"			try{",
	}, "\n")

	os.Unsetenv("JDEC_JEDIS_REMAINING_OFF")
	on := fixJedisRemainingReconstructs(in)
	if strings.Contains(on, "getKeys((Map.Entry)(var2))") || strings.Contains(on, "getKeys((Map.Entry)(var4))") {
		t.Errorf("ON: expected getKeys array wrap dropped, got:\n%s", on)
	}
	if !strings.Contains(on, "getKeys(var2)") || !strings.Contains(on, "getKeys(var4)") {
		t.Errorf("ON: missing getKeys(varN), got:\n%s", on)
	}
	if !strings.Contains(on, "var3.put((byte[])(var4.next()),(byte[])(var4.next()))") {
		t.Errorf("ON: missing JedisByteHashMap byte[] casts, got:\n%s", on)
	}
	if strings.Contains(on, "JedisConnectionException var4 = null;") {
		t.Errorf("ON: expected catch redeclare dropped, got:\n%s", on)
	}

	t.Setenv("JDEC_JEDIS_REMAINING_OFF", "1")
	off := fixJedisRemainingReconstructs(in)
	if off != in {
		t.Errorf("OFF: expected identity, got:\n%s", off)
	}
}

func TestJedisRemainingGetKeysIsLoadBearing(t *testing.T) {
	assertJedisDecompileDiff(t, "testdata/regression/BinaryJedisCluster.class",
		"getKeys(var2)", "getKeys((Map.Entry)(var2))")
}

func TestJedisRemainingByteHashMapPutIsLoadBearing(t *testing.T) {
	assertJedisDecompileDiff(t, "testdata/regression/BuilderFactory$15.class",
		"(byte[])(var4.next())", "var3.put(var4.next(),var4.next())")
}

func TestJedisRemainingCatchRedeclareIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/Connection.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_JEDIS_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "JedisConnectionException var4 = null;") {
		t.Errorf("ON still redeclares catch var4")
	}
	t.Setenv("JDEC_JEDIS_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "JedisConnectionException var4 = null;") {
		t.Errorf("OFF missing catch redeclare")
	}
}

func assertJedisDecompileDiff(t *testing.T, seed, onMust, offMust string) {
	t.Helper()
	raw, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_JEDIS_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON decompile: %v", err)
	}
	if !strings.Contains(on, onMust) {
		t.Errorf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv("JDEC_JEDIS_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF decompile: %v", err)
	}
	if !strings.Contains(off, offMust) {
		t.Errorf("OFF missing %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical; reconstruct is not load-bearing")
	}
}

func clipForTest(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		if len(s) > 800 {
			return s[:800]
		}
		return s
	}
	a := i - 120
	if a < 0 {
		a = 0
	}
	b := i + len(needle) + 120
	if b > len(s) {
		b = len(s)
	}
	return s[a:b]
}
