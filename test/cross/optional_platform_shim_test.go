package cross

// Optional-platform package completion for the recompile metric (okhttp android.* / org.conscrypt,
// netty-handler conscrypt ALPN engine + jetty ALPN/NPN).
//
// okhttp 3.14.9 faithfully decompiles AndroidPlatform / Android10Platform / ConscryptPlatform, which
// import android.os.Build, android.util.Log, android.annotation.SuppressLint, android.net.ssl.SSLSockets
// and org.conscrypt.Conscrypt. Those packages are NOT on a desktop JDK classpath: android.* is the
// Android SDK, conscrypt is an optional native TLS provider. javac then reports
// `package android.os does not exist` / `package org.conscrypt does not exist` — and, once the import
// of `android.os.Build` fails, `Build.VERSION.SDK_INT` is misread as `package Build does not exist`.
// This is an ENVIRONMENT false-positive that hits EVERY faithful decompiler (CFR/Vineflower emit the
// same imports), the same class as sun.misc.Unsafe (jdk_sunmisc_test.go) and jdk.jfr (jdk_jfr_test.go).
//
// We "complete" the missing packages by compiling a tiny stub tree (the APIs the decompiled sources
// actually reference) into a process-cached temp dir and putting that dir on the classpath. A
// classpath-supplied android.* / org.conscrypt IS honored under `--release 8`. Stubs are prepended
// even when a real conscrypt jar is also on the classpath (the stub signatures are a subset; javac
// only needs the symbols). Extraction/compilation runs once per process; on any failure we silently
// fall back (the false-positive stays).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	optionalPlatformOnce sync.Once
	optionalPlatformDir  string
)

const optionalPlatformSrc = `package android.annotation;
import java.lang.annotation.*;
@Retention(RetentionPolicy.CLASS)
@Target({ElementType.TYPE, ElementType.METHOD, ElementType.CONSTRUCTOR, ElementType.FIELD})
public @interface SuppressLint { String[] value(); }
//---
package android.os;
public class Build {
  public static class VERSION { public static int SDK_INT = 0; }
}
//---
package android.util;
public class Log {
  public static String getStackTraceString(Throwable tr) { return ""; }
  public static int println(int priority, String tag, String msg) { return 0; }
}
//---
package android.net.ssl;
import javax.net.ssl.SSLSocket;
public class SSLSockets {
  public static boolean isSupportedSocket(SSLSocket s) { return false; }
  public static void setUseSessionTickets(SSLSocket s, boolean use) {}
}
//---
package org.conscrypt;
import java.nio.ByteBuffer;
import java.security.Provider;
import javax.net.ssl.SSLEngine;
import javax.net.ssl.SSLEngineResult;
import javax.net.ssl.SSLException;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.SSLSocketFactory;
public class Conscrypt {
  public static ProviderBuilder newProviderBuilder() { return new ProviderBuilder(); }
  public static class ProviderBuilder {
    public ProviderBuilder provideTrustManager() { return this; }
    public Provider build() { return new Provider("Conscrypt", 1.0, "stub") {}; }
  }
  public static boolean isConscrypt(SSLSocket s) { return false; }
  public static boolean isConscrypt(SSLSocketFactory s) { return false; }
  public static boolean isAvailable() { return false; }
  public static void setUseSessionTickets(SSLSocket s, boolean v) {}
  public static void setHostname(SSLSocket s, String h) {}
  public static void setApplicationProtocols(SSLSocket s, String[] p) {}
  public static void setApplicationProtocols(SSLEngine e, String[] p) {}
  public static String getApplicationProtocol(SSLSocket s) { return null; }
  public static String getApplicationProtocol(SSLEngine e) { return null; }
  public static void setUseEngineSocket(SSLSocketFactory f, boolean v) {}
  public static void setBufferAllocator(SSLEngine e, BufferAllocator a) {}
  public static void setHandshakeListener(SSLEngine e, HandshakeListener l) {}
  public static int maxEncryptedPacketLength() { return 0; }
  public static int maxSealOverhead(SSLEngine e) { return 0; }
  public static SSLEngineResult unwrap(SSLEngine e, ByteBuffer[] src, ByteBuffer[] dst) throws SSLException { return null; }
}
//---
package org.conscrypt;
import java.nio.ByteBuffer;
public abstract class AllocatedBuffer {
  public abstract ByteBuffer nioBuffer();
  public abstract AllocatedBuffer retain();
  public abstract AllocatedBuffer release();
}
//---
package org.conscrypt;
public abstract class BufferAllocator {
  public abstract AllocatedBuffer allocateDirectBuffer(int size);
}
//---
package org.conscrypt;
import javax.net.ssl.SSLException;
public abstract class HandshakeListener {
  public abstract void onHandshakeFinished() throws SSLException;
}
//---
package org.eclipse.jetty.alpn;
import java.util.List;
import javax.net.ssl.SSLEngine;
import javax.net.ssl.SSLException;
public class ALPN {
  public interface Provider {}
  public interface ClientProvider extends Provider {
    List<String> protocols();
    void selected(String protocol) throws SSLException;
    void unsupported();
  }
  public interface ServerProvider extends Provider {
    String select(List<String> protocols) throws SSLException;
    void unsupported();
  }
  public static void put(SSLEngine engine, Provider provider) {}
  public static void remove(SSLEngine engine) {}
}
//---
package org.eclipse.jetty.npn;
import java.util.List;
import javax.net.ssl.SSLEngine;
public class NextProtoNego {
  public interface Provider {}
  public interface ClientProvider extends Provider {
    boolean supports();
    void unsupported();
    String selectProtocol(List<String> protocols);
  }
  public interface ServerProvider extends Provider {
    void unsupported();
    List<String> protocols();
    void protocolSelected(String protocol);
  }
  public static void put(SSLEngine engine, Provider provider) {}
  public static void remove(SSLEngine engine) {}
}
//---
package org.zeroturnaround.javarebel;
public interface ClassEventListener {
  void onClassEvent(int eventType, Class cls);
}
//---
package org.zeroturnaround.javarebel;
public interface Reloader {
  void addClassReloadListener(ClassEventListener l);
  void removeClassReloadListener(ClassEventListener l);
}
//---
package org.zeroturnaround.javarebel;
public class ReloaderFactory {
  public static Reloader getInstance() { return null; }
}
//---
package org.apache.log;
public class Logger {
  public void debug(String m) {}
  public void debug(String m, Throwable t) {}
  public void error(String m) {}
  public void error(String m, Throwable t) {}
  public void info(String m) {}
  public void info(String m, Throwable t) {}
  public void warn(String m) {}
  public void warn(String m, Throwable t) {}
  public boolean isDebugEnabled() { return false; }
  public boolean isInfoEnabled() { return false; }
  public boolean isWarnEnabled() { return false; }
  public boolean isErrorEnabled() { return false; }
  public boolean isFatalErrorEnabled() { return false; }
}
//---
package org.apache.log;
public class Hierarchy {
  public static Hierarchy getDefaultHierarchy() { return new Hierarchy(); }
  public Logger getLoggerFor(String name) { return new Logger(); }
}
//---
package com.sun.org.apache.xml.internal.utils;
import org.w3c.dom.Node;
public interface PrefixResolver {
  String getNamespaceForPrefix(String prefix, Node node);
  String getNamespaceForPrefix(String prefix);
  String getBaseIdentifier();
  boolean handlesNullPrefixes();
}
//---
package com.sun.org.apache.xpath.internal;
import com.sun.org.apache.xml.internal.utils.PrefixResolver;
import com.sun.org.apache.xpath.internal.objects.XObject;
import javax.xml.transform.ErrorListener;
import javax.xml.transform.SourceLocator;
import javax.xml.transform.TransformerException;
import org.w3c.dom.Node;
public class XPath {
  public XPath(String expr, SourceLocator loc, PrefixResolver r, int type, ErrorListener el) {}
  public XObject execute(XPathContext ctx, int dtm, PrefixResolver r) throws TransformerException { return null; }
}
//---
package com.sun.org.apache.xpath.internal;
import org.w3c.dom.Node;
public class XPathContext {
  public int getDTMHandleFromNode(Node n) { return 0; }
}
//---
package com.sun.org.apache.xpath.internal.objects;
import org.w3c.dom.traversal.NodeIterator;
public class XObject {
  public NodeIterator nodeset() { return null; }
}
//---
package com.sun.org.apache.xpath.internal.objects;
public class XBoolean extends XObject {
  public boolean bool() { return false; }
}
//---
package com.sun.org.apache.xpath.internal.objects;
public class XNodeSet extends XObject {}
//---
package com.sun.org.apache.xpath.internal.objects;
public class XNull extends XObject {}
//---
package com.sun.org.apache.xpath.internal.objects;
public class XNumber extends XObject {
  public double num() { return 0; }
}
//---
package com.sun.org.apache.xpath.internal.objects;
public class XString extends XObject {}
//---
package org.python.core;
public class PyJavaInstance {
  public Object __tojava__(Class c) { return null; }
}
`

// optionalPlatformClasspathDir compiles the android.* / org.conscrypt stubs into a process-cached
// temp dir and returns that classpath root (or "" if javac is missing / compilation fails).
func optionalPlatformClasspathDir(t *testing.T) string {
	t.Helper()
	optionalPlatformOnce.Do(func() {
		javac, err := exec.LookPath("javac")
		if err != nil {
			return
		}
		dir, err := os.MkdirTemp("", "jdec-optplat-")
		if err != nil {
			return
		}
		srcDir, err := os.MkdirTemp("", "jdec-optplat-src-")
		if err != nil {
			return
		}
		chunks := splitJavaCompilationUnits(optionalPlatformSrc)
		var files []string
		for _, src := range chunks {
			rel, ok := javaPublicUnitPath(src)
			if !ok {
				t.Logf("optional platform stub missing package/public type")
				return
			}
			p := filepath.Join(srcDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return
			}
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				return
			}
			files = append(files, p)
		}
		args := append([]string{"-encoding", "UTF-8", "--release", "8", "-d", dir}, files...)
		cmd := exec.Command(javac, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("optional platform completion unavailable (android.* / conscrypt stay false-positives): %v\n%s", err, out)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "android", "os", "Build.class")); err != nil {
			t.Logf("optional platform completion produced no android.os.Build; leaving unresolved")
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "org", "conscrypt", "Conscrypt.class")); err != nil {
			t.Logf("optional platform completion produced no org.conscrypt.Conscrypt; leaving unresolved")
			return
		}
		if _, err := os.Stat(filepath.Join(dir, "org", "eclipse", "jetty", "alpn", "ALPN.class")); err != nil {
			t.Logf("optional platform completion produced no jetty ALPN; leaving unresolved")
			return
		}
		optionalPlatformDir = dir
	})
	return optionalPlatformDir
}

func splitJavaCompilationUnits(src string) []string {
	var parts []string
	var cur []string
	for _, line := range strings.Split(src, "\n") {
		if line == "//---" {
			if len(cur) > 0 {
				parts = append(parts, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		parts = append(parts, strings.Join(cur, "\n"))
	}
	return parts
}

func javaPublicUnitPath(src string) (string, bool) {
	pkg := ""
	name := ""
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") && strings.HasSuffix(line, ";") {
			pkg = strings.TrimSuffix(strings.TrimPrefix(line, "package "), ";")
			continue
		}
		if strings.HasPrefix(line, "public class ") {
			name = strings.Fields(strings.TrimPrefix(line, "public class "))[0]
			break
		}
		if strings.HasPrefix(line, "public abstract class ") {
			name = strings.Fields(strings.TrimPrefix(line, "public abstract class "))[0]
			break
		}
		if strings.HasPrefix(line, "public @interface ") {
			name = strings.Fields(strings.TrimPrefix(line, "public @interface "))[0]
			break
		}
		if strings.HasPrefix(line, "public interface ") {
			name = strings.Fields(strings.TrimPrefix(line, "public interface "))[0]
			break
		}
	}
	if pkg == "" || name == "" {
		return "", false
	}
	return strings.ReplaceAll(pkg, ".", "/") + "/" + name + ".java", true
}

// withOptionalPlatforms prepends the android.* / org.conscrypt stub classpath root (no-op when
// unavailable or empty). Prepending is harmless for jars that do not import those packages.
func withOptionalPlatforms(t *testing.T, classpath string) string {
	d := optionalPlatformClasspathDir(t)
	if d == "" {
		return classpath
	}
	if classpath == "" {
		return d
	}
	return d + string(os.PathListSeparator) + classpath
}

// withEnvShims prepends every environment-false-positive package completion the harness uses
// (android/conscrypt stubs, java.util.concurrent.Flow, jdk.jfr, sun.misc). Harmless for jars that
// do not import them.
func withEnvShims(t *testing.T, classpath string) string {
	return withOptionalPlatforms(t, withFlow(t, withJfr(t, withSunMisc(t, withSunX509(t, withJdi(t, classpath))))))
}

func TestOptionalPlatformIncludesNettyStubs(t *testing.T) {
	d := optionalPlatformClasspathDir(t)
	if d == "" {
		t.Fatal("optional platform stubs failed to compile")
	}
	for _, rel := range []string{
		"org/conscrypt/Conscrypt.class",
		"org/conscrypt/AllocatedBuffer.class",
		"org/conscrypt/BufferAllocator.class",
		"org/conscrypt/HandshakeListener.class",
		"org/eclipse/jetty/alpn/ALPN.class",
		"org/eclipse/jetty/npn/NextProtoNego.class",
		"org/zeroturnaround/javarebel/ReloaderFactory.class",
		"org/apache/log/Hierarchy.class",
		"com/sun/org/apache/xpath/internal/XPath.class",
		"org/python/core/PyJavaInstance.class",
	} {
		if _, err := os.Stat(filepath.Join(d, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing stub %s: %v", rel, err)
		}
	}
}
