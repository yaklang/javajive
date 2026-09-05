package javaclassparser

// 承重测试: log4j-core 剩余 dump 重构 (EnglishEnums.valueOf Enum 擦除,
// PrivateConfig filter 过载, LoggerContext putIfAbsent, OutputStream 三元 LUB,
// PluginAttribute float 字面量, MarkerManager 嵌套 import, ClockFactory Supplier,
// FixedTimeZoneFormat this() 等). kill-switch: JDEC_LOG4J_REMAINING_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestLog4jRemainingReconstructsAreLoadBearing(t *testing.T) {
	in := strings.Join([]string{
		"public enum Filter$Result {",
		"		return ((Filter$Result)(EnglishEnums.valueOf(Filter$Result.class,var0,(Enum)(var1))));",
		"}",
		"public class Logger$PrivateConfig {",
		"	boolean filter(Level var1, Marker var2, String var3, Throwable var4) {",
		"		Filter var5 = this.config.getFilter();",
		"		if ((var5) != (null)){",
		"			Filter$Result var6 = var5.filter(this.logger,var1,var2,var3,var4);",
		"}",
		"public class LoggerContext {",
		"			this.loggerRegistry.putIfAbsent(var1,var2,(ExtendedLogger)(var3));",
		"}",
		"public final class OutputStreamAppender {",
		"		NullOutputStream var3 = ((var0) == (null)) ? (NullOutputStream.getInstance()) : (new CloseShieldOutputStream(var0));",
		"		NullOutputStream var4 = ((var0) == (null)) ? (var3) : (var0);",
		"}",
		"public @interface PluginAttribute {",
		"	public abstract float defaultFloat() default 0.000000;",
		"}",
		"abstract class MarkerMixIn {",
		"import MarkerManager.Log4jMarker;",
		"}",
		"public class ExtendedThreadInfoFactory {",
		"import ThreadDumpMessage.ThreadInfoFactory;",
		"}",
		"public final class ClockFactory {",
		"		HashMap var0 = new HashMap();",
		"		var0.put(\"SystemClock\",SystemClock::new);",
		"}",
		"public enum FixedDateFormat$FixedTimeZoneFormat {",
		"	private FixedDateFormat$FixedTimeZoneFormat() {",
		"	Object var1 = null;",
		"	Object var2 = null;",
		"		this(var1,var2,(char)(0),true,4);",
		"	}",
		"}",
		"public class BurstFilter {",
		"				this.history.add((Delayed)(var2));",
		"}",
		"public class JdkMapAdapterStringMap {",
		"				var1.accept(var2[var3],this.map.get(var2[var3]));",
		"		PUT_ALL = (TriConsumer) ((l0, l1, l2) -> {",
		"			l2.put(l0,l1);",
		"		});",
		"}",
		"public class CommandLine {",
		"	static boolean access$1776(CommandLine var0, boolean var1) {",
		"		byte var2 = (byte)((var0.versionHelpRequested) | (var1));",
		"		var0.versionHelpRequested = var2;",
		"		return var2;",
		"	}",
		"}",
		"public class GelfLayout {",
		"			this.mapWriter.accept(l0,l1,var2);",
		"}",
		"public class ConsoleAppender {",
		"		}catch(UnsupportedEncodingException | NoSuchMethodException var4_1){",
		"}",
		"public class Log4jStackTraceElementDeserializer {",
		"					return new StackTraceElement(var4,var5,var6,var7,var8,var9,var10);",
		"			} while (true);",
		"		}else{",
		"			throw JsonMappingException.from(var1,String.format(\"Cannot deserialize instance of %s out of %s token\"",
		"}",
		"public class JmsManager {",
		"						this.closeJndiManager();",
		"						this.reconnector.reconnect();",
		"						try{",
		"							this.createMessageAndSend(var1,var2);",
		"						}catch(JMSException var5){",
		"}",
		"public class Base64Converter {",
		"			}catch(ClassNotFoundException | NoSuchMethodException var1){",
		"				LOGGER.error(\"No Base64 Converter is available\");",
		"			}catch(NoSuchMethodException var1){",
		"",
		"			}",
		"}",
		"public class PluginCache {",
		"			lv2_6.setDefer(var2_f6.readBoolean());",
		"}",
		"public class TextEncoderHelper {",
		"		if (var2.isOverflow()){",
		"			ByteBufferDestination var3 = var0;",
		"			synchronized(var0){",
		"",
		"			}",
		"		}else{",
		"			return var1;",
		"		}",
		"}",
		"public class MulticastDnsAdvertiser {",
		"			}catch(IllegalAccessException | InvocationTargetException | NoSuchMethodException var10){",
		"				LOGGER.warn(\"Unable to invoke registerService method\",var10);",
		"			}catch(NoSuchMethodException var10){",
		"				LOGGER.warn(\"No registerService method\",(Throwable)(var10));",
		"			}",
		"}",
		"public class ScriptManager$MainScriptRunner {",
		"	public Object execute(Bindings var1) {",
		"		if ((this.compiledScript) != (null)){",
		"			return this.compiledScript.eval(var1);",
		"		}else{",
		"			try{",
		"				try{",
		"					return this.scriptEngine.eval(this.script.getScriptText(),var1);",
		"				}catch(ScriptException var2){",
		"					ScriptManager.access$100().error(new StringBuilder().append(\"Error running script \").append(this.script.getName()).toString(),(Throwable)(var2));",
		"					return null;",
		"				}",
		"			}catch(ScriptException var2){",
		"				ScriptManager.access$100().error(new StringBuilder().append(\"Error running script \").append(this.script.getName()).toString(),(Throwable)(var2));",
		"				return null;",
		"			}",
		"		}",
		"	}",
		"}",
	}, "\n")

	os.Unsetenv("JDEC_LOG4J_REMAINING_OFF")
	on := fixLog4jRemainingReconstructs(in)
	checks := []string{
		"EnglishEnums.valueOf(Filter$Result.class,var0,var1)",
		"var5.filter(this.logger,var1,var2,(Object)(var3),var4)",
		"putIfAbsent(var1,var2,var3)",
		"OutputStream var3 = ((var0) == (null))",
		"float defaultFloat() default 0.0f;",
		"import org.apache.logging.log4j.MarkerManager.Log4jMarker;",
		"import org.apache.logging.log4j.message.ThreadDumpMessage.ThreadInfoFactory;",
		"HashMap<String, Supplier<Clock>> var0 = new HashMap();",
		"this.history.add(var2);",
		"(V)(this.map.get(var2[var3]))",
		"((Map)(l2)).put(l0,l1)",
		"var0.versionHelpRequested |= var1;",
		"this.mapWriter.accept((String)(l0),l1,var2);",
		"}catch(UnsupportedEncodingException var4_1){",
		"return null;\n\t\t}else{",
		"this.reconnector.reconnect();\n\t\t\t\t\t\t\tthis.createMessageAndSend",
		"LOGGER.error(\"No Base64 Converter is available\");\n\t\t\t}",
		"catch(IOException var_io)",
		"return var0.getByteBuffer();",
		"Unable to invoke registerService method\",var10);",
		"try{\n\t\t\tif ((this.compiledScript) != (null)){",
	}
	for _, want := range checks {
		if !strings.Contains(on, want) {
			t.Errorf("ON: missing %q, got:\n%s", want, on)
		}
	}
	if strings.Contains(on, "this(var1,var2,(char)(0),true,4)") {
		t.Errorf("ON: synthetic FixedTimeZoneFormat no-arg ctor should be removed, got:\n%s", on)
	}
	if strings.Contains(on, "No registerService method") {
		t.Errorf("ON: duplicate NoSuchMethodException catch should be removed, got:\n%s", on)
	}

	t.Setenv("JDEC_LOG4J_REMAINING_OFF", "1")
	off := fixLog4jRemainingReconstructs(in)
	if off != in {
		t.Errorf("OFF: expected identity, got:\n%s", off)
	}
}

func TestLog4jEnglishEnumsValueOfIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Log4jFilterResult.class")
	if err != nil {
		t.Fatalf("read Log4jFilterResult: %v", err)
	}
	os.Unsetenv("JDEC_LOG4J_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "EnglishEnums.valueOf(Filter$Result.class") {
		t.Fatalf("expected EnglishEnums.valueOf, got:\n%s", on)
	}
	if strings.Contains(on, "valueOf(Filter$Result.class,var0,(Enum)") {
		t.Errorf("ON: must NOT pass raw (Enum) to EnglishEnums.valueOf, got:\n%s", on)
	}

	t.Setenv("JDEC_LOG4J_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "valueOf(Filter$Result.class,var0,(Enum)") {
		t.Errorf("OFF: expected raw (Enum) third arg, got:\n%s", off)
	}
}

func TestLog4jPluginAttributeFloatDefaultIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Log4jPluginAttribute.class")
	if err != nil {
		t.Fatalf("read Log4jPluginAttribute: %v", err)
	}
	os.Unsetenv("JDEC_LOG4J_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "float defaultFloat() default 0.0f;") {
		t.Errorf("ON: expected 0.0f float default, got:\n%s", on)
	}

	t.Setenv("JDEC_LOG4J_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "float defaultFloat() default 0.000000;") {
		t.Errorf("OFF: expected double 0.000000 default, got:\n%s", off)
	}
}
