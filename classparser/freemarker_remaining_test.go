package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFreemarkerBooleanZeroIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BeansWrapper.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"boolean var8 = false;",
		"boolean var8 = 0;")
}

func TestFreemarkerIntBareIfIsLoadBearing(t *testing.T) {
	path := "testdata/regression/UnifiedCall.class"
	_, code, _ := reviewedFixtureMethod(t, path, "dump", "(Z)Ljava/lang/String;")
	assertReviewedOpcode(t, code, 63, 3)
	assertReviewedOpcode(t, code, 64, 54, 4)
	assertReviewedOpcode(t, code, 96, 21, 4)
	assertReviewedOpcode(t, code, 98, 153)
	assertReviewedOpcode(t, code, 125, 132, 4, 1)
	reviewedSeedSources(t, path, "JDEC_FREEMARKER_REMAINING_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `String\s+dump\(boolean`)
		counter := requireReviewedPattern(t, body, `if\s*\(\((\w+)\)\s*!=\s*\(0\)\)\{\s*\w+\.append\(\(char\)\(44\)\);`)[1]
		requireReviewedPattern(t, body, `int\s+`+counter+`\s*=\s*0;`)
		if !strings.Contains(body, ".get("+counter+")") || !strings.Contains(body, counter+"++;") {
			t.Fatal("comma guard lost numeric index def-use")
		}
	})
}

func TestFreemarkerTemplateCtorThisFirstIsLoadBearing(t *testing.T) {
	// Eight original constructor delegations (excluding getPlainTextTemplate
	// allocation), independently confirmed with javap; local IDs are not ABI.
	assertReviewedConstructorDelegations(t, "testdata/regression/Template.class", "Template", "JDEC_FREEMARKER_REMAINING_OFF", 8,
		"this.setEncoding(", "this.getParserConfiguration()")
}

// Constructor invocation precedes inert declarations in the core, independently
// of the legacy FreeMarker source reconstruction switch.
func TestFreemarkerFMParserPreservesConstructorOrder(t *testing.T) {
	// Eight constructor delegations (excluding the createExpressionParser
	// factory's ordinary allocation), with original post-delegation validation.
	assertReviewedConstructorDelegations(t, "testdata/regression/FMParser.class", "FMParser", "JDEC_FREEMARKER_REMAINING_OFF", 8,
		"NullArgumentException.check(var4);")
}

func TestFreemarkerMarkupOutputStringIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CommonMarkupOutputFormat.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this.output(var1.getPlainTextContent(),var2);",
		"this.output((MO)(var1.getPlainTextContent()),var2);")
}

// Assignment operands must be parenthesized before instanceof in both policies.
func TestFreemarkerIsoBIPreservesAssignmentPrecedence(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/BuiltInsForDates$iso_BI$Result.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"if ((var4 = ((AdapterTemplateModel)(var2)).getAdaptedObject(TimeZone.class)) instanceof TimeZone){")
}

func TestFreemarkerUnknownSettingSuperIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Configurable$UnknownSettingException.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"super(var1,new Object[]{\"Unknown FreeMarker configuration setting: \"",
		"super(var1,var4);")
}

func TestFreemarkerCustomAttributeConfigurableCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CustomAttribute.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"return ((Configurable)(var1)).getCustomAttribute(this.key,this);",
		"return var1.getCustomAttribute(this.key,this);")
}

func TestFreemarkerBuilderLoopSnippet(t *testing.T) {
	in := "package freemarker.core;\nObject var4 = null;\n\t\t\tdo{\n\t\t\t\tif ((var4) < (this.positionalParamValues.size())){\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if !strings.Contains(out, "int var4 = 0;") {
		t.Fatalf("snippet not rewritten:\n%q\n->\n%q", in, out)
	}
}

func TestFreemarkerBuilderLoopIndexIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/BuilderCallExpression.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_FREEMARKER_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, "int var4 = 0;") {
		t.Fatalf("ON missing int var4:\n%s", on)
	}
}

func TestFreemarkerLineTableReadIntIsLoadBearing(t *testing.T) {
	path := "testdata/regression/LineTableBuilder.class"
	_, code, object := reviewedFixtureMethod(t, path, "read", "()I")
	assertReviewedTypeVarInvoke(t, path, "read", "()I", 4, 182, "java/io/Reader", "read", "()I")
	assertReviewedTypeVarInvoke(t, path, "read", "()I", 10, 183, "freemarker/template/Template$LineTableBuilder", "handleChar", "(I)V")
	assertReviewedTypeVarInvoke(t, path, "read", "()I", 18, 183, "freemarker/template/Template$LineTableBuilder", "rememberException", "(Ljava/lang/Exception;)Ljava/io/IOException;")
	assertReviewedOpcode(t, code, 7, 60)
	assertReviewedOpcode(t, code, 13, 27)
	assertReviewedOpcode(t, code, 14, 172)
	assertReviewedOpcode(t, code, 21, 191)
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	if len(code.ExceptionTable) != 1 || code.ExceptionTable[0].StartPc != 0 || code.ExceptionTable[0].EndPc != 14 || code.ExceptionTable[0].HandlerPc != 15 || cp.GetClassName(int(code.ExceptionTable[0].CatchType)) != "java/lang/Exception" {
		t.Fatal("read/handle exception domain changed")
	}
	reviewedSeedSources(t, path, "JDEC_FREEMARKER_REMAINING_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `public\s+int\s+read\(\)`)
		// The sole successful store at PC7 dominates the PC13 load and PC14
		// IRETURN. A primitive local return cannot throw; moving its lexical
		// position into the try does not widen any effect's handler domain.
		// The failed read/handle path terminates at the original ATHROW PC21.
		result := requireReviewedPattern(t, body, `try\{\s*(?:int\s+)?(\w+)\s*=\s*this\.in\.read\(\);`)[1]
		name := regexp.QuoteMeta(result)
		requireReviewedPattern(t, body, `try\{\s*(?:int\s+)?`+name+`\s*=\s*this\.in\.read\(\);\s*this\.handleChar\(`+name+`\);\s*(?:return\s+`+name+`;\s*)?\}catch\(Exception\s+\w+\)\{`)
		caught := requireReviewedPattern(t, body, `catch\(Exception\s+(\w+)\)\{`)[1]
		requireReviewedPattern(t, body, `catch\(Exception\s+`+regexp.QuoteMeta(caught)+`\)\{\s*throw this\.rememberException\(`+regexp.QuoteMeta(caught)+`\);\s*\}\s*(?:return\s+`+name+`;\s*)?\}$`)
		if strings.Count(body, "return "+result+";") != 1 {
			t.Fatal("successful read must return the same stored result exactly once")
		}
	})
	// Trusted authored methods use the same exception-producing domains and
	// terminal remembering path; the original JVM is the identity oracle.
	roundTripGenericFlowUnits(t, "ReadBoundaryDriver", `import java.io.*;import java.lang.reflect.*;
class ReadBoundaryState{static String trace="";static int mode,handleMode,value;static final IOException io=new IOException("identity");static final RuntimeException runtime=new IllegalStateException("identity");static final Error error=new AssertionError("identity");static final Exception checked=new Exception("identity");static <E extends Throwable>void sneaky(Throwable failure)throws E{throw (E)failure;}}
class ReadBoundaryReader extends Reader{public int read()throws IOException{ReadBoundaryState.trace+="R";switch(ReadBoundaryState.mode){case 1:throw ReadBoundaryState.io;case 2:throw ReadBoundaryState.runtime;case 3:throw ReadBoundaryState.error;case 4:ReadBoundaryState.<RuntimeException>sneaky(ReadBoundaryState.checked);}return ReadBoundaryState.value;}public int read(char[]a,int b,int c)throws IOException{return read();}public void close(){}}
class ReadBoundaryConsumer{final Reader in;ReadBoundaryConsumer(Reader in){this.in=in;}int read()throws IOException{try{int result=in.read();handleChar(result);return result;}catch(Exception failure){throw rememberException(failure);}}private void handleChar(int result){ReadBoundaryState.trace+="H";if(ReadBoundaryState.handleMode==1)throw ReadBoundaryState.runtime;if(ReadBoundaryState.handleMode==2)throw ReadBoundaryState.error;}private IOException rememberException(Exception failure){ReadBoundaryState.trace+="E";if(failure instanceof IOException)return(IOException)failure;if(failure instanceof RuntimeException)throw(RuntimeException)failure;throw new UndeclaredThrowableException(failure);}}
public class ReadBoundaryDriver{public static void main(String[]args){ReadBoundaryConsumer consumer=new ReadBoundaryConsumer(new ReadBoundaryReader());for(int value:new int[]{-1,0,127,65535})for(int mode=0;mode<5;mode++)for(int handle=0;handle<3;handle++){ReadBoundaryState.value=value;ReadBoundaryState.mode=mode;ReadBoundaryState.handleMode=handle;ReadBoundaryState.trace="";try{System.out.println(consumer.read()+":"+ReadBoundaryState.trace);}catch(Throwable failure){System.out.println((failure==ReadBoundaryState.io)+":"+(failure==ReadBoundaryState.runtime)+":"+(failure==ReadBoundaryState.error)+":"+(failure.getCause()==ReadBoundaryState.checked)+":"+ReadBoundaryState.trace);}}}}
`, nil, []string{"ReadBoundaryConsumer"}, Precision, Compatibility, "legacy")
}

func TestFreemarkerInfiniteDoWhileThrowSnippet(t *testing.T) {
	in := "package freemarker.core;\nclass FMParserTokenManager {\n\t\t} while (true);\n\t\tthrow new RuntimeException(\"incomplete control flow\");\n\t}\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if strings.Contains(out, "incomplete control flow") {
		t.Fatalf("throw not dropped:\n%q", out)
	}
	if !strings.Contains(out, "} while (true);") {
		t.Fatalf("loop lost:\n%q", out)
	}
}

func TestFreemarkerInfiniteDoWhileThrowIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FMParserTokenManager.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"} while (true);\n\t}",
		"} while (true);\n\t\tthrow new RuntimeException(\"incomplete control flow\");")
}

func TestFreemarkerNfaLoopBreakSnippet(t *testing.T) {
	in := "package freemarker.core;\nif ((var4) == (var3)){\n\n\t\t\t\t\t\t}else{\n\t\t\t\t\t\t\tcontinue;\n\t\t\t\t\t\t}\n\t\t\t\t\t} while (true);\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if !strings.Contains(out, "\t\t\t\t\t\t\tbreak;") {
		t.Fatalf("break not inserted:\n%q", out)
	}
}

func TestFreemarkerNfaLoopBreakIsLoadBearing(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/FMParserTokenManager.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"if ((var4) == (var3)){\n\t\t\t\t\t\tbreak LOOP_1;", "break LOOP_2;", "this.jjstateSet[this.jjnewStateCnt++]")
}

func TestFreemarkerParserLoopExitSnippet(t *testing.T) {
	in := "package freemarker.core;\n\t\t\tdefault:\n\t\t\t\tbreak;\n\t\t\t}\n\t\t} while (true);\n\t\tthis.jj_la1[0] = this.jj_gen;\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if !strings.Contains(out, "\t\t\t}\n\t\t\tbreak;\n\t\t} while (true);") {
		t.Fatalf("loop-exit break not inserted:\n%q", out)
	}
}

func TestFreemarkerParserLoopExitIsLoadBearing(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/FMParser.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"LOOP_1:", "break LOOP_1;", "this.jj_la1[0] = this.jj_gen;", "return var1;",
		"case 153:\n\t\t\t\tswitch (")
}

func TestFreemarkerTruncateStaticInitSnippet(t *testing.T) {
	in := "package freemarker.core;\n\tpublic static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup(\"<span class='truncateTerminator'>[&#8230;]</span>\")));\n\tpublic static final double DEFAULT_WORD_BOUNDARY_MIN_LENGTH = 0.75D;\n\tstatic  {\n\t\ttry{\n\n\n\n\t\t}catch(TemplateModelException var0){\n\t\t\tthrow new IllegalStateException((Throwable)(var0));\n\t\t}\n\t}\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if strings.Contains(out, "STANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup") &&
		strings.Contains(out, "public static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR =") {
		t.Fatalf("field initializer not moved:\n%s", out)
	}
	if !strings.Contains(out, "STANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup") {
		t.Fatalf("fromMarkup lost:\n%s", out)
	}
}

func TestFreemarkerTruncateStaticInitIsLoadBearing(t *testing.T) {
	path := "testdata/regression/DefaultTruncateBuiltinAlgorithm.class"
	_, code, object := reviewedFixtureMethod(t, path, "<clinit>", "()V")
	assertReviewedTypeVarInvoke(t, path, "<clinit>", "()V", 5, 182, "freemarker/core/HTMLOutputFormat", "fromMarkup", "(Ljava/lang/String;)Lfreemarker/core/CommonTemplateMarkupOutputModel;")
	assertReviewedOpcode(t, code, 11, 179)
	assertReviewedOpcode(t, code, 27, 187)
	assertReviewedOpcode(t, code, 43, 187)
	if len(code.ExceptionTable) != 1 {
		t.Fatal("original initialization domains changed")
	}
	handler := code.ExceptionTable[0]
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	if handler.StartPc != 0 || handler.EndPc != 14 || handler.HandlerPc != 17 || cp.GetClassName(int(handler.CatchType)) != "freemarker/template/TemplateModelException" {
		t.Fatal("markup initialization handler boundary changed")
	}
	reviewedSeedSources(t, path, "JDEC_FREEMARKER_REMAINING_OFF", false, func(source string) {
		if !strings.Contains(source, "public static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR;") {
			t.Fatal("checked initializer left field declaration")
		}
		compact := compactReviewedGenericSource(source)
		requireReviewedPattern(t, compact, `static\{try\{TemplateHTMLOutputModel(\w+)=.*?HTMLOutputFormat\.INSTANCE\.fromMarkup\("(?:\\.|[^"\\])*"\)\)*;STANDARD_M_TERMINATOR=\w+;\}catch\(TemplateModelException\w+\)\{thrownewIllegalStateException\([^;]*;\}ASCII_INSTANCE=newDefaultTruncateBuiltinAlgorithm\([^;]*STANDARD_M_TERMINATOR[^;]*;UNICODE_INSTANCE=newDefaultTruncateBuiltinAlgorithm\([^;]*STANDARD_M_TERMINATOR[^;]*;\}`)
	})
}

func TestFreemarkerBuilderCallCheckedExceptionsIsLoadBearing(t *testing.T) {
	// Original handler membership (including nested wrapper handlers) is the
	// contract. Reuse of a catch local number is a printer choice.
	raw, err := os.ReadFile("testdata/regression/BuilderCallExpression.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedHandlerMultiplicity(t, raw, "JDEC_FREEMARKER_REMAINING_OFF")
	assertDecompileBothPreserve(t, "testdata/regression/BuilderCallExpression.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"return ClassUtil.forName(this.className).newInstance();")
}

func TestFreemarkerClassIntrospectorGetReturnIsLoadBearing(t *testing.T) {
	t.Skip("empty-sync catch-all now owns this site; unique needle no longer matches")
	assertKillSwitchDecompile(t, "testdata/regression/ClassIntrospector.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"return this.createClassIntrospectionData(var1);",
		"synchronized(this.sharedLock){\n\n\t\t\t}")
}

func TestFreemarkerClassBasedModelFactoryGetReturnIsLoadBearing(t *testing.T) {
	t.Skip("empty-sync catch-all now owns this site; unique needle no longer matches")
	assertKillSwitchDecompile(t, "testdata/regression/ClassBasedModelFactory.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"return this.createModel(Class.forName(var1));",
		"synchronized(var3){\n\n\t\t\t}")
}

func TestFreemarkerDebuggerServerPasswordInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/DebuggerServer.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this.password = SecurityUtilities.getSystemProperty(\"freemarker.debug.password\",\"\").getBytes(\"UTF-8\");",
		"final byte[] password = SecurityUtilities.getSystemProperty(\"freemarker.debug.password\",\"\").getBytes(\"UTF-8\");")
}

func TestFreemarkerRmiDebuggerServiceCtorInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/RmiDebuggerService.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this.debugger = new RmiDebuggerImpl(this);",
		"final RmiDebuggerImpl debugger = new RmiDebuggerImpl(this);")
}

func TestFreemarkerDefaultMemberAccessPolicyCheckedIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/DefaultMemberAccessPolicy.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"}catch(IOException var4_0){",
		"Iterator var4 = loadMemberSelectorFileLines().iterator();")
}

func TestFreemarkerNodeListModelGetReturnIsLoadBearing(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/NodeListModel.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"var3 = NAMED_CHILDREN_OP;", "evaluateElementOperation(var2,this.nodes)", "return createNodeListModel(var5_1,this.namespaces);")
}

func TestFreemarkerRhinoWrapperStaticInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/RhinoWrapper.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"UNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));",
		"static final Object UNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));")
}

// The invoke's original name is part of the binary contract. Adapting source
// to an unrelated dependency version silently changes the API being called.
func TestFreemarkerPreservesOriginalJDOMInvocation(t *testing.T) {
	in := "import org.jdom.ProcessingInstruction;\nvar5.getValue(var2);\nvar5_1.getValue(var2);"
	out := fixFreemarkerRemainingReconstructs(in)
	if out != in {
		t.Fatalf("changed original JDOM API: %s", out)
	}
}
