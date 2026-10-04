package javaclassparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Parse/decompile only. The independently authored JVM oracles cover runtime
// identity; no third-party library code executes in this native metadata check.
func TestOriginalGroovyCallSiteCheckedEscapeMetadata(t *testing.T) {
	assertReviewedGroovyCheckedEscape(t)
}

func assertReviewedGroovyCheckedEscape(t *testing.T) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	var jars []*zip.ReadCloser
	for _, relative := range []string{"org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/codehaus/groovy/groovy/2.5.14/groovy-2.5.14.jar"} {
		path := filepath.Join(home, ".m2/repository", relative)
		if _, err := os.Stat(path); err != nil {
			t.Skip("native original fixture not installed: " + path)
		}
		jar, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		jars = append(jars, jar)
		defer jar.Close()
	}
	resolve := func(name string) ([]byte, bool) {
		for _, jar := range jars {
			for _, file := range jar.File {
				if file.Name != name+".class" {
					continue
				}
				reader, err := file.Open()
				if err != nil {
					return nil, false
				}
				data, err := io.ReadAll(reader)
				reader.Close()
				return data, err == nil
			}
		}
		return nil, false
	}
	raw, ok := resolve("org/springframework/beans/factory/groovy/GroovyDynamicElementReader")
	if !ok {
		t.Fatal("missing original owner")
	}
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dumper := NewClassObjectDumper(object)
	dumper.foldSiblingResolver = resolve
	provider := dumper.buildInvocationMetadata()
	pool := NewConstantPoolWithConstant(&object.ConstantPool)
	witnessed := false
	for _, method := range object.Methods {
		if pool.GetUtf8(int(method.NameIndex)).Value != "invokeMethod" {
			continue
		}
		exceptions, known := originalMethodExceptions(object, method)
		if !known || len(exceptions) != 0 {
			t.Fatalf("original caller Exceptions %v known%v", exceptions, known)
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(dumper.ConstantPool, index) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			for _, op := range decoder.Opcodes() {
				if op.Instr.OpCode != core.OP_INVOKEINTERFACE {
					continue
				}
				member, ok := GetValueFromCP(dumper.ConstantPool, int(core.Convert2bytesToInt(op.Data[:2]))).(*values.JavaClassMember)
				if !ok {
					continue
				}
				declared, known := exactInvocationExceptions(provider, member.Name, member.Member, member.Description)
				if strings.ReplaceAll(member.Name, ".", "/") == "org/codehaus/groovy/runtime/callsite/CallSite" && known && len(declared) == 1 && declared[0] == "java/lang/Throwable" {
					witnessed = true
				}
			}
		}
	}
	if !witnessed {
		t.Fatal("no original exact invoke + callee throws Throwable witness")
	}
	for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
		var result DecompileResult
		if mode == "legacy" {
			result.Source, err = DecompileWithResolver(raw, resolve)
		} else {
			result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
		}
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedControlBody(t, result.Source, `public Object invokeMethod\(`)
		helper := regexp.MustCompile(`private static <E extends java\.lang\.Throwable> java\.lang\.RuntimeException (jdec\$rethrow\$[0-9]+)\(java\.lang\.Throwable failure\) throws E \{throw \(E\) failure;\}`).FindStringSubmatch(result.Source)
		caught := regexp.MustCompile(`catch \(java\.lang\.Throwable (jdec\$escape\$[0-9]+)\)`).FindStringSubmatch(body)
		if len(helper) != 2 || len(caught) != 2 || !strings.Contains(body, "throw "+helper[1]+"("+caught[1]+");") {
			t.Fatalf("%s original checked escape has no exactly bound identity bridge:\n%s", mode, body)
		}
		if strings.Contains(body, "new RuntimeException(") || strings.Contains(body, "new java.lang.RuntimeException(") {
			t.Fatalf("%s fabricated wrapper changes original Throwable identity:\n%s", mode, body)
		}
	}
}
