package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestLeakedCatchFromSiblingIsLoadBearing(t *testing.T) {
	in := "" +
		"\t\tif ((var2) >= (0)){\n" +
		"\t\t\tif ((var5) >= (1)){\n" +
		"\t\t\t\tvar6 = ByteBuffer.allocate(16);\n" +
		"\t\t\t\tNumberFormatException var7 = Exception;\n" +
		"\t\t\t\treturn null;\n" +
		"\t\t\t}\n" +
		"\t\t}else{\n" +
		"\t\t\tif ((var5) != (0)){\n" +
		"\t\t\t\treturn null;\n" +
		"\t\t\t}else{\n" +
		"\t\t\t\tvar6 = ByteBuffer.allocate(16);\n" +
		"\t\t\t\ttry{\n" +
		"\t\t\t\t\tvar8 = 0;\n" +
		"\t\t\t\t\tvar6.putShort(parseHextet(s));\n" +
		"\t\t\t\t\treturn var6.array();\n" +
		"\t\t\t\t}catch(NumberFormatException var7){\n" +
		"\t\t\t\t\treturn null;\n" +
		"\t\t\t\t}\n" +
		"\t\t\t}\n" +
		"\t\t}\n"

	os.Unsetenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF")
	on := fixLeakedExceptionSentinel(in)
	if strings.Contains(on, "= Exception") {
		t.Errorf("fix ON: leaked sentinel survived:\n%s", on)
	}
	if !strings.Contains(on, "parseHextet") || strings.Count(on, "catch(NumberFormatException") < 2 {
		t.Errorf("fix ON: expected sibling try copied into leaked arm, got:\n%s", on)
	}

	t.Setenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF", "1")
	off := fixLeakedExceptionSentinel(in)
	if !strings.Contains(off, "= Exception") {
		t.Errorf("fix OFF: expected leaked sentinel to remain, got:\n%s", off)
	}
}

func TestLeakedTryLockFinallyIsLoadBearing(t *testing.T) {
	in := "" +
		"\t\t\t\tif (var5.tryLock()){\n" +
		"\t\t\t\t\tvar8 = false;\n" +
		"\t\t\t\t\tvar9 = 1;\n" +
		"\t\t\t\t\tvar10 = Exception;\n" +
		"\t\t\t\t\tif (!(var8)){};\n" +
		"\t\t\t\t\tvar11 = Exception;\n" +
		"\t\t\t\t\tvar5.unlock();\n" +
		"\t\t\t\t\tthrow var11;\n" +
		"\t\t\t\t}\n" +
		"\t\t\t}\n" +
		"\t\t}\n" +
		"\t\tvar7 = initNanoTime(var4);\n" +
		"\t\tif (!(var5.tryLock(var2,var3))){\n" +
		"\t\t\treturn false;\n" +
		"\t\t}else{\n" +
		"\t\t\tvar8 = false;\n" +
		"\t\t\tvar9 = 1;\n" +
		"\t\t\ttry{\n" +
		"\t\t\t\tvar8 = (var1.isSatisfied()) || (this.awaitNanos(var1,var4,var6));\n" +
		"\t\t\t\treturn var8;\n" +
		"\t\t\t}catch(Throwable var10_1){\n" +
		"\t\t\t\tvar5.unlock();\n" +
		"\t\t\t\ttry{\n" +
		"\t\t\t\t}catch(Throwable var11_1){\n" +
		"\t\t\t\t\tthrow var11;\n" +
		"\t\t\t\t}\n" +
		"\t\t\t\tthrow var10;\n" +
		"\t\t\t}\n" +
		"\t\t}\n"

	os.Unsetenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF")
	on := fixLeakedExceptionSentinel(in)
	if strings.Contains(on, "= Exception") {
		t.Errorf("fix ON: leaked sentinel survived:\n%s", on)
	}
	if strings.Count(on, "awaitNanos") < 2 {
		t.Errorf("fix ON: expected tryLock success arm to copy awaitNanos try, got:\n%s", on)
	}
	if strings.Contains(on, "throw var10;") || strings.Contains(on, "throw var11;") {
		t.Errorf("fix ON: expected throw var10_1/var11_1, got:\n%s", on)
	}
	if !strings.Contains(on, "throw var10_1;") || !strings.Contains(on, "throw var11_1;") {
		t.Errorf("fix ON: missing throw var10_1/var11_1:\n%s", on)
	}

	t.Setenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF", "1")
	off := fixLeakedExceptionSentinel(in)
	if !strings.Contains(off, "= Exception") {
		t.Errorf("fix OFF: expected leaked sentinel to remain, got:\n%s", off)
	}
}

func TestLeakedExceptionSentinelDecompileIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ExceptionSentinelSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF")
	os.Unsetenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if strings.Contains(on, "= Exception;") || strings.Contains(on, "yak-decompiler:") {
		t.Errorf("fix ON: enterWhen should reconstruct, not stub/leak:\n%s", on)
	}
	if !strings.Contains(on, "awaitNanos") || !strings.Contains(on, "tryLock") {
		t.Errorf("fix ON: expected reconstructed enterWhen body with awaitNanos/tryLock, got:\n%s", on)
	}

	t.Setenv("JDEC_LEAKED_EXCEPTION_SENTINEL_OFF", "1")
	t.Setenv("JDEC_EXCEPTION_SENTINEL_DEGRADE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "= Exception;") {
		t.Errorf("fix OFF: expected leaked `= Exception;` to reappear, got:\n%s", off)
	}
}
