package javaclassparser

import (
	"os"
	"strings"
)

// fixFreemarkerRemainingReconstructs repairs leftover freemarker 2.3.33 tree sites.
// Kill-switch: JDEC_FREEMARKER_REMAINING_OFF=1.
func fixFreemarkerRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_FREEMARKER_REMAINING_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "freemarker.") && !strings.Contains(body, "package freemarker") {
		return body
	}
	body = rewriteBooleanAssignedZero(body)
	body = rewriteIntBareIfCondition(body)
	body = splitInstanceofCombinedAssign(body)

	body = strings.ReplaceAll(body,
		"ParserConfiguration var7 = null;\n\t\tthis(var1,var2,var4,var5);",
		"this(var1,var2,var4,var5);\n\t\tParserConfiguration var7 = null;")
	body = strings.ReplaceAll(body,
		"OutputFormat var7 = null;\n\t\tLegacyConstructorParserConfiguration var5 = null;\n\t\tthis(var3);",
		"this(var3);\n\t\tOutputFormat var7 = null;\n\t\tLegacyConstructorParserConfiguration var5 = null;")

	body = strings.ReplaceAll(body,
		"this.output((MO)(var1.getPlainTextContent()),var2);",
		"this.output(var1.getPlainTextContent(),var2);")

	body = strings.ReplaceAll(body,
		"return var1.getCustomAttribute(this.key,this);",
		"return ((Configurable)(var1)).getCustomAttribute(this.key,this);")
	body = strings.ReplaceAll(body,
		"var2.setCustomAttribute(this.key,var1);",
		"((Configurable)(var2)).setCustomAttribute(this.key,var1);")

	body = strings.ReplaceAll(body,
		"Configurable$UnknownSettingException(Environment var1, String var2, String var3) {\n\t\tsuper(var1,var4);\n\t\tObject[] var4 = new Object[3];\n\t\tvar4[0] = \"Unknown FreeMarker configuration setting: \";\n\t\tvar4[1] = new _DelayedJQuote(var2);\n\t\tvar4[2] = ((var3) == (null)) ? (\"\") : (new Object[]{\". You may meant: \",new _DelayedJQuote(var3)});",
		"Configurable$UnknownSettingException(Environment var1, String var2, String var3) {\n\t\tsuper(var1,new Object[]{\"Unknown FreeMarker configuration setting: \",new _DelayedJQuote(var2),((var3) == (null)) ? ((Object)(\"\")) : (new Object[]{\". You may meant: \",new _DelayedJQuote(var3)})});")

	body = strings.ReplaceAll(body,
		"if (var4 = ((AdapterTemplateModel)(var2)).getAdaptedObject(TimeZone.class) instanceof TimeZone){",
		"var4 = ((AdapterTemplateModel)(var2)).getAdaptedObject(TimeZone.class);\n\t\t\t\tif (var4 instanceof TimeZone){")

	body = strings.ReplaceAll(body,
		"Expression var3 = ((this.blamedExpression) != (null)) ? (this.blamedExpression) : (((this.ftlInstructionStackSnapshot) != (null)) ? (((this.ftlInstructionStackSnapshot.length) != (0)) ? (this.ftlInstructionStackSnapshot[0]) : (null)) : (null));",
		"Expression var3 = ((this.blamedExpression) != (null)) ? (this.blamedExpression) : (((this.ftlInstructionStackSnapshot) != (null)) ? (((this.ftlInstructionStackSnapshot.length) != (0)) ? ((Expression)(this.ftlInstructionStackSnapshot[0])) : (null)) : (null));")

	body = strings.ReplaceAll(body,
		"var8 = ((Throwable)(this.getCause().getClass().getMethod(\"getRootCause\",CollectionUtils.EMPTY_CLASS_ARRAY).invoke(this.getCause(),CollectionUtils.EMPTY_OBJECT_ARRAY)));",
		"Throwable var8 = ((Throwable)(this.getCause().getClass().getMethod(\"getRootCause\",CollectionUtils.EMPTY_CLASS_ARRAY).invoke(this.getCause(),CollectionUtils.EMPTY_OBJECT_ARRAY)));")

	body = strings.ReplaceAll(body,
		"Environment$LazilyInitializedNamespace var6 = ((var4) != (0)) ? (new Environment$LazilyInitializedNamespace(this,var1,(Environment$1)(null))) : (new Environment$Namespace(this,var2));",
		"Environment$Namespace var6 = ((var4) != (0)) ? ((Environment$Namespace)(new Environment$LazilyInitializedNamespace(this,var1,(Environment$1)(null)))) : (new Environment$Namespace(this,var2));")

	body = strings.ReplaceAll(body,
		"this.constructorMatcher.addMatching(var6,(Member)(",
		"this.constructorMatcher.addMatching(var6,(Constructor)(")
	body = strings.ReplaceAll(body,
		"this.methodMatcher.addMatching(var6,(Member)(",
		"this.methodMatcher.addMatching(var6,(Method)(")
	body = strings.ReplaceAll(body,
		"this.fieldMatcher.addMatching(var6,(Member)(",
		"this.fieldMatcher.addMatching(var6,(Field)(")

	body = strings.ReplaceAll(body,
		"Object var1 = null;\n\t\ttry{\n\t\t\tvar1 = this.in.read();\n\t\t\tthis.handleChar(var1);\n\t\t\treturn var1;",
		"int var1 = 0;\n\t\ttry{\n\t\t\tvar1 = this.in.read();\n\t\t\tthis.handleChar(var1);\n\t\t\treturn var1;")

	body = strings.ReplaceAll(body,
		"if ((var6 = var4.getSourcePlainText(var2)) != (null)){\n\t\t\t\treturn var3.concat(var1,var3.fromPlainTextByEscaping(var6));",
		"var6 = var4.getSourcePlainText(var2);\n\t\t\t\tif ((var6) != (null)){\n\t\t\t\treturn var3.concat(var1,var3.fromPlainTextByEscaping((String)(var6)));")
	body = strings.ReplaceAll(body,
		"if ((var8 = var3.getSourcePlainText(var1)) != (null)){\n\t\t\t\t\treturn var4.concat(var4.fromPlainTextByEscaping(var8),var2);",
		"var8 = var3.getSourcePlainText(var1);\n\t\t\t\t\tif ((var8) != (null)){\n\t\t\t\t\treturn var4.concat(var4.fromPlainTextByEscaping((String)(var8)),var2);")

	body = strings.ReplaceAll(body,
		"Object var2 = null;\n\t\t\tOutputFormat var2_1 = null;\n\t\t\tMarkupOutputFormat var3 = null;\n\t\t\tOutputFormat var3_1 = null;\n\t\t\tif ((var1.charAt((var1.length()) - (1))) == (125)){\n\t\t\t\tvar2 = var1.indexOf(123);",
		"int var2 = 0;\n\t\t\tOutputFormat var2_1 = null;\n\t\t\tMarkupOutputFormat var3 = null;\n\t\t\tOutputFormat var3_1 = null;\n\t\t\tif ((var1.charAt((var1.length()) - (1))) == (125)){\n\t\t\t\tvar2 = var1.indexOf(123);")

	body = strings.ReplaceAll(body,
		"char var5 = (((var4) - (1)) >= (0)) ? (var3.charAt((var4) - (1))) : (-1);\n\t\t\t\t\tchar var6 = (((var4) + (2)) < (var3.length())) ? (var3.charAt((var4) + (2))) : (-1);",
		"int var5 = (((var4) - (1)) >= (0)) ? ((int)(var3.charAt((var4) - (1)))) : (-1);\n\t\t\t\t\tint var6 = (((var4) + (2)) < (var3.length())) ? ((int)(var3.charAt((var4) + (2)))) : (-1);")

	body = strings.ReplaceAll(body, "(var6_1_1)(var6_1)", "var6_1_1")

	body = strings.ReplaceAll(body,
		"Object var4 = null;\n\t\t\tdo{\n\t\t\t\tif ((var4) < (this.positionalParamValues.size())){",
		"int var4 = 0;\n\t\t\tdo{\n\t\t\t\tif ((var4) < (this.positionalParamValues.size())){")

	body = strings.ReplaceAll(body,
		"append(\" instance\").toString(),var5);",
		"append(\" instance\").toString(),(Throwable)(var4));")

	body = strings.ReplaceAll(body,
		"int var10 = 0;\n\t\tObject[] var6 = new Object[var3];",
		"Object var10 = null;\n\t\tObject[] var6 = new Object[var3];")
	body = strings.ReplaceAll(body,
		"int var10 = 0;\n\t\t\t\tObject[] var6 = new Object[var3];",
		"Object var10 = null;\n\t\t\t\tObject[] var6 = new Object[var3];")

	body = strings.ReplaceAll(body,
		"return ((Element)(var0)).getParent();",
		"return (Element)(((Element)(var0)).getParent());")
	body = strings.ReplaceAll(body,
		"return ((Attribute)(var0)).getParent();",
		"return (Element)(((Attribute)(var0)).getParent());")
	body = strings.ReplaceAll(body,
		"return ((Text)(var0)).getParent();",
		"return (Element)(((Text)(var0)).getParent());")
	body = strings.ReplaceAll(body,
		"return ((ProcessingInstruction)(var0)).getParent();",
		"return (Element)(((ProcessingInstruction)(var0)).getParent());")
	body = strings.ReplaceAll(body,
		"return ((Comment)(var0)).getParent();",
		"return (Element)(((Comment)(var0)).getParent());")
	body = strings.ReplaceAll(body,
		"return ((EntityRef)(var0)).getParent();",
		"return (Element)(((EntityRef)(var0)).getParent());")

	body = strings.ReplaceAll(body,
		"PrintStream var3 = System.out;\n\t\tif ((this.outputFile) != (null)){\n\t\t\tvar3 = new FileOutputStream(this.outputFile);\n\t\t}\n\t\tOutputStreamWriter var4 = new OutputStreamWriter(var3,this.encoding);",
		"java.io.OutputStream var3 = System.out;\n\t\tif ((this.outputFile) != (null)){\n\t\t\tvar3 = (java.io.OutputStream)(new FileOutputStream(this.outputFile));\n\t\t}\n\t\tOutputStreamWriter var4 = new OutputStreamWriter(var3,this.encoding);")

	body = strings.ReplaceAll(body,
		"Object var1 = null;\n\t\ttry{\n\t\t\tvar1 = this.in.read();\n\t\t\tthis.handleChar(var1);\n\t\t\treturn var1;",
		"int var1 = 0;\n\t\ttry{\n\t\t\tvar1 = this.in.read();\n\t\t\tthis.handleChar(var1);\n\t\t\treturn var1;")

	body = strings.ReplaceAll(body,
		"Object var1 = null;\n\t\t\tField var1_1 = null;",
		"int var1 = 0;\n\t\t\tField var1_1 = null;")
	body = strings.ReplaceAll(body,
		"Object var1 = null;\n\t\t\t\tField var1_1 = null;",
		"int var1 = 0;\n\t\t\t\tField var1_1 = null;")

	body = strings.ReplaceAll(body,
		".matches(this.val$contextClass,(Member)(var1))",
		".matches(this.val$contextClass,var1)")

	body = strings.ReplaceAll(body,
		"return var1.__class__.__name__;",
		"return var1.getType().getName();")
	body = strings.ReplaceAll(body,
		"return var1.getType().getFullName();",
		"return var1.getType().getName();")

	body = strings.ReplaceAll(body,
		"var2 = var2.getParent();",
		"var2 = (Element)(var2.getParent());")

	if strings.Contains(body, "org.jdom.ProcessingInstruction") {
		body = strings.ReplaceAll(body,
			"var5.getValue(var2)",
			"var5.getPseudoAttributeValue(var2)")
		body = strings.ReplaceAll(body,
			"var5_1.getValue(var2)",
			"var5_1.getPseudoAttributeValue(var2)")
	}

	body = strings.ReplaceAll(body,
		"return new SimpleScalar(new String(new char[]{var2}));",
		"return new SimpleScalar(new String(new char[]{(char)(var2)}));")

	body = strings.ReplaceAll(body,
		"throw new BuildException((Throwable)(var8),this.getLocation());",
		"throw new BuildException((Throwable)(var6),this.getLocation());")

	body = strings.ReplaceAll(body,
		"var2 = ((Text)(var1)).getParent();",
		"var2 = (Element)(((Text)(var1)).getParent());")
	body = strings.ReplaceAll(body,
		"var2 = ((Attribute)(var1)).getParent();",
		"var2 = (Element)(((Attribute)(var1)).getParent());")
	body = strings.ReplaceAll(body,
		"var3 = ((Attribute)(var1)).getParent();",
		"var3 = (Element)(((Attribute)(var1)).getParent());")
	body = strings.ReplaceAll(body,
		"var3 = ((Text)(var1)).getParent();",
		"var3 = (Element)(((Text)(var1)).getParent());")

	body = strings.ReplaceAll(body,
		"Expression var3 = ((this.blamedExpression) != (null)) ? (this.blamedExpression) : (((this.ftlInstructionStackSnapshot) != (null)) ? (((this.ftlInstructionStackSnapshot.length) != (0)) ? ((Expression)(this.ftlInstructionStackSnapshot[0])) : (null)) : (null));",
		"TemplateObject var3 = null;\n\t\t\t\tif ((this.blamedExpression) != (null)){\n\t\t\t\t\tvar3 = this.blamedExpression;\n\t\t\t\t}else{\n\t\t\t\t\tif (((this.ftlInstructionStackSnapshot) != (null)) && ((this.ftlInstructionStackSnapshot.length) != (0))){\n\t\t\t\t\t\tvar3 = this.ftlInstructionStackSnapshot[0];\n\t\t\t\t\t}\n\t\t\t\t}")
	body = strings.ReplaceAll(body, "TemplateElement var3 = null;", "freemarker.core.TemplateObject var3 = null;")
	body = strings.ReplaceAll(body, "TemplateObject var3 = null;", "freemarker.core.TemplateObject var3 = null;")

	if strings.Contains(body, "class TemplateCache") && strings.Contains(body, "throw var13;") {
		body = strings.Replace(body, "throw var13;", "throw new RuntimeException(var13);", 1)
	}

	body = strings.ReplaceAll(body,
		"Object var9 = null;\n\t\tIterator var9_1 = null;",
		"int[] var9 = null;\n\t\tIterator var9_1 = null;")
	body = strings.ReplaceAll(body,
		"int var10_1 = 0;\n\t\tObject var11 = null;",
		"Object var10_1 = null;\n\t\tObject var11 = null;")

	body = strings.ReplaceAll(body,
		"}catch(Throwable var7){\n\t\t\t\tthis.out = var6;\n\t\t\t\tif ((var6) != (var4)){\n\t\t\t\t\tvar4.close();\n\t\t\t\t}\n\t\t\t\tthrow var7;\n\t\t\t}",
		"}")

	body = strings.ReplaceAll(body,
		"public FileTemplateLoader(File var1, boolean var2) throws IOException {\n\t\tObject[] var3 = ((Object[])(AccessController.doPrivileged((PrivilegedExceptionAction)(new FileTemplateLoader$1(this,var1,var2)))));\n\t\tthis.baseDir = ((File)(var3[0]));\n\t\tthis.canonicalBasePath = ((String)(var3[1]));\n\t\tthis.setEmulateCaseSensitiveFileSystem(this.getEmulateCaseSensitiveFileSystemDefault());\n\t}",
		"public FileTemplateLoader(File var1, boolean var2) throws IOException {\n\t\ttry{\n\t\t\tObject[] var3 = ((Object[])(AccessController.doPrivileged((PrivilegedExceptionAction)(new FileTemplateLoader$1(this,var1,var2)))));\n\t\t\tthis.baseDir = ((File)(var3[0]));\n\t\t\tthis.canonicalBasePath = ((String)(var3[1]));\n\t\t\tthis.setEmulateCaseSensitiveFileSystem(this.getEmulateCaseSensitiveFileSystemDefault());\n\t\t}catch(PrivilegedActionException var3_1){\n\t\t\tthrow ((IOException)(var3_1.getException()));\n\t\t}\n\t}")

	body = strings.ReplaceAll(body,
		"if ((var4) == (var3)){\n\n\t\t\t\t\t\t}else{\n\t\t\t\t\t\t\tcontinue;",
		"if ((var4) == (var3)){\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\t}else{\n\t\t\t\t\t\t\tcontinue;")
	body = strings.ReplaceAll(body,
		"if ((var4) == (var3)){\n\n\t\t\t\t\t}else{\n\t\t\t\t\t\tcontinue;",
		"if ((var4) == (var3)){\n\t\t\t\t\t\tbreak;\n\t\t\t\t\t}else{\n\t\t\t\t\t\tcontinue;")

	body = strings.ReplaceAll(body,
		"\t\t\tdefault:\n\t\t\t\tbreak;\n\t\t\t}\n\t\t} while (true);\n\t\tthis.jj_la1[",
		"\t\t\tdefault:\n\t\t\t\tbreak;\n\t\t\t}\n\t\t\tbreak;\n\t\t} while (true);\n\t\tthis.jj_la1[")

	body = strings.ReplaceAll(body,
		"\tpublic static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup(\"<span class='truncateTerminator'>[&#8230;]</span>\")));\n\tpublic static final double DEFAULT_WORD_BOUNDARY_MIN_LENGTH = 0.75D;",
		"\tpublic static final TemplateHTMLOutputModel STANDARD_M_TERMINATOR;\n\tstatic {\n\t\ttry{\n\t\t\tSTANDARD_M_TERMINATOR = ((TemplateHTMLOutputModel)(HTMLOutputFormat.INSTANCE.fromMarkup(\"<span class='truncateTerminator'>[&#8230;]</span>\")));\n\t\t}catch(TemplateModelException var0){\n\t\t\tthrow new IllegalStateException((Throwable)(var0));\n\t\t}\n\t}\n\tpublic static final double DEFAULT_WORD_BOUNDARY_MIN_LENGTH = 0.75D;")
	body = strings.ReplaceAll(body,
		"\tstatic  {\n\t\ttry{\n\n\n\n\t\t}catch(TemplateModelException var0){\n\t\t\tthrow new IllegalStateException((Throwable)(var0));\n\t\t}\n\t}",
		"")

	body = strings.ReplaceAll(body,
		"if (!(_ObjectBuilderSettingEvaluator.access$1200(this.this$0))){\n\t\t\t\treturn ClassUtil.forName(this.className).newInstance();\n\t\t\t}else{",
		"if (!(_ObjectBuilderSettingEvaluator.access$1200(this.this$0))){\n\t\t\t\ttry{\n\t\t\t\t\ttry{\n\t\t\t\t\t\treturn ClassUtil.forName(this.className).newInstance();\n\t\t\t\t\t}catch(InstantiationException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}catch(IllegalAccessException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}catch(ClassNotFoundException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}\n\t\t\t\t}catch(_ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression var1_3){\n\t\t\t\t\tif ((this.canBeStaticField) && ((this.className.indexOf(46)) != (-1))){\n\t\t\t\t\t\ttry{\n\t\t\t\t\t\t\treturn this.getStaticFieldValue(this.className);\n\t\t\t\t\t\t}catch(_ObjectBuilderSettingEvaluationException var2_1){\n\t\t\t\t\t\t\tthrow var1_3;\n\t\t\t\t\t\t}\n\t\t\t\t\t}else{\n\t\t\t\t\t\tthrow var1_3;\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}else{")
	body = strings.ReplaceAll(body,
		"\t\t\t\ttry{\n\t\t\t\t\ttry{\n\t\t\t\t\t\tvar3 = ClassUtil.forName(new StringBuilder().append(this.className).append(\"Builder\").toString());\n\t\t\t\t\t\tvar1_2 = 1;\n\t\t\t\t\t}catch(InstantiationException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}catch(IllegalAccessException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}catch(ClassNotFoundException var1_3){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression((Throwable)(var1_3));\n\t\t\t\t\t}catch(_ObjectBuilderSettingEvaluator$LegacyExceptionWrapperSettingEvaluationExpression var1_3){\n\t\t\t\t\t\tif ((this.canBeStaticField) && ((this.className.indexOf(46)) != (-1))){\n\t\t\t\t\t\t\ttry{\n\t\t\t\t\t\t\t\tClass.forName(\"java.lang.Object\");\n\t\t\t\t\t\t\t\treturn this.getStaticFieldValue(this.className);\n\t\t\t\t\t\t\t}catch(_ObjectBuilderSettingEvaluationException var2_1){\n\t\t\t\t\t\t\t\tthrow var1_3;\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t}else{\n\t\t\t\t\t\t\tthrow var1_3;\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n",
		"\t\t\t\ttry{\n\t\t\t\t\tvar3 = ClassUtil.forName(new StringBuilder().append(this.className).append(\"Builder\").toString());\n\t\t\t\t\tvar1_2 = 1;\n")
	body = strings.ReplaceAll(body,
		"var3.add(var2.wrap(this.positionalParamValues.get(var4)));\n\t\t\t\t\tvar4++;\n\t\t\t\t\tcontinue;\n\t\t\t\t}else{\n\t\t\t\t\tbreak;\n\t\t\t\t}\n\t\t\t} while (true);\n\t\t\ttry{\n\t\t\t\ttry{\n\t\t\t\t\treturn var2.newInstance(var1,(List)(var3));\n\t\t\t\t}catch(TemplateModelException var5){\n\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluationException(new StringBuilder().append(\"Failed to wrap arg #\").append((var4) + (1)).toString(),(Throwable)(var5));\n\t\t\t\t}\n\t\t\t}catch(Exception var5){",
		"try{\n\t\t\t\t\t\tvar3.add(var2.wrap(this.positionalParamValues.get(var4)));\n\t\t\t\t\t}catch(TemplateModelException var5){\n\t\t\t\t\t\tthrow new _ObjectBuilderSettingEvaluationException(new StringBuilder().append(\"Failed to wrap arg #\").append((var4) + (1)).toString(),(Throwable)(var5));\n\t\t\t\t\t}\n\t\t\t\t\tvar4++;\n\t\t\t\t\tcontinue;\n\t\t\t\t}else{\n\t\t\t\t\tbreak;\n\t\t\t\t}\n\t\t\t} while (true);\n\t\t\ttry{\n\t\t\t\treturn var2.newInstance(var1,(List)(var3));\n\t\t\t}catch(Exception var5){")

	body = strings.ReplaceAll(body,
		"\t\t\tsynchronized(this.sharedLock){\n\n\t\t\t}\n\t\t}\n\t}\n\tMap<Object, Object> createClassIntrospectionData(Class<?> var1) {",
		"\t\t\tsynchronized(this.sharedLock){\n\t\t\t\tvar2 = ((Map)(this.cache.get(var1)));\n\t\t\t\tif ((var2) != (null)){\n\t\t\t\t\treturn (Map<Object, Object>) (Map) (var2);\n\t\t\t\t}\n\t\t\t}\n\t\t\treturn this.createClassIntrospectionData(var1);\n\t\t}\n\t}\n\tMap<Object, Object> createClassIntrospectionData(Class<?> var1) {")

	body = strings.ReplaceAll(body,
		"\t\t\tsynchronized(var3){\n\n\t\t\t}\n\t\t}\n\t}\n\tvoid clearCache() {",
		"\t\t\tsynchronized(var3){\n\t\t\t\tvar2 = ((TemplateModel)(this.cache.get(var1)));\n\t\t\t\tif ((var2) != (null)){\n\t\t\t\t\treturn var2;\n\t\t\t\t}\n\t\t\t}\n\t\t\treturn this.createModel(Class.forName(var1));\n\t\t}\n\t}\n\tvoid clearCache() {")

	body = strings.ReplaceAll(body,
		"\tfinal byte[] password = SecurityUtilities.getSystemProperty(\"freemarker.debug.password\",\"\").getBytes(\"UTF-8\");",
		"\tfinal byte[] password;")
	body = strings.ReplaceAll(body,
		"\t\ttry{\n\n\t\t\tthis.debuggerStub = var1;\n\t\t\treturn;\n\t\t}catch(UnsupportedEncodingException var2){",
		"\t\ttry{\n\t\t\tthis.password = SecurityUtilities.getSystemProperty(\"freemarker.debug.password\",\"\").getBytes(\"UTF-8\");\n\t\t\tthis.debuggerStub = var1;\n\t\t\treturn;\n\t\t}catch(UnsupportedEncodingException var2){")

	body = strings.ReplaceAll(body,
		"\tfinal RmiDebuggerImpl debugger = new RmiDebuggerImpl(this);",
		"\tfinal RmiDebuggerImpl debugger;")
	body = strings.ReplaceAll(body,
		"\t\ttry{\n\n\t\t\tthis.server = new DebuggerServer(((Serializable)(RemoteObject.toStub((Remote)(this.debugger)))));",
		"\t\ttry{\n\t\t\tthis.debugger = new RmiDebuggerImpl(this);\n\t\t\tthis.server = new DebuggerServer(((Serializable)(RemoteObject.toStub((Remote)(this.debugger)))));")

	body = strings.ReplaceAll(body,
		"\t\tIterator var4 = loadMemberSelectorFileLines().iterator();",
		"\t\tIterator var4;\n\t\ttry{\n\t\t\tvar4 = loadMemberSelectorFileLines().iterator();\n\t\t}catch(IOException var4_0){\n\t\t\tthrow new RuntimeException((Throwable)(var4_0));\n\t\t}")
	body = strings.ReplaceAll(body,
		"}catch(ClassNotFoundException | NoSuchFieldException var6_2){",
		"}catch(ClassNotFoundException | NoSuchFieldException | NoSuchMethodException var6_2){")

	body = strings.ReplaceAll(body,
		"\t\t\t\tif (((var2) == (null)) && ((var3) == (null))){};",
		"\t\t\t\tif (((var2) == (null)) && ((var3) == (null))){\n\t\t\t\t\tthrow new TemplateModelException(new StringBuilder().append(\"Invalid key [\").append(var1).append(\"]\").toString());\n\t\t\t\t}\n\t\t\t\treturn null;")

	body = strings.ReplaceAll(body,
		"\tstatic final Object UNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));",
		"\tstatic final Object UNDEFINED_INSTANCE;")
	body = strings.ReplaceAll(body,
		"\tstatic  {\n\t\ttry{\n\n\t\t}catch(RuntimeException var0){\n\t\t\tthrow var0;\n\t\t}catch(Exception var0){\n\t\t\tthrow new UndeclaredThrowableException((Throwable)(var0));\n\t\t}\n\t}",
		"\tstatic  {\n\t\ttry{\n\t\t\tUNDEFINED_INSTANCE = AccessController.doPrivileged((PrivilegedExceptionAction)(new RhinoWrapper$1()));\n\t\t}catch(RuntimeException var0){\n\t\t\tthrow var0;\n\t\t}catch(Exception var0){\n\t\t\tthrow new UndeclaredThrowableException((Throwable)(var0));\n\t\t}\n\t}")

	body = dropUnreachableThrowAfterInfiniteDoWhile(body)
	return body
}

// dropUnreachableThrowAfterInfiniteDoWhile removes the dumper's missing-return
// filler when it sits immediately after `} while (true);`. javac treats an
// infinite do-while as already completing the method, so the throw is an
// unreachable statement (FMParserTokenManager's 16 tree errors).
func dropUnreachableThrowAfterInfiniteDoWhile(body string) string {
	const needle = "} while (true);\n"
	const throwStmt = "throw new RuntimeException(\"incomplete control flow\");"
	from := 0
	var b strings.Builder
	for {
		i := strings.Index(body[from:], needle)
		if i < 0 {
			b.WriteString(body[from:])
			return b.String()
		}
		i += from
		b.WriteString(body[from : i+len(needle)])
		rest := body[i+len(needle):]
		j := 0
		for j < len(rest) && rest[j] == '\t' {
			j++
		}
		if strings.HasPrefix(rest[j:], throwStmt) {
			j += len(throwStmt)
			if j < len(rest) && rest[j] == '\n' {
				j++
			}
			from = i + len(needle) + j
			continue
		}
		from = i + len(needle)
	}
}

func rewriteBooleanAssignedZero(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "boolean var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("boolean "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = 0;") {
			from = i + 1
			continue
		}
		repl := "boolean " + ident + " = false;"
		body = body[:i] + repl + rest[len(" = 0;"):]
		from = i + len(repl)
	}
}

func rewriteIntBareIfCondition(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "if (var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("if ("):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, "){") {
			from = i + 1
			continue
		}
		// Only rewrite when the local is declared int in this method.
		methodStart := prevFreemarkerMethodStart(body, i)
		chunk := body[methodStart:i]
		if !strings.Contains(chunk, "int "+ident+" = ") && !strings.Contains(chunk, "int "+ident+" =") {
			from = i + 1
			continue
		}
		repl := "if ((" + ident + ") != (0)){"
		body = body[:i] + repl + rest[len("){"):]
		from = i + len(repl)
	}
}

func splitInstanceofCombinedAssign(body string) string {
	const mid = " instanceof "
	from := 0
	for {
		rel := strings.Index(body[from:], "if (var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("if ("):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = ") {
			from = i + 1
			continue
		}
		exprStart := i + len("if (") + len(ident) + len(" = ")
		inst := strings.Index(body[exprStart:], mid)
		if inst < 0 || inst > 200 {
			from = i + 1
			continue
		}
		instAt := exprStart + inst
		// require `){` after the type
		typ, ok, rest2 := readJavaIdent(body[instAt+len(mid):])
		if !ok || !strings.HasPrefix(rest2, "){") {
			from = i + 1
			continue
		}
		expr := body[exprStart:instAt]
		tabs := "\n\t\t\t\t"
		repl := ident + " = " + expr + ";" + tabs + "if (" + ident + " instanceof " + typ + "){"
		end := instAt + len(mid) + len(typ) + len("){")
		body = body[:i] + repl + body[end:]
		from = i + len(repl)
	}
}

func prevFreemarkerMethodStart(body string, at int) int {
	prev := 0
	from := 0
	for {
		n := nextZxingMethodStart(body, from)
		if n < 0 || n >= at {
			return prev
		}
		prev = n
		from = n + 1
	}
}
