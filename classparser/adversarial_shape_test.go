package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// assertOrig14Decompile pins a compiled seed on the real Decompile path:
// ON contains the reconstructed construct, OFF contains the unfixed dump, ON ≠ OFF.
// Unlike assertKillSwitchDecompile it does not early-return when OFF already has onMust.
func assertOrig14Decompile(t *testing.T, seed, onMust, offMust string) {
	t.Helper()
	raw, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON decompile: %v", err)
	}
	if !strings.Contains(on, onMust) {
		t.Fatalf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF decompile: %v", err)
	}
	if strings.Contains(off, onMust) {
		t.Fatalf("OFF already contains reconstructed %q (switch inert)\n%s", onMust, clipForTest(off, onMust))
	}
	if !strings.Contains(off, offMust) {
		t.Fatalf("OFF missing unfixed %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}

func TestAdversarialBareIfMissesOldUnique(t *testing.T) {
	assertOrig14Decompile(t, "testdata/regression/BareIfAdv.class",
		"if ((var3_1) != (0)){",
		"if (var3_1){")
}

func TestAdversarialEmptySyncTrailingElse(t *testing.T) {
	assertOrig14Decompile(t, "testdata/regression/EmptySyncAdv.class",
		"synchronized(this){\n\n\t\t\t}\n\t\t\treturn false;",
		"synchronized(this){\n\n\t\t\t}\n\t\t}")
}

func TestAdversarialNsmeCatchThisBuild(t *testing.T) {
	assertOrig14Decompile(t, "testdata/regression/NsmeCatchAdv.class",
		"ClassNotFoundException | NoSuchMethodException var2",
		"catch(ClassNotFoundException var2){")
}

// Standalone javac of `boolean acc |= bits.set()` already dumps as boolean; the
// unfixed `int varN = 0` OR-accumulator only appears when an enum constant body
// is folded (BloomFilterStrategies$1). Pin that real Decompile path here.
func TestAdversarialBoolOrFoldedEnum(t *testing.T) {
	outer, err := os.ReadFile("testdata/regression/BloomFilterStrategies.class")
	if err != nil {
		t.Fatal(err)
	}
	inner1, err := os.ReadFile("testdata/regression/BloomFilterStrategies$1.class")
	if err != nil {
		t.Fatal(err)
	}
	inner2, err := os.ReadFile("testdata/regression/BloomFilterStrategies$2.class")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name string) ([]byte, bool) {
		switch {
		case strings.Contains(name, "BloomFilterStrategies$1"):
			return inner1, true
		case strings.Contains(name, "BloomFilterStrategies$2"):
			return inner2, true
		default:
			return nil, false
		}
	}
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on, err := DecompileWithResolver(outer, resolve)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if !strings.Contains(on, "boolean var9 = false;") {
		t.Fatalf("ON missing boolean OR-accumulator:\n%s", clipForTest(on, "var9"))
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := DecompileWithResolver(outer, resolve)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if strings.Contains(off, "boolean var9 = false;") {
		t.Fatal("OFF already boolean (switch inert)")
	}
	if !strings.Contains(off, "int var9 = 0;") {
		t.Fatalf("OFF missing int OR-accumulator:\n%s", clipForTest(off, "var9"))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}

func TestFieldWriterListFuncBareIfIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FieldWriterListFunc.class", "JDEC_ORIG14_REMAINING_OFF",
		"if ((var6_1) != (0)){",
		"if (var6_1){")
}

func TestHttp2StreamTrailingElseEmptySyncIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/Http2Stream.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_HTTP2_STREAM_SYNC_OFF", "1")
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, "return false;") {
		t.Fatalf("ON missing return after empty sync:\n%s", clipForTest(on, "closeInternal"))
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if on == off {
		t.Fatal("ON and OFF identical with HTTP2_STREAM_SYNC off")
	}
}

func TestAdversarialShapeSnippetsRenameLocals(t *testing.T) {
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	bare := "\tvoid m() {\n\t\tint var12 = 0;\n\t\tif (var12){\n\t\t\treturn;\n\t\t}\n\t}\n"
	on := fixIntBareIf(bare)
	if !strings.Contains(on, "if ((var12) != (0)){") {
		t.Fatalf("bare-if renamed local:\n%s", on)
	}
	or := "\tboolean any() {\n\t\tint var4 = 0;\n\t\tvar4 = (var4) | (var1.ok());\n\t\treturn var4;\n\t}\n"
	on = fixBoolOrAccumulator(or)
	if !strings.Contains(on, "boolean var4 = false;") {
		t.Fatalf("bool-or renamed local:\n%s", on)
	}
	nsme := "try{\nClass.forName(\"x\").getConstructor(new Class[0]);\n}catch(ClassNotFoundException var7){\nthrow new RuntimeException(var7);\n}"
	on = fixMissingNSMECatch(nsme)
	if !strings.Contains(on, "ClassNotFoundException | NoSuchMethodException var7") {
		t.Fatalf("nsme renamed catch ident:\n%s", on)
	}
	already := "try{\nClass.forName(\"x\").getConstructor(new Class[0]);\n}catch(Exception var7){\nthrow var7;\n}"
	if fixMissingNSMECatch(already) != already {
		t.Fatalf("must not add NSME next to Exception:\n%s", fixMissingNSMECatch(already))
	}
	sibling := "try{\nClass.forName(\"x\").getConstructor(new Class[0]);\n}catch(Exception var7){\nthrow var7;\n}catch(ClassNotFoundException var8){\nthrow var8;\n}"
	if strings.Contains(fixMissingNSMECatch(sibling), "NoSuchMethodException") {
		t.Fatalf("must not add NSME when Exception sibling covers it:\n%s", fixMissingNSMECatch(sibling))
	}
	nested := "try{\ntry{\nClass.forName(\"x\").getConstructor(new Class[0]);\n}catch(Exception var7){\nthrow var7;\n}\n}catch(SecurityException var8){\nthrow var8;\n}"
	if strings.Contains(fixMissingNSMECatch(nested), "SecurityException | NoSuchMethodException") {
		t.Fatalf("must not add NSME to outer catch when inner catch(Exception) covers it:\n%s", fixMissingNSMECatch(nested))
	}
	mapPrint := "\tvoid m() {\n\t\tint var4 = 0;\n\t\tif ((var4) == (max)){\n\t\t\treturn;\n\t\t}\n\t\tvar3.append(this.format(var1,var5.getKey()));\n\t\tvar4++;\n\t}\n"
	if fixObjectGetKeyAssignedToInt(mapPrint) != mapPrint {
		t.Fatalf("int counter next to getKey() arg must stay int:\n%s", fixObjectGetKeyAssignedToInt(mapPrint))
	}
	entry := "\tvoid m() {\n\t\tint var11 = 0;\n\t\tvar11 = var12.getKey();\n\t\tif ((var11) == (var13)){\n\t\t\treturn;\n\t\t}\n\t}\n"
	on = fixObjectGetKeyAssignedToInt(entry)
	if !strings.Contains(on, "Object var11 = null;") {
		t.Fatalf("getKey assignment should retype int:\n%s", on)
	}
	arith := "\tvoid m() {\n\t\tint var5 = 0;\n\t\tvar5 = var6.getKey();\n\t\tvar5 = (var5) + (1);\n\t}\n"
	if fixObjectGetKeyAssignedToInt(arith) != arith {
		t.Fatalf("getKey assigned into an int accumulator must stay int:\n%s", fixObjectGetKeyAssignedToInt(arith))
	}
	inc := "\tvoid m() {\n\t\tint var5 = 0;\n\t\tvar5 = var6.getValue();\n\t\tvar5++;\n\t}\n"
	if fixObjectGetKeyAssignedToInt(inc) != inc {
		t.Fatalf("getKey assigned then incremented must stay int:\n%s", fixObjectGetKeyAssignedToInt(inc))
	}
	lt := "\tvoid m() {\n\t\tint var5 = 0;\n\t\tvar5 = var6.getKey();\n\t\tif ((var5) < (var7)){\n\t\t\treturn;\n\t\t}\n\t}\n"
	if fixObjectGetKeyAssignedToInt(lt) != lt {
		t.Fatalf("getKey assigned then compared with < must stay int:\n%s", fixObjectGetKeyAssignedToInt(lt))
	}
	eq0 := "\tvoid m() {\n\t\tint var5 = 0;\n\t\tvar5 = var6.getKey();\n\t\tif ((var5)==(0)){\n\t\t\treturn;\n\t\t}\n\t}\n"
	if fixObjectGetKeyAssignedToInt(eq0) != eq0 {
		t.Fatalf("getKey assigned then == (0) must stay int:\n%s", fixObjectGetKeyAssignedToInt(eq0))
	}
	mixed := "\tvoid m() {\n\t\tint var11 = 0;\n\t\tvar11 = var12.getKey();\n\t\tif ((var11)==(var13)){\n\t\t}\n\t\tvar11 = (var11) + (1);\n\t}\n"
	if fixObjectGetKeyAssignedToInt(mixed) != mixed {
		t.Fatalf("object-compare plus arithmetic must stay int:\n%s", fixObjectGetKeyAssignedToInt(mixed))
	}
	hashVal := "\tvoid m() {\n\t\tint var5 = 0;\n\t\tvar5 = (int)(((this.contentHash.getValue()) >> (8)) & (255L));\n\t\tif ((var4_1) != (var5)){\n\t\t\treturn;\n\t\t}\n\t}\n"
	if fixObjectGetKeyAssignedToInt(hashVal) != hashVal {
		t.Fatalf("Checksum.getValue arithmetic must stay int:\n%s", fixObjectGetKeyAssignedToInt(hashVal))
	}
	parse := "\tvoid m() {\n\t\tint var4 = 0;\n\t\tvar4 = Integer.parseInt(var1.getValue());\n\t\treturn var4;\n\t}\n"
	if fixObjectGetKeyAssignedToInt(parse) != parse {
		t.Fatalf("parseInt(reader.getValue()) must stay int:\n%s", fixObjectGetKeyAssignedToInt(parse))
	}
	voidSync := "\tpublic void run() {\n\t\tif (done){\n\t\t}else{\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}\n"
	if strings.Contains(fixEmptySyncInTrailingElse(voidSync), "return ") {
		t.Fatalf("void method must not get a return after empty sync:\n%s", fixEmptySyncInTrailingElse(voidSync))
	}
	setSess := "boolean setSession(long var1) {\n\t\tif ((var5) == (null)){\n\t\t\treturn false;\n\t\t}else{\n\t\t\tboolean var6 = false;\n\t\t\tOpenSslClientSessionCache var7 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}\n"
	on = fixEmptySyncInTrailingElse(setSess)
	if !strings.Contains(on, "return false;") || strings.Count(on, "return false;") < 2 {
		t.Fatalf("boolean setSession else-empty-sync should return false:\n%s", on)
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	if fixIntBareIf(bare) != bare || fixBoolOrAccumulator(or) != or || fixMissingNSMECatch(nsme) != nsme {
		t.Fatal("OFF expected identity")
	}
}
