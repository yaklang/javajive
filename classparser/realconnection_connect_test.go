package javaclassparser

// 承重测试: okhttp RealConnection.connect 的 try 体被掏成 `if(false) throw IOException;
// break;`, connectTunnel/connectSocket 落到 try 外, javac 报 unreported IOException。
// kill-switch: JDEC_REALCONNECTION_CONNECT_OFF。

import (
	"strings"
	"testing"
)

func TestRealConnectionConnectIsLoadBearing(t *testing.T) {
	path := "testdata/regression/RealConnection.class"
	descriptor := "(IIIIZLokhttp3/Call;Lokhttp3/EventListener;)V"
	raw, code, object := reviewedFixtureMethod(t, path, "connect", descriptor)
	assertReviewedTypeVarInvoke(t, path, "connect", descriptor, 211, 183, "okhttp3/internal/connection/RealConnection", "connectTunnel", "(IIILokhttp3/Call;Lokhttp3/EventListener;)V")
	assertReviewedTypeVarInvoke(t, path, "connect", descriptor, 231, 183, "okhttp3/internal/connection/RealConnection", "connectSocket", "(IILokhttp3/Call;Lokhttp3/EventListener;)V")
	assertReviewedTypeVarInvoke(t, path, "connect", descriptor, 243, 183, "okhttp3/internal/connection/RealConnection", "establishProtocol", "(Lokhttp3/internal/connection/ConnectionSpecSelector;ILokhttp3/Call;Lokhttp3/EventListener;)V")
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	if len(code.ExceptionTable) < 2 {
		t.Fatal("original retry handler missing")
	}
	for i, row := range [][3]uint16{{193, 221, 274}, {224, 271, 274}} {
		handler := code.ExceptionTable[i]
		if handler.StartPc != row[0] || handler.EndPc != row[1] || handler.HandlerPc != row[2] || cp.GetClassName(int(handler.CatchType)) != "java/io/IOException" {
			t.Fatal("connection call exception ranges changed")
		}
	}
	reviewedOriginalFamilySources(t, raw, "com/squareup/okhttp3/okhttp/3.14.9/okhttp-3.14.9.jar", "JDEC_REALCONNECTION_CONNECT_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `void\s+connect\(`)
		requireReviewedPattern(t, body, `try\{(?:\s*if\(false\)throw new IOException\(\);)?\s*if\s*\([^{}]*requiresTunnel\(\)[^{}]*\)\{[^{}]*this\.connectTunnel\([^;]*;\s*if\s*\(\(this\.rawSocket\)\s*==\s*\(null\)\)\{\s*break;\s*\}\s*\}else\{[^{}]*this\.connectSocket\([^;]*;[^{}]*\}\s*this\.establishProtocol\([^;]*;[^{}]*connectEnd\([^;]*;[^{}]*break;\s*\}catch\(IOException\s+\w+\)`)
		if !strings.Contains(body, ".addConnectException(") || !strings.Contains(body, "this.socket = null;") || !strings.Contains(body, "this.protocol = null;") {
			t.Fatal("retry catch lost original accumulated failure or state cleanup")
		}
	})
}
