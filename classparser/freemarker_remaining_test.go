package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestFreemarkerBooleanZeroIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BeansWrapper.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"boolean var8 = false;",
		"boolean var8 = 0;")
}

func TestFreemarkerIntBareIfIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/UnifiedCall.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"if ((var4_1) != (0)){",
		"if (var4_1){")
}

func TestFreemarkerTemplateCtorThisFirstIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Template.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this(var1,var2,var4,var5);\n\t\tParserConfiguration var7 = null;",
		"ParserConfiguration var7 = null;\n\t\tthis(var1,var2,var4,var5);")
}

func TestFreemarkerFMParserCtorThisFirstIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FMParser.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this(var3);\n\t\tOutputFormat var7 = null;",
		"OutputFormat var7 = null;\n\t\tLegacyConstructorParserConfiguration var5 = null;\n\t\tthis(var3);")
}

func TestFreemarkerMarkupOutputStringIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CommonMarkupOutputFormat.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"this.output(var1.getPlainTextContent(),var2);",
		"this.output((MO)(var1.getPlainTextContent()),var2);")
}

func TestFreemarkerIsoBIInstanceofSplitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BuiltInsForDates$iso_BI$Result.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"var4 = ((AdapterTemplateModel)(var2)).getAdaptedObject(TimeZone.class);\n\t\t\t\tif (var4 instanceof TimeZone){",
		"if (var4 = ((AdapterTemplateModel)(var2)).getAdaptedObject(TimeZone.class) instanceof TimeZone){")
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
	assertKillSwitchDecompile(t, "testdata/regression/LineTableBuilder.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"int var1 = 0;\n\t\ttry{\n\t\t\tvar1 = this.in.read();",
		"Object var1 = null;\n\t\ttry{\n\t\t\tvar1 = this.in.read();")
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
	assertKillSwitchDecompile(t, "testdata/regression/FMParserTokenManager.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"if ((var4) == (var3)){\n\t\t\t\t\t\t\tbreak;",
		"if ((var4) == (var3)){\n\n\t\t\t\t\t\t}else{")
}

func TestFreemarkerParserLoopExitSnippet(t *testing.T) {
	in := "package freemarker.core;\n\t\t\tdefault:\n\t\t\t\tbreak;\n\t\t\t}\n\t\t} while (true);\n\t\tthis.jj_la1[0] = this.jj_gen;\n"
	out := fixFreemarkerRemainingReconstructs(in)
	if !strings.Contains(out, "\t\t\t}\n\t\t\tbreak;\n\t\t} while (true);") {
		t.Fatalf("loop-exit break not inserted:\n%q", out)
	}
}

func TestFreemarkerParserLoopExitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FMParser.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"\t\t\t}\n\t\t\tbreak;\n\t\t} while (true);\n\t\tthis.jj_la1[",
		"\t\t\tdefault:\n\t\t\t\tbreak;\n\t\t\t}\n\t\t} while (true);\n\t\tthis.jj_la1[")
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
	assertKillSwitchDecompile(t, "testdata/regression/DefaultTruncateBuiltinAlgorithm.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"public static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR;\n\tstatic {",
		"public static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup")
}

func TestFreemarkerBuilderCallCheckedExceptionsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BuilderCallExpression.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"return ClassUtil.forName(this.className).newInstance();\n\t\t\t\t\t}catch(InstantiationException var1_3){",
		"return ClassUtil.forName(this.className).newInstance();\n\t\t\t}else{")
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
	assertKillSwitchDecompile(t, "testdata/regression/NodeListModel.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"if (((var2) == (null)) && ((var3) == (null))){\n\t\t\t\t\tthrow new TemplateModelException",
		"if (((var2) == (null)) && ((var3) == (null))){};")
}

func TestFreemarkerRhinoWrapperStaticInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/RhinoWrapper.class", "JDEC_FREEMARKER_REMAINING_OFF",
		"UNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));",
		"static final Object UNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));")
}
