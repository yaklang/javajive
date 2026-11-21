package dex2jar_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive"
	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar"
	"github.com/yaklang/javajive/dex2jar/ir"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
)

const (
	dexAccessPublic      = 0x0001
	dexAccessPrivate     = 0x0002
	dexAccessStatic      = 0x0008
	dexAccessConstructor = 0x10000
)

func decompileLoweredClass(t *testing.T, raw []byte) string {
	t.Helper()
	res, err := classparser.DecompileWithOptions(raw, classparser.DecompileOptions{
		Mode:           classparser.Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("classparser.DecompileWithOptions(ShadowIR): %v", err)
	}
	if res.Source == "" {
		t.Fatalf("empty decompiled source (status=%s)", res.Status)
	}
	return res.Source
}

func TestAdversarialDEXReadOnlyParamsSuperAndIIncLoop(t *testing.T) {
	const owner = "Ladv/SubAct;"
	const base = "Ladv/BaseAct;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: base},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "I"},
		},
		Protos: []model.Proto{
			{ReturnType: "V", Parameters: []string{"Ljava/lang/Object;"}, ParameterTypes: []uint32{2}},
			{ReturnType: "V"},
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "onCreate", ProtoIndex: 0},
			{Class: base, Name: "onCreate", ProtoIndex: 0},
			{Class: owner, Name: "helper", ProtoIndex: 1},
			{Class: owner, Name: "sumTo", ProtoIndex: 2},
		},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  base,
		AccessFlags: dexAccessPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: dexAccessPrivate,
				Method:      file.Methods[2],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
			{
				MethodIndex: 3,
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      file.Methods[3],
				// v0=sum, v1=i, v2=n(param):
				// 0: const/4 v0, 0
				// 1: const/4 v1, 0
				// 2: if-ge v1, v2, +5 (to offset 7: return v0)
				// 4: add-int/2addr v0, v1
				// 5: add-int/lit8 v1, v1, 1  --> emits JVM IINC!
				// 7..8 -> wait: goto -4 -> target offset 2
				// offset calculation:
				// 0: 0x0012 (1u) -> v0=0
				// 1: 0x0112 (1u) -> v1=0
				// 2: 0x2135, 0x0006 (2u) -> if-ge v1, v2, +6 (target=8)
				// 4: 0x10b0 (1u) -> add-int/2addr v0, v1
				// 5: 0x01d8, 0x0101 (2u) -> add-int/lit8 v1, v1, +1
				// 7: 0xfb28 (1u) -> goto -5 (7 - 5 = 2)
				// 8: 0x000f (1u) -> return v0
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1,
					Instructions: []uint16{
						0x0012,
						0x0112,
						0x2135, 0x0006,
						0x10b0,
						0x01d8, 0x0101,
						0xfb28,
						0x000f,
					},
				},
			},
		},
		VirtualMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexAccessPublic,
				Method:      file.Methods[0],
				// v0=this, v1=bundle: invoke-super {v0, v1}, BaseAct.onCreate; invoke-direct {v0}, helper; return-void
				Code: &model.Code{
					RegistersSize: 2, InsSize: 2, OutsSize: 2,
					Instructions: []uint16{
						0x206f, 0x0001, 0x0010,
						0x1070, 0x0002, 0x0000,
						0x000e,
					},
				},
			},
		},
	}

	raw, err := lower.Class(file, class)
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	parsed, err := classparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sawIInc := false
	for _, m := range parsed.Methods {
		if utf8 := parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)); utf8 != nil && utf8.Value == "sumTo" {
			for _, attr := range m.Attributes {
				if ca, ok := attr.(*classparser.CodeAttribute); ok {
					for _, b := range ca.Code {
						if b == 0x84 { // IINC
							sawIInc = true
						}
					}
				}
			}
		}
	}
	if !sawIInc {
		t.Fatal("expected JVM IINC (0x84) from add-int/lit8 in sumTo")
	}

	src := decompileLoweredClass(t, raw)
	if !strings.Contains(src, "super.onCreate(") {
		t.Fatalf("expected super.onCreate in decompiled DEX class:\n%s", src)
	}
	if strings.Contains(src, "= this;") {
		t.Fatalf("read-only this parameter should map directly to slot 0 without var alias:\n%s", src)
	}
	if !strings.Contains(src, "++") && !strings.Contains(src, "for (") {
		t.Fatalf("expected clean increment loop in sumTo:\n%s", src)
	}
}

func TestAdversarialDEXPolymorphicZeroAndFloatArray(t *testing.T) {
	const owner = "Ladv/PolyZero;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"ok"},
		Types: []model.Type{
			{Descriptor: owner},                 // 0
			{Descriptor: "Ljava/lang/Object;"},  // 1
			{Descriptor: "I"},                   // 2
			{Descriptor: "F"},                   // 3
			{Descriptor: "[F"},                  // 4
			{Descriptor: "[I"},                  // 5
			{Descriptor: "[Ljava/lang/Object;"}, // 6
			{Descriptor: "J"},                   // 7
			{Descriptor: "D"},                   // 8
		},
		Protos: []model.Proto{
			{ReturnType: "V", Parameters: []string{"I", "Ljava/lang/Object;"}, ParameterTypes: []uint32{2, 1}}, // 0
			{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{2}},                          // 1
			{ReturnType: "V", Parameters: []string{"F"}, ParameterTypes: []uint32{3}},                          // 2
			{ReturnType: "V", Parameters: []string{"Ljava/lang/Object;"}, ParameterTypes: []uint32{1}},         // 3
			{ReturnType: "[F", Parameters: []string{"F", "F"}, ParameterTypes: []uint32{3, 3}},                 // 4
			{ReturnType: "V"}, // 5
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{2}}, // 6
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/Object;"}, ParameterTypes: []uint32{6}},
		},
		Fields: []model.FieldID{
			{Class: owner, Name: "sf", Type: "F"},                  // 0
			{Class: owner, Name: "instI", Type: "I"},               // 1
			{Class: owner, Name: "so", Type: "Ljava/lang/Object;"}, // 2
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "takeIntObj", ProtoIndex: 0},            // 0
			{Class: owner, Name: "takeInt", ProtoIndex: 1},               // 1
			{Class: owner, Name: "takeFloat", ProtoIndex: 2},             // 2
			{Class: owner, Name: "takeObj", ProtoIndex: 3},               // 3
			{Class: owner, Name: "sameInsn", ProtoIndex: 5},              // 4
			{Class: owner, Name: "branchedAll", ProtoIndex: 6},           // 5
			{Class: owner, Name: "makeFloatArr", ProtoIndex: 4},          // 6
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 5}, // 7
			{Class: owner, Name: "<init>", ProtoIndex: 1},                // 8
			{Class: owner, Name: "debugLocals", ProtoIndex: 5},           // 9
			{Class: owner, Name: "takeObjArr", ProtoIndex: 7},            // 10
		},
	}

	// Constructor <init>(int p): sput/iput deferred poly-zero before super()
	// RegistersSize=3, InsSize=2 (v1=this, v2=p):
	// const/4 v0, 0
	// iput v0, v1, instI (field 1) -- pre-super deferred store of PolyZero
	// invoke-static {v0}, takeFloat(F) -- v0 is PolyZero(Int|Float)
	// invoke-direct {v1}, Object.<init>
	// return-void
	ctorMethod := model.EncodedMethod{
		MethodIndex: 8,
		AccessFlags: dexAccessPublic | dexAccessConstructor,
		Method:      file.Methods[8],
		Code: &model.Code{
			RegistersSize: 3, InsSize: 2, OutsSize: 1,
			Instructions: []uint16{
				0x0012,
				0x1059, 0x0001,
				0x1071, 0x0002, 0x0000,
				0x1070, 0x0007, 0x0001,
				0x000e,
			},
		},
	}

	// Same-instruction mixed Int+Obj + move + array/unary/arith/field/if-eq/switch:
	// RegistersSize=5, InsSize=0:
	// 0: const/4 v0, 0
	// 1: invoke-static {v0, v0}, takeIntObj (Int + Ref)
	// 4: invoke-static {v0}, takeFloat (Float -> all 3 PolyZero bits!)
	// 7: move v1, v0 (v1 used only as Int!)
	// 8: neg-float v2, v0
	// 9: add-float v3, v0, v2
	// 11: add-int v4, v1, v1
	// 13: sput v0, sf (field 0: Float)
	// 15: sput-object v0, so (field 2: Object)
	// 17: const/4 v2, 1
	// 18: new-array v3, v2, [Ljava/lang/Object; (type 6)
	// 20: aput-object v0, v3, v0 (v0 as Obj value + Int idx!)
	// 22: invoke-static {v3}, takeObjArr
	// 25: if-eqz v0, +2 (25 + 2 = 27)
	// 27: move v2, v0 (dead move of PolyZero)
	// 28: packed-switch v0, +5 (payload at 33)
	// 31: const/4 v0, 0
	// 32: return-void
	// 33: payload (0x0100, count=1, firstKey=0, target=-2 -> 28-2 = wait target 31 = +3):
	sameInsn := model.EncodedMethod{
		MethodIndex: 4,
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      file.Methods[4],
		Code: &model.Code{
			RegistersSize: 5, InsSize: 0, OutsSize: 2,
			Instructions: []uint16{
				0x0012,
				0x2071, 0x0000, 0x0000,
				0x1071, 0x0002, 0x0000,
				0x0101,
				0x027f,
				0x03a6, 0x0200,
				0x0490, 0x0101,
				0x0067, 0x0000,
				0x0069, 0x0002,
				0x1212,
				0x2323, 0x0006,
				0x004d, 0x0003,
				0x1071, 0x000a, 0x0003,
				0x0038, 0x0002,
				0x0201,
				0x002b, 0x0005, 0x0000,
				0x0012,
				0x000e,
				0x0100, 0x0001, 0x0000, 0x0000, 0x0003, 0x0000,
			},
		},
	}

	// Branched diamond: v0=const/4 0; if-eqz p(v4), L_obj; invoke takeInt(v0); goto L_cmp; L_obj: invoke takeObj(v0); L_cmp:
	// const/4 v1, 0; if-eq v0, v1, L_ret; add-int/lit8 v0, v0, 1; return v0; L_ret: return v4
	// Offsets:
	// 0: 0x0012 (const/4 v0, 0)
	// 1: 0x0438, 0x0006 (if-eqz v4, +6 -> 7)
	// 3: 0x1071, 0x0001, 0x0000 (takeInt v0)
	// 6: 0x0428 (goto +4 -> 10)
	// 7: 0x1071, 0x0003, 0x0000 (takeObj v0)
	// 10: 0x0112 (const/4 v1, 0)
	// 11: 0x1032, 0x0005 (if-eq v0, v1, +5 -> 16)
	// 13: 0x00d8, 0x0100 (add-int/lit8 v0, v0, 1 -> wait: (dst<<8)|0xd8 = 0x00d8, (lit8<<8)|src = 0x0100)
	// 15: 0x000f (return v0)
	// 16: 0x040f (return v4)
	branchedAll := model.EncodedMethod{
		MethodIndex: 5,
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      file.Methods[5],
		Code: &model.Code{
			RegistersSize: 5, InsSize: 1, OutsSize: 1,
			Instructions: []uint16{
				0x0012,
				0x0438, 0x0006,
				0x1071, 0x0001, 0x0000,
				0x0428,
				0x1071, 0x0003, 0x0000,
				0x0112,
				0x1032, 0x0005,
				0x00d8, 0x0100,
				0x000f,
				0x040f,
			},
		},
	}

	// filled-new-array {v1, v2}, [F
	makeFloatArr := model.EncodedMethod{
		MethodIndex: 6,
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      file.Methods[6],
		Code: &model.Code{
			RegistersSize: 3, InsSize: 2, OutsSize: 2,
			Instructions: []uint16{
				0x2024, 0x0004, 0x0021,
				0x000c,
				0x0011,
			},
		},
	}

	// debug_info with F, J, D local variable table entries preserved (not skipped):
	// v0: F, v1:v2: J, v3:v4: D
	debugLocals := model.EncodedMethod{
		MethodIndex: 9,
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      file.Methods[9],
		Code: &model.Code{
			RegistersSize: 5, InsSize: 0,
			Instructions: []uint16{
				0x0012,         // const/4 v0, 0
				0x0116, 0x0001, // const-wide/16 v1, 1
				0x0316, 0x0000, // const-wide/16 v3, 0
				0x0082, // int-to-float v0, v0 (types v0 Float!)
				0x3386, // long-to-double v3, v3 (types v3 Double!)
				0x000e, // return-void
			},
			Debug: &model.DebugInfo{
				LineStart: 10,
				Positions: []model.DebugPosition{{Address: 0, Line: 10}},
				Locals: []model.DebugLocal{
					{Register: 0, Name: "fLocal", Type: "F", Start: 0, End: 8},
					{Register: 1, Name: "jLocal", Type: "J", Start: 1, End: 8},
					{Register: 3, Name: "dLocal", Type: "D", Start: 3, End: 8},
				},
			},
		},
	}

	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexAccessPublic,
		StaticFields: []model.EncodedField{
			{FieldIndex: 0, AccessFlags: dexAccessPublic | dexAccessStatic, Field: file.Fields[0]},
			{FieldIndex: 2, AccessFlags: dexAccessPublic | dexAccessStatic, Field: file.Fields[2]},
		},
		InstanceFields: []model.EncodedField{
			{FieldIndex: 1, AccessFlags: dexAccessPublic, Field: file.Fields[1]},
		},
		DirectMethods: []model.EncodedMethod{
			ctorMethod,
			{AccessFlags: dexAccessPublic | dexAccessStatic, Method: file.Methods[0], Code: &model.Code{RegistersSize: 2, InsSize: 2, Instructions: []uint16{0x000e}}},
			{AccessFlags: dexAccessPublic | dexAccessStatic, Method: file.Methods[1], Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}}},
			{AccessFlags: dexAccessPublic | dexAccessStatic, Method: file.Methods[2], Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}}},
			{AccessFlags: dexAccessPublic | dexAccessStatic, Method: file.Methods[3], Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}}},
			{AccessFlags: dexAccessPublic | dexAccessStatic, Method: file.Methods[10], Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}}},
			sameInsn,
			branchedAll,
			makeFloatArr,
			debugLocals,
		},
	}

	raw, err := lower.Class(file, class)
	if err != nil {
		t.Fatalf("lower.Class(PolyZero + float[] + F/J/D LVT): %v", err)
	}
	parsed, err := classparser.Parse(raw)
	if err != nil {
		t.Fatalf("classparser.Parse: %v", err)
	}

	// Verify LocalVariableTable preserved fLocal, jLocal, dLocal.
	sawLVT := map[string]bool{}
	for _, m := range parsed.Methods {
		if utf8 := parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)); utf8 != nil && utf8.Value == "debugLocals" {
			for _, attr := range m.Attributes {
				if ca, ok := attr.(*classparser.CodeAttribute); ok {
					for _, sub := range ca.Attributes {
						if lvt, ok := sub.(*classparser.UnparsedAttribute); ok && lvt.Name == "LocalVariableTable" {
							for _, name := range []string{"fLocal", "jLocal", "dLocal"} {
								if strings.Contains(string(lvt.Info), name) || containsUTF8Constant(parsed, lvt.Info, name) {
									sawLVT[name] = true
								}
							}
						}
					}
				}
			}
		}
	}
	for _, name := range []string{"fLocal", "jLocal", "dLocal"} {
		if !sawLVT[name] {
			t.Fatalf("LocalVariableTable dropped %q", name)
		}
	}

	src := decompileLoweredClass(t, raw)
	if !strings.Contains(src, "float[]") {
		t.Fatalf("expected float[] in makeFloatArr output:\n%s", src)
	}

	// Negative tests: non-zero constant and parameter must still be rejected for mixed Int + Ref.
	for _, neg := range []struct {
		name string
		code model.Code
		ins  uint16
		regs uint16
	}{
		{
			name: "non-zero constant mixed",
			regs: 1, ins: 0,
			code: model.Code{
				RegistersSize: 1, InsSize: 0, OutsSize: 2,
				Instructions: []uint16{0x1012, 0x2071, 0x0000, 0x0000, 0x000e},
			},
		},
		{
			name: "parameter mixed",
			regs: 1, ins: 1,
			code: model.Code{
				RegistersSize: 1, InsSize: 1, OutsSize: 2,
				Instructions: []uint16{0x2071, 0x0000, 0x0000, 0x000e},
			},
		},
	} {
		t.Run(neg.name, func(t *testing.T) {
			proto := model.Proto{ReturnType: "V"}
			if neg.ins == 1 {
				proto = model.Proto{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{2}}
			}
			fCopy := *file
			fCopy.Protos = append([]model.Proto(nil), file.Protos...)
			pIdx := uint16(len(fCopy.Protos))
			fCopy.Protos = append(fCopy.Protos, proto)
			mID := model.MethodID{Class: owner, Name: "bad", ProtoIndex: pIdx}
			cCopy := &model.Class{Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexAccessPublic}
			em := &model.EncodedMethod{AccessFlags: dexAccessPublic | dexAccessStatic, Method: mID, Code: &neg.code}
			if _, err := ir.Build(ir.Input{File: &fCopy, Class: cCopy, Method: em}); err == nil {
				t.Fatalf("%s unexpectedly accepted by ir.Build", neg.name)
			}
		})
	}
}

func containsUTF8Constant(parsed *classparser.ClassObject, info []byte, target string) bool {
	// Parse standard JVMS LocalVariableTable u2 count + entries (10 bytes each, name_index at offset +4..+6)
	if len(info) < 2 {
		return false
	}
	count := int(info[0])<<8 | int(info[1])
	for i := 0; i < count && 2+(i+1)*10 <= len(info); i++ {
		off := 2 + i*10
		nameIdx := int(info[off+4])<<8 | int(info[off+5])
		if utf8 := parsed.ConstantPoolManager.GetUtf8(nameIdx); utf8 != nil && utf8.Value == target {
			return true
		}
	}
	return false
}

func TestAdversarialAABRecoverRetainsR8Classes(t *testing.T) {
	// Step A: verify end-to-end DecompileDEXFileWithPolicy recovery pipeline on compact APK.
	apk := filepath.Join("..", "testdata", "external", "bundletool-app.apk")
	if _, err := os.Stat(apk); err == nil {
		outDir := t.TempDir()
		report, err := javajive.DecompileDEXFileWithPolicy(apk, outDir, javajive.DexConvertOptions{
			Policy: javajive.DexFailureRecover,
		}, 0)
		if err != nil {
			t.Fatalf("DecompileDEXFileWithPolicy(bundletool-app.apk, DexFailureRecover): %v", err)
		}
		if report.Status != "partial" && report.Status != "complete" {
			t.Fatalf("unexpected APK recover report status: %q", report.Status)
		}
		act, err := os.ReadFile(filepath.Join(outDir, "com", "test", "app", "TestActivity.java"))
		if err != nil {
			t.Fatalf("read TestActivity.java: %v", err)
		}
		if strings.Contains(string(act), "// decompile dump failed") || !strings.Contains(string(act), "super.onCreate(") {
			t.Fatalf("unexpected TestActivity.java output:\n%s", act)
		}
	}

	// Step B: full 1,298-class AAB translation + targeted decompilation of representative R8 failure classes.
	aab := filepath.Join("..", "testdata", "external", "bundletool-install-time.aab")
	if _, err := os.Stat(aab); err != nil {
		t.Skipf("external AAB fixture unavailable: %v", err)
	}
	jarPath := filepath.Join(t.TempDir(), "aab-recover.jar")
	detailed, err := dex2jar.DexFileToJarWithOptions(aab, jarPath, dex2jar.ConvertOptions{
		Policy: dex2jar.FailureRecover,
	})
	if err != nil {
		t.Fatalf("DexFileToJarWithOptions(DexFailureRecover): %v", err)
	}
	if detailed.Summary.Classes != 1298 {
		t.Fatalf("expected 1298 AAB classes, got %d", detailed.Summary.Classes)
	}

	strictFS, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal(strict): %v", err)
	}
	defer strictFS.Close()
	strictMain, err := strictFS.ReadFile("com/google/android/samples/dynamicfeatures/MainActivity.class")
	if err != nil {
		t.Fatalf("strict ReadFile(MainActivity.class): %v", err)
	}
	if !strings.Contains(string(strictMain), "// decompile dump failed") {
		t.Fatalf("expected strict mode to keep failing on unproved MainActivity.class, got:\n%s", strictMain)
	}

	recoverFS, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal(recover): %v", err)
	}
	defer recoverFS.Close()
	recoverFS.SetRetainUnprovedSource(true)

	targets := []string{
		"com/google/android/samples/dynamicfeatures/MainActivity.class",
		"com/google/android/samples/dynamicfeatures/a.class",
		"a/a/k/a/a.class",
		"a/a/l/a/a.class",
		"c/b.class",
		"com/google/android/material/appbar/AppBarLayout$BaseBehavior.class",
		"com/google/android/material/floatingactionbutton/a.class",
		"com/google/android/material/transformation/FabTransformationBehavior.class",
		"a/e/a/i$a.class",
		"b/a/a/b/a/d/s.class",
		"androidx/lifecycle/d$a.class",
		"a/d/f$e.class",
		"androidx/lifecycle/LiveData$a.class",
		"c/e.class",
		"c/f.class",
		"c/g.class",
	}
	for _, entry := range targets {
		raw, rerr := recoverFS.ReadFile(entry)
		if rerr != nil {
			t.Fatalf("recover ReadFile(%s): %v", entry, rerr)
		}
		src := string(raw)
		if strings.TrimSpace(src) == "" {
			t.Fatalf("recover ReadFile(%s) returned empty source", entry)
		}
		if strings.Contains(src, "// decompile dump failed") {
			t.Fatalf("%s still failed to decompile under RetainUnprovedSource:\n%s", entry, src)
		}
		if entry == "com/google/android/samples/dynamicfeatures/MainActivity.class" && !strings.Contains(src, "class MainActivity") {
			t.Fatalf("expected class MainActivity in MainActivity.class output:\n%s", src)
		}
	}
}
