package javaclassparser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Metadata and original exception ranges are independent evidence. Neither
// historical classes nor their regenerated source is executed on the host.
func reviewedControlCode(t *testing.T, raw []byte, name, descriptor string, tables []string) (*CodeAttribute, *ClassObject) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, method := range object.Methods {
		if cp.GetUtf8(int(method.NameIndex)).Value != name || cp.GetUtf8(int(method.DescriptorIndex)).Value != descriptor {
			continue
		}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			got := make([]string, 0, len(code.ExceptionTable))
			for _, handler := range code.ExceptionTable {
				catch := "*"
				if handler.CatchType != 0 {
					catch = cp.GetClassName(int(handler.CatchType))
				}
				got = append(got, fmt.Sprintf("%d:%d:%d:%s", handler.StartPc, handler.EndPc, handler.HandlerPc, catch))
			}
			if strings.Join(got, "\n") != strings.Join(tables, "\n") {
				t.Fatalf("original %s%s exception regions changed: %v != %v", name, descriptor, got, tables)
			}
			return code, object
		}
	}
	t.Fatalf("original method %s%s missing", name, descriptor)
	return nil, nil
}

func reviewedControlInvokes(t *testing.T, code *CodeAttribute, object *ClassObject, wanted ...reviewedViewInvoke) {
	t.Helper()
	d := core.NewDecompiler(code.Code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	found := make([]bool, len(wanted))
	for _, op := range d.Opcodes() {
		if len(op.Data) < 2 {
			continue
		}
		switch op.Instr.OpCode {
		case core.OP_INVOKEVIRTUAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE, core.OP_INVOKESPECIAL, core.OP_GETFIELD, core.OP_GETSTATIC, core.OP_PUTFIELD, core.OP_PUTSTATIC:
		default:
			continue
		}
		index := int(op.Data[0])<<8 | int(op.Data[1])
		var member ConstantMemberrefInfo
		switch reference := cp.IndexInfo(index).(type) {
		case *ConstantMethodrefInfo:
			member = reference.ConstantMemberrefInfo
		case *ConstantInterfaceMethodrefInfo:
			member = reference.ConstantMemberrefInfo
		case *ConstantFieldrefInfo:
			member = reference.ConstantMemberrefInfo
		default:
			continue
		}
		pair := cp.IndexInfo(int(member.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		for i, call := range wanted {
			if op.Instr.OpCode == call.opcode && cp.GetClassName(int(member.ClassIndex)) == call.owner && cp.GetUtf8(int(pair.NameIndex)).Value == call.name && cp.GetUtf8(int(pair.DescriptorIndex)).Value == call.descriptor {
				found[i] = true
			}
		}
	}
	for i, call := range wanted {
		if !found[i] {
			t.Fatalf("original method instruction missing %+v", call)
		}
	}
}

func reviewedControlBody(t *testing.T, source, declaration string) string {
	t.Helper()
	match := regexp.MustCompile(declaration).FindStringIndex(source)
	if match == nil {
		t.Fatalf("missing declaration %s\n%s", declaration, source)
	}
	open := javaIndexBraceFrom(source, match[0])
	if open < 0 {
		t.Fatal("member body absent")
	}
	end := javaMatchBrace(source, open)
	if end < 0 {
		t.Fatal("member body unclosed")
	}
	return source[match[0] : end+1]
}

func reviewedControlJar(t *testing.T, jar, entry, method, descriptor string, tables []string, calls []reviewedViewInvoke, check func(string)) {
	t.Helper()
	raw := originalJarClassForReview(t, jar, entry)
	code, object := reviewedControlCode(t, raw, method, descriptor, tables)
	reviewedControlInvokes(t, code, object, calls...)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		fs, err := NewJarFSFromLocal(filepath.Join(home, ".m2/repository", jar))
		if err != nil {
			t.Fatal(err)
		}
		text, err := fs.ReadFile(entry)
		fs.Close()
		if err != nil {
			t.Fatal(err)
		}
		check(string(text))
	}
}

func assertReviewedCreationReflection(t *testing.T) {
	reviewedControlJar(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/dynamic/loading/ByteArrayClassLoader$SynchronizationStrategy$CreationAction.class", "run", "()Lnet/bytebuddy/dynamic/loading/ByteArrayClassLoader$SynchronizationStrategy$Initializable;",
		[]string{"0:145:146:java/lang/Exception", "0:145:203:java/lang/Exception", "146:202:203:java/lang/Exception"},
		[]reviewedViewInvoke{{"java/lang/Class", "getDeclaredMethod", "(Ljava/lang/String;[Ljava/lang/Class;)Ljava/lang/reflect/Method;", core.OP_INVOKEVIRTUAL}}, func(source string) {
			body := reviewedControlBody(t, source, `public\s+ByteArrayClassLoader\$SynchronizationStrategy\$Initializable\s+run\(\)`)
			// The fallback reflection call must be covered by the outer handler;
			// adding a catch around an empty sibling block proves nothing.
			outer := requireReviewedPattern(t, body, `(?s)try\s*\{\s*try\s*\{.*?\}\s*catch\(Exception\s+\w+\)\s*\{.*?getDeclaredMethod\("getClassLoadingLock".*?\}\s*\}\s*catch\(Exception\s+\w+\)\s*\{\s*return\s+ByteArrayClassLoader\$SynchronizationStrategy\$ForLegacyVm\.INSTANCE;`)
			_ = outer
		})
}

func assertReviewedConstructorReflectionInitializer(t *testing.T) {
	reviewedControlJar(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/bytecode/constant/MethodConstant$ForConstructor.class", "<clinit>", "()V", []string{"0:52:55:java/lang/NoSuchMethodException"},
		[]reviewedViewInvoke{{"java/lang/Class", "getMethod", "(Ljava/lang/String;[Ljava/lang/Class;)Ljava/lang/reflect/Method;", core.OP_INVOKEVIRTUAL}, {"java/lang/IllegalStateException", "<init>", "(Ljava/lang/String;Ljava/lang/Throwable;)V", core.OP_INVOKESPECIAL}}, func(source string) {
			body := reviewedControlBody(t, source, `static\s*\{`)
			requireReviewedPattern(t, body, `(?s)try\s*\{.*GET_CONSTRUCTOR\s*=.*getMethod\("getConstructor".*GET_DECLARED_CONSTRUCTOR\s*=.*getMethod\("getDeclaredConstructor".*\}\s*catch\(NoSuchMethodException\s+(\w+)\)`)
			caught := requireReviewedPattern(t, body, `catch\(NoSuchMethodException\s+(\w+)\)`)[1]
			requireReviewedPattern(t, body, `throw new IllegalStateException\("Could not locate Class::getDeclaredConstructor",\s*(?:\(Throwable\)\s*)?\(*`+regexp.QuoteMeta(caught)+`\)*\);`)
			if strings.Count(source, "getMethod(\"getConstructor\"") != 1 || strings.Count(source, "getMethod(\"getDeclaredConstructor\"") != 1 {
				t.Fatal("original reflective initializers must each execute once")
			}
		})
}

func assertReviewedNormSwitch(t *testing.T) {
	reviewedControlJar(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/codecs/lucene80/Lucene80NormsProducer.class", "readFields", "(Lorg/apache/lucene/store/IndexInput;Lorg/apache/lucene/index/FieldInfos;)V", nil,
		[]reviewedViewInvoke{{"org/apache/lucene/store/IndexInput", "readLong", "()J", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/codecs/lucene80/Lucene80NormsProducer$NormsEntry", "normsOffset", "J", core.OP_PUTFIELD}, {"java/util/Map", "put", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", core.OP_INVOKEINTERFACE}}, func(source string) {
			body := reviewedControlBody(t, source, `private\s+void\s+readFields\([^\n]+\)`)
			cases := requireReviewedPattern(t, body, `(?s)case 0:\s*case 1:\s*case 2:\s*case 4:\s*case 8:\s*(.*?)\s*case 3:`)[1]
			requireReviewedPattern(t, cases, `(?s)\.normsOffset\s*=\s*\w+\.readLong\(\);.*this\.norms\.put\(.*\.readInt\(\);\s*(?:continue(?:\s+\w+)?|break)\s*;`)
			requireReviewedPattern(t, body, `(?s)case 3:\s*case 5:\s*case 6:\s*case 7:\s*default:\s*throw new CorruptIndexException\(`)
		})
}

func assertReviewedAppenderAccumulator(t *testing.T) {
	reviewedControlJar(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/attribute/TypeAttributeAppender$ForInstrumentedType$Differentiating.class", "apply", "(Lnet/bytebuddy/jar/asm/ClassVisitor;Lnet/bytebuddy/description/type/TypeDescription;Lnet/bytebuddy/implementation/attribute/AnnotationValueFilter;)V", nil,
		[]reviewedViewInvoke{{"net/bytebuddy/implementation/attribute/AnnotationAppender", "append", "(Lnet/bytebuddy/description/annotation/AnnotationDescription;Lnet/bytebuddy/implementation/attribute/AnnotationValueFilter;)Lnet/bytebuddy/implementation/attribute/AnnotationAppender;", core.OP_INVOKEINTERFACE}}, func(source string) {
			body := reviewedControlBody(t, source, `public\s+void\s+apply\([^\n]+\)`)
			local := requireReviewedPattern(t, body, `AnnotationAppender\s+(\w+)\s*=\s*new AnnotationAppender\$Default\(`)[1]
			name := regexp.QuoteMeta(local)
			requireReviewedPattern(t, body, name+`\s*=\s*`+name+`\.append\(`)
			requireReviewedPattern(t, body, `ofInterfaceType\(`+name+`,\s*\w+,\s*\w+\+\+\)`)
			if regexp.MustCompile(`AnnotationAppender\s+\w+\s*=\s*\w+\.append\(`).MatchString(body) {
				t.Fatalf("loop update became a self-initialized declaration:\n%s", body)
			}
		})
}

func assertReviewedTermsLabeledExit(t *testing.T) {
	reviewedControlJar(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/Terms.class", "getMax", "()Lorg/apache/lucene/util/BytesRef;", []string{"19:35:36:java/lang/UnsupportedOperationException"},
		[]reviewedViewInvoke{{"org/apache/lucene/util/BytesRefBuilder", "setLength", "(I)V", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/util/BytesRefBuilder", "get", "()Lorg/apache/lucene/util/BytesRef;", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/index/TermsEnum", "seekCeil", "(Lorg/apache/lucene/util/BytesRef;)Lorg/apache/lucene/index/TermsEnum$SeekStatus;", core.OP_INVOKEVIRTUAL}}, func(source string) {
			body := reviewedControlBody(t, source, `public\s+BytesRef\s+getMax\(\)`)
			builder := requireReviewedPattern(t, body, `BytesRefBuilder\s+(\w+)\s*=\s*new BytesRefBuilder\(\)`)[1]
			name := regexp.QuoteMeta(builder)
			label := requireReviewedPattern(t, body, `(\w+):\s*do\s*\{`)[1]
			requireReviewedPattern(t, body, `break\s+`+regexp.QuoteMeta(label)+`;`)
			requireReviewedPattern(t, body, name+`\.setLength\(\(*`+name+`\.length\(\)\)*\s*-\s*\(?1\)?\);\s*return\s+`+name+`\.get\(\);`)
		})
}

func assertReviewedLifecycleFirstFailure(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/TestCase.class", "runBare", "()V")
	code, object := reviewedControlCode(t, raw, "runBare", "()V", []string{"10:14:17:java/lang/Throwable", "6:10:27:java/lang/Throwable", "30:34:37:java/lang/Throwable", "6:10:47:*", "27:30:47:*", "48:52:55:java/lang/Throwable", "47:48:47:*"})
	reviewedControlInvokes(t, code, object,
		reviewedViewInvoke{"junit/framework/TestCase", "setUp", "()V", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"junit/framework/TestCase", "runTest", "()V", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"junit/framework/TestCase", "tearDown", "()V", core.OP_INVOKEVIRTUAL})
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_ALREADY_CAUGHT_OFF", setting)
		assertReviewedSources(t, raw, "JDEC_DUP_THROWABLE_CATCH_FINALLY_OFF", func(source string) {
			body := reviewedControlBody(t, source, `public\s+void\s+runBare\(\)`)
			saved := requireReviewedPattern(t, body, `Throwable\s+(\w+)\s*=\s*null;`)[1]
			requireReviewedPattern(t, body, `this\.setUp\(\);\s*(?:try\s*\{\s*)+this\.runTest\(\);`)
			requireReviewedPattern(t, body, `throw\s+(?:\(Throwable\)\s*)?`+regexp.QuoteMeta(saved)+`;`)
			requireReviewedPattern(t, body, `(?s)this\.tearDown\(\);\s*\}\s*catch\(Throwable\s+(\w+)\)\s*\{\s*if\s*\(\(*`+regexp.QuoteMeta(saved)+`\)*\s*==\s*\(?null\)?\)\s*\{\s*`+regexp.QuoteMeta(saved)+`\s*=\s*\w+;`)
		})
	}
}

func assertReviewedConnectionCatchBinding(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/Connection.class", "sendCommand", "(Lredis/clients/jedis/commands/ProtocolCommand;[[B)V")
	code, object := reviewedControlCode(t, raw, "sendCommand", "(Lredis/clients/jedis/commands/ProtocolCommand;[[B)V", []string{"0:13:16:redis/clients/jedis/exceptions/JedisConnectionException", "17:53:56:java/lang/RuntimeException"})
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"redis/clients/jedis/Protocol", "readErrorLineIfPossible", "(Lredis/clients/jedis/util/RedisInputStream;)Ljava/lang/String;", core.OP_INVOKESTATIC}, reviewedViewInvoke{"redis/clients/jedis/Connection", "broken", "Z", core.OP_PUTFIELD})
	assertReviewedSources(t, raw, "JDEC_JEDIS_REMAINING_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+void\s+sendCommand\(ProtocolCommand\s+\w+,\s*byte\[\](?:\[\]|\.\.\.)\s+\w+\)`)
		caught := requireReviewedPattern(t, body, `catch\(JedisConnectionException\s+(\w+)\)`)[1]
		name := regexp.QuoteMeta(caught)
		requireReviewedPattern(t, body, name+`\s*=\s*new JedisConnectionException\([^;]*`+name+`\.getCause\(\)\);`)
		requireReviewedPattern(t, body, `(?s)catch\(RuntimeException\s+\w+\)\s*\{\s*\}\s*this\.broken\s*=\s*true;\s*throw\s+`+name+`;`)
		if regexp.MustCompile(`JedisConnectionException\s+` + name + `\s*=`).MatchString(body) {
			t.Fatalf("caught exception redeclared instead of updated:\n%s", body)
		}
	})
}

func assertReviewedPoolReflectionCatchRegions(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/BaseGenericObjectPool.class", "setEvictionPolicyClassName", "(Ljava/lang/String;Ljava/lang/ClassLoader;)V")
	code, object := reviewedControlCode(t, raw, "setEvictionPolicyClassName", "(Ljava/lang/String;Ljava/lang/ClassLoader;)V", []string{"9:15:18:java/lang/ClassCastException", "9:15:18:java/lang/ClassNotFoundException", "9:27:30:java/lang/ClassCastException", "9:27:89:java/lang/ClassNotFoundException", "9:27:89:java/lang/InstantiationException", "9:27:89:java/lang/IllegalAccessException", "9:27:89:java/lang/reflect/InvocationTargetException", "9:27:89:java/lang/NoSuchMethodException"})
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"org/apache/commons/pool2/impl/BaseGenericObjectPool", "setEvictionPolicy", "(Ljava/lang/String;Ljava/lang/ClassLoader;)V", core.OP_INVOKESPECIAL})
	assertReviewedSources(t, raw, "JDEC_ORIG14_REMAINING_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+final\s+void\s+setEvictionPolicyClassName\(String\s+\w+,\s*ClassLoader\s+\w+\)`)
		requireReviewedPattern(t, body, `(?s)try\s*\{\s*try\s*\{\s*this\.setEvictionPolicy\(.*?catch\(ClassCastException\s*\|\s*ClassNotFoundException\s+\w+\).*?this\.setEvictionPolicy\(.*?\}\s*return;\s*\}\s*catch\(ClassCastException\s+\w+\)`)
		caught := requireReviewedPattern(t, body, `catch\(ClassNotFoundException\s*\|\s*InstantiationException\s*\|\s*IllegalAccessException\s*\|\s*InvocationTargetException\s*\|\s*NoSuchMethodException\s+(\w+)\)`)[1]
		requireReviewedPattern(t, body, `(?s)throw new IllegalArgumentException\(.*"Unable to create ".*\.toString\(\),\s*`+regexp.QuoteMeta(caught)+`\);`)
	})
}

func assertReviewedUnmarshallerProtectedCleanup(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/TreeUnmarshaller.class", "convert", "(Ljava/lang/Object;Ljava/lang/Class;Lcom/thoughtworks/xstream/converters/Converter;)Ljava/lang/Object;")
	code, object := reviewedControlCode(t, raw, "convert", "(Ljava/lang/Object;Ljava/lang/Class;Lcom/thoughtworks/xstream/converters/Converter;)Ljava/lang/Object;", []string{"9:22:32:com/thoughtworks/xstream/converters/ConversionException", "9:22:46:com/thoughtworks/xstream/security/AbstractSecurityException", "9:22:51:java/lang/RuntimeException", "9:22:76:*", "32:78:76:*"})
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"com/thoughtworks/xstream/core/util/FastStack", "popSilently", "()V", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"com/thoughtworks/xstream/converters/ConversionException", "<init>", "(Ljava/lang/Throwable;)V", core.OP_INVOKESPECIAL})
	assertReviewedSources(t, raw, "JDEC_THROW_OBJECT_OFF", func(source string) {
		body := reviewedControlBody(t, source, `protected\s+Object\s+convert\([^\n]+\)`)
		for _, typ := range []string{"ConversionException", "AbstractSecurityException"} {
			caught := requireReviewedPattern(t, body, `catch\(`+typ+`\s+(\w+)\)`)[1]
			requireReviewedPattern(t, body, `throw\s+`+regexp.QuoteMeta(caught)+`;`)
		}
		caught := requireReviewedPattern(t, body, `catch\(RuntimeException\s+(\w+)\)`)[1]
		requireReviewedPattern(t, body, `new ConversionException\(\s*(?:\(Throwable\)\s*)?\(*`+regexp.QuoteMeta(caught)+`\)*\)`)
		// This is required by the original catch-all range over typed handlers.
		// A sibling catch(Throwable) cleans up only the main protected body.
		if !regexp.MustCompile(`finally\s*\{\s*this\.types\.popSilently\(\);`).MatchString(body) {
			t.Fatalf("cleanup does not protect typed-handler rethrows:\n%s", body)
		}
	})
}

func assertReviewedClassCacheInitializer(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/CustomObjectInputStream.class", "<clinit>", "()V")
	code, object := reviewedControlCode(t, raw, "<clinit>", "()V", nil)
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"java/lang/Class", "getName", "()Ljava/lang/String;", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"com/thoughtworks/xstream/core/util/CustomObjectInputStream", "DATA_HOLDER_KEY", "Ljava/lang/String;", core.OP_PUTSTATIC})
	assertReviewedSources(t, raw, "JDEC_CLASSDOLLAR_FORWARD_OFF", func(source string) {
		body := reviewedControlBody(t, source, `static\s*\{`)
		cache := requireReviewedPattern(t, source, `static\s+Class\s+(class\$[\w$]+);`)[1]
		producer := requireReviewedPattern(t, body, `Class\s+(\w+)\s*=\s*class\$\("com.thoughtworks.xstream.core.util.CustomObjectInputStream"\);`)[1]
		requireReviewedPattern(t, body, regexp.QuoteMeta(cache)+`\s*=\s*`+regexp.QuoteMeta(producer)+`;`)
		view := requireReviewedPattern(t, body, `Class\s+(\w+)\s*=\s*null;`)[1]
		requireReviewedPattern(t, body, regexp.QuoteMeta(view)+`\s*=\s*`+regexp.QuoteMeta(producer)+`;`)
		requireReviewedPattern(t, body, regexp.QuoteMeta(view)+`\s*=\s*`+regexp.QuoteMeta(cache)+`;`)
		requireReviewedPattern(t, body, `DATA_HOLDER_KEY\s*=\s*`+regexp.QuoteMeta(view)+`\.getName\(\);`)
		if strings.Index(body, cache+" = "+producer) > strings.Index(body, "DATA_HOLDER_KEY =") {
			t.Fatal("key read moved before cache initialization")
		}
		if strings.Count(body, `class$("com.thoughtworks.xstream.core.util.CustomObjectInputStream")`) != 1 {
			t.Fatal("class lookup duplicated")
		}
	})
}
