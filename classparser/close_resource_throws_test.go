package javaclassparser

// 承重测试: javac 9-13 TWR 合成方法 `$closeResource(Throwable, AutoCloseable)` 的 else
// 分支 `var1.close()` 是 AutoCloseable.close(), 受检异常 Exception。源码合成方法不写
// throws (javac 内部生成)。反编译后声明 `throws IOException` 并把 else 收到
// Closeable.close() 上, 与 bytes()/string() 等已声明 throws IOException 的调用点对齐。
// 镜像 okhttp ResponseBody / DiskLruCache。kill-switch: JDEC_CLOSE_RESOURCE_THROWS_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestCloseResourceThrowsIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ResponseBody.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CLOSE_RESOURCE_THROWS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "static void $closeResource(Throwable var0, AutoCloseable var1) throws java.io.IOException {") {
		t.Errorf("fix ON: expected `$closeResource(...) throws java.io.IOException`, got:\n%s", on)
	}
	if !strings.Contains(on, "((java.io.Closeable)(var1)).close()") {
		t.Errorf("fix ON: expected else-branch Closeable.close(), got:\n%s", on)
	}

	t.Setenv("JDEC_CLOSE_RESOURCE_THROWS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "$closeResource(Throwable var0, AutoCloseable var1) throws") {
		t.Errorf("fix OFF: expected no throws on $closeResource, got:\n%s", off)
	}
	if !strings.Contains(off, "static void $closeResource(Throwable var0, AutoCloseable var1) {") {
		t.Errorf("fix OFF: expected the no-throws $closeResource declaration, got:\n%s", off)
	}
	if strings.Contains(off, "((java.io.Closeable)(var1)).close()") {
		t.Errorf("fix OFF: expected the bare AutoCloseable.close(), got:\n%s", off)
	}
}
