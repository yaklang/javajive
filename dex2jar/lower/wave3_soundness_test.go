package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func runMultipleWithVerify(t *testing.T, classes map[string][]byte, mainClass string) {
	t.Helper()
	for name, raw := range classes {
		if _, err := javaclassparser.Parse(raw); err != nil {
			t.Fatalf("Parse(%s) failed: %v", name, err)
		}
	}
	java, err := exec.LookPath("java")
	if err != nil {
		return
	}
	dir := t.TempDir()
	for name, raw := range classes {
		dst := filepath.Join(dir, filepath.FromSlash(name)+".class")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, mainClass).CombinedOutput(); err != nil {
		t.Fatalf("java -Xverify:all %s failed: %v\n%s", mainClass, err, out)
	}
}

func TestWave3AssignHomesLegacyPinnedZeroKindVerify(t *testing.T) {
	// Exceed assignWebs cap (Blocks*RegistersSize > 200000) so assignHomesLegacy
	// handles register v0, where an if-eqz / const/4 0 leaves const/4 0 typed as
	// Null while another definition in v0 uses PolyZeroInt (without Float or Ref).
	// Step 1 ensures pinnedZeroKind[v0] = ir.Int resolves const/4 0 to ICONST_0
	// rather than ACONST_NULL.
	const owner = "Ldex/LegacyPinnedZero;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I", "I"}, ParameterTypes: []uint32{1, 1}},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{3}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "legacy", ProtoIndex: 0},
			{Class: owner, Name: "main", ProtoIndex: 1},
		},
	}
	var insns []uint16
	// 101 goto +1 instructions create 102 blocks; * 2048 regs = 208896 > 200000 -> assignHomesLegacy.
	for i := 0; i < 101; i++ {
		insns = append(insns, 0x0128) // goto +1
	}
	// Block 101+:
	//   const/4 v0, #0        (defCat=Null because if-eqz demands nothing yet, regNarrowMask[v0]==PolyZeroInt)
	//   if-eqz v2, +2         (if v2 == 0 jump past overwrite)
	//   const/4 v0, #7
	//   add-int/2addr v0, v1
	//   return v0
	// Wait: here v0 at add-int/2addr would propagate Int backward unless v0 was ALSO redefined!
	// Even better:
	//   const/4 v0, #0        (followed immediately by if-eqz v0, +2 -> target returns v1; fallthrough does const/4 v0, #3 + add-int + return v0)
	// On the fallthrough path, const/4 v0, #3 overwrites v0 before add-int, so const/4 v0, #0 stays Null
	// while regNarrowMask[v0] == PolyZeroInt!
	insns = append(insns,
		0x0012,         // const/4 v0, #0
		0x0038, 0x0006, // if-eqz v0, +6 -> const/4 v0, #4
		0x3012, // const/4 v0, #3
		0x5112, // const/4 v1, #5
		0x10b0, // add-int/2addr v0, v1
		0x000f, // return v0
		0x4012, // target: const/4 v0, #4
		0x000f, // return v0
	)

	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{
					RegistersSize: 2048, InsSize: 2, OutsSize: 0,
					Instructions: insns,
				},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x1012,                 // const/4 v0, #1
						0x2112,                 // const/4 v1, #2
						0x2071, 0x0000, 0x0010, // invoke-static {v0, v1}, legacy(II)I
						0x000a, // move-result v0
						0x000e, // return-void
					},
				},
			},
		},
	}

	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed: %v", err)
	}
	code := findWave2MethodCode(t, raw, "legacy")
	// Verify every byte in legacy(): const/4 v0, #0 should emit ICONST_0 (0x03), never ACONST_NULL (0x01) + ASTORE.
	hasAconstNull := false
	for _, b := range code.Code {
		if b == 0x01 {
			hasAconstNull = true
			break
		}
	}
	if hasAconstNull {
		t.Fatalf("legacy() emitted ACONST_NULL for single-kind PolyZeroInt register v0: %x", code.Code)
	}
	runMainWithVerify(t, raw, "dex.LegacyPinnedZero")
}

func TestWave3NullArrayOpsVerify(t *testing.T) {
	const owner = "Ldex/NullArrayOps;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "Ljava/lang/NullPointerException;"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{4}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "probe", ProtoIndex: 0},
			{Class: owner, Name: "main", ProtoIndex: 1},
		},
	}
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{
					RegistersSize: 4, InsSize: 1, OutsSize: 0, // v3 = arg String
					Instructions: []uint16{
						0x0012,         // 0: const/4 v0, #0 (null array)
						0x0112,         // 1: const/4 v1, #0 (index 0)
						0x0246, 0x0100, // 2..3: aget-object v2, v0, v1
						0x034d, 0x0100, // 4..5: aput-object v3, v0, v1
						0x0144, 0x0100, // 6..7: aget v1, v0, v1
						0x0211, // 8: return-object v2
					},
				},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x0012,                 // 0: const/4 v0, #0
						0x1071, 0x0000, 0x0000, // 1..3: invoke-static {v0}, probe(String)String
						0x000e, // 4: return-void (try ends at 4)
						0x000d, // 5: move-exception v0
						0x000e, // 6: return-void
					},
					Tries: []model.Try{
						{
							StartAddress: 1, InstructionCount: 3,
							Handler: model.CatchHandler{Handlers: []model.TypedHandler{{TypeIndex: 2, Type: "Ljava/lang/NullPointerException;", Address: 5}}},
						},
					},
				},
			},
		},
	}

	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed on null array ops: %v", err)
	}
	runMainWithVerify(t, raw, "dex.NullArrayOps")
}

func TestWave3InterfaceAssignabilityAcceptsButConcreteRejects(t *testing.T) {
	const (
		implDesc  = "LC0/e;"
		ifaceDesc = "LN0/f;"
		otherDesc = "LO0/g;"
		hostDesc  = "LD0/H;"
	)
	buildFixture := func(expectedDesc string) (*model.File, *model.Class) {
		file := &model.File{
			Entry: "classes.dex",
			Types: []model.Type{
				{Descriptor: "Ljava/lang/Object;"},
				{Descriptor: implDesc},
				{Descriptor: ifaceDesc},
				{Descriptor: otherDesc},
				{Descriptor: hostDesc},
				{Descriptor: "V"},
				{Descriptor: "[Ljava/lang/String;"},
			},
			Protos: []model.Proto{
				{ReturnType: "V"}, // 0
				{ReturnType: "V", Parameters: []string{implDesc}, ParameterTypes: []uint32{1}},     // 1
				{ReturnType: "V", Parameters: []string{expectedDesc}, ParameterTypes: []uint32{2}}, // 2
				{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{6}},
			},
			Methods: []model.MethodID{
				{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, // 0
				{Class: implDesc, Name: "<init>", ProtoIndex: 0},             // 1
				{Class: hostDesc, Name: "z", ProtoIndex: 1},                  // 2
				{Class: hostDesc, Name: "accept", ProtoIndex: 2},             // 3
				{Class: hostDesc, Name: "main", ProtoIndex: 3},               // 4
			},
			Classes: []model.Class{
				{Descriptor: implDesc, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic},
				{Descriptor: ifaceDesc, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic | dexInterface | dexAbstract},
				{Descriptor: otherDesc, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic},
			},
		}
		if expectedDesc == otherDesc {
			file.Protos[2].ParameterTypes[0] = 3
		}
		host := &model.Class{
			Descriptor:  hostDesc,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexPublic,
			DirectMethods: []model.EncodedMethod{
				{
					MethodIndex: 2,
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[2],
					Code: &model.Code{
						RegistersSize: 1, InsSize: 1, OutsSize: 1,
						Instructions: []uint16{
							0x1071, 0x0003, 0x0000, // invoke-static {v0}, accept(expected)V
							0x000e,
						},
					},
				},
				{
					MethodIndex: 3,
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[3],
					Code: &model.Code{
						RegistersSize: 1, InsSize: 1, OutsSize: 0,
						Instructions: []uint16{0x000e},
					},
				},
				{
					MethodIndex: 4,
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[4],
					Code: &model.Code{
						RegistersSize: 2, InsSize: 1, OutsSize: 1,
						Instructions: []uint16{
							0x0012,                 // const/4 v0, #0 (null C0.e)
							0x1071, 0x0002, 0x0000, // invoke-static {v0}, z(C0/e;)V
							0x000e,
						},
					},
				},
			},
		}
		file.Classes = append(file.Classes, *host)
		return file, host
	}

	ifaceFile, ifaceHost := buildFixture(ifaceDesc)
	hostRaw, err := Class(ifaceFile, ifaceHost)
	if err != nil {
		t.Fatalf("concrete -> unimplemented interface rejected: %v", err)
	}
	implRaw, err := Class(ifaceFile, &ifaceFile.Classes[0])
	if err != nil {
		t.Fatalf("impl Class failed: %v", err)
	}
	ifaceRaw, err := Class(ifaceFile, &ifaceFile.Classes[1])
	if err != nil {
		t.Fatalf("iface Class failed: %v", err)
	}
	runMultipleWithVerify(t, map[string][]byte{
		"D0/H": hostRaw,
		"C0/e": implRaw,
		"N0/f": ifaceRaw,
	}, "D0.H")

	badFile, badHost := buildFixture(otherDesc)
	if _, err := Class(badFile, badHost); err == nil || !strings.Contains(err.Error(), "not provably assignable") {
		t.Fatalf("concrete -> unrelated concrete class error = %v, want not provably assignable", err)
	}
}

func TestWave3CanonicalEnumHelperAndKotlinEntries(t *testing.T) {
	t.Run("synthetic $values() helper", func(t *testing.T) {
		const owner = "Ldex/HelperColor;"
		const arrayOwner = "[Ldex/HelperColor;"
		file := &model.File{
			Entry:   "classes.dex",
			Strings: []string{"RED", "GREEN"},
			Types: []model.Type{
				{Descriptor: owner},
				{Descriptor: "Ljava/lang/Enum;"},
				{Descriptor: "Ljava/lang/String;"},
				{Descriptor: "I"},
				{Descriptor: "V"},
				{Descriptor: arrayOwner},
				{Descriptor: "Ljava/lang/Object;"},
				{Descriptor: "Ljava/lang/Class;"},
				{Descriptor: "[Ljava/lang/String;"},
			},
			Fields: []model.FieldID{
				{Class: owner, Name: "RED", Type: owner},
				{Class: owner, Name: "GREEN", Type: owner},
				{Class: owner, Name: "$VALUES", Type: arrayOwner},
			},
			Protos: []model.Proto{
				{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}},
				{ReturnType: "V"},
				{ReturnType: arrayOwner},
				{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
				{ReturnType: "Ljava/lang/Object;"},
				{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{7, 2}},
				{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{8}},
			},
			Methods: []model.MethodID{
				{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 0},  // 0
				{Class: owner, Name: "<init>", ProtoIndex: 0},               // 1
				{Class: owner, Name: "<clinit>", ProtoIndex: 1},             // 2
				{Class: owner, Name: "$values", ProtoIndex: 2},              // 3
				{Class: owner, Name: "values", ProtoIndex: 2},               // 4
				{Class: arrayOwner, Name: "clone", ProtoIndex: 4},           // 5
				{Class: owner, Name: "valueOf", ProtoIndex: 3},              // 6
				{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5}, // 7
				{Class: owner, Name: "main", ProtoIndex: 6},                 // 8
			},
		}
		cls := &model.Class{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Enum;",
			AccessFlags: dexPublic | dexFinal | dexEnum,
			StaticFields: []model.EncodedField{
				{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[0]},
				{FieldIndex: 1, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[1]},
				{FieldIndex: 2, AccessFlags: dexPrivate | dexStatic | dexFinal | dexSynthetic, Field: file.Fields[2]},
			},
			DirectMethods: []model.EncodedMethod{
				{
					MethodIndex: 1,
					AccessFlags: dexPrivate | dexConstructor,
					Method:      file.Methods[1],
					Code: &model.Code{
						RegistersSize: 3, InsSize: 3, OutsSize: 3,
						Instructions: []uint16{0x3070, 0x0000, 0x0210, 0x000e},
					},
				},
				{
					MethodIndex: 3, // $values()
					AccessFlags: dexPrivate | dexStatic | dexSynthetic,
					Method:      file.Methods[3],
					Code: &model.Code{
						RegistersSize: 3, InsSize: 0, OutsSize: 0,
						Instructions: []uint16{
							0x2012,         // const/4 v0, #2
							0x0023, 0x0005, // new-array v0, v0, [Ldex/HelperColor;
							0x0162, 0x0000, // sget-object v1, RED
							0x0212,         // const/4 v2, #0
							0x014d, 0x0200, // aput-object v1, v0[v2]
							0x0162, 0x0001, // sget-object v1, GREEN
							0x1212,         // const/4 v2, #1
							0x014d, 0x0200, // aput-object v1, v0[v2]
							0x0011, // return-object v0
						},
					},
				},
				{
					MethodIndex: 2, // <clinit>() calls $values()
					AccessFlags: dexStatic | dexConstructor,
					Method:      file.Methods[2],
					Code: &model.Code{
						RegistersSize: 3, InsSize: 0, OutsSize: 3,
						Instructions: []uint16{
							0x0022, 0x0000, // new-instance v0, HelperColor
							0x011a, 0x0000, // const-string v1, "RED"
							0x0212,                 // const/4 v2, #0
							0x3070, 0x0001, 0x0210, // invoke-direct {v0, v1, v2}, <init>
							0x0069, 0x0000, // sput-object v0, RED
							0x0022, 0x0000, // new-instance v0, HelperColor
							0x011a, 0x0001, // const-string v1, "GREEN"
							0x1212,                 // const/4 v2, #1
							0x3070, 0x0001, 0x0210, // invoke-direct {v0, v1, v2}, <init>
							0x0069, 0x0001, // sput-object v0, GREEN
							0x0071, 0x0003, 0x0000, // invoke-static {}, $values()
							0x000c,         // move-result-object v0
							0x0069, 0x0002, // sput-object v0, $VALUES
							0x000e, // return-void
						},
					},
				},
				{
					MethodIndex: 4, // values()
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[4],
					Code: &model.Code{
						RegistersSize: 1, InsSize: 0, OutsSize: 1,
						Instructions: []uint16{0x0062, 0x0002, 0x106e, 0x0005, 0x0000, 0x000c, 0x001f, 0x0005, 0x0011},
					},
				},
				{
					MethodIndex: 6, // valueOf(String)
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[6],
					Code: &model.Code{
						RegistersSize: 2, InsSize: 1, OutsSize: 2,
						Instructions: []uint16{0x001c, 0x0000, 0x2071, 0x0007, 0x0010, 0x000c, 0x001f, 0x0000, 0x0011},
					},
				},
				{
					MethodIndex: 8, // main([String;)V
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[8],
					Code: &model.Code{
						RegistersSize: 2, InsSize: 1, OutsSize: 0,
						Instructions: []uint16{
							0x0071, 0x0004, 0x0000, // invoke-static {}, values()
							0x000c, // move-result-object v0
							0x000e,
						},
					},
				},
			},
		}
		raw, err := Class(file, cls)
		if err != nil {
			t.Fatalf("strict Class failed: %v", err)
		}
		src, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Fatalf("strict Decompile failed: %v", err)
		}
		if !strings.Contains(src, "public enum HelperColor") || strings.Contains(src, "extends Enum") || strings.Contains(src, "ParseBytesCode failed") {
			t.Fatalf("unexpected HelperColor decompile output:\n%s", src)
		}
		if !strings.Contains(src, "RED") || !strings.Contains(src, "GREEN") {
			t.Fatalf("missing constants in HelperColor:\n%s", src)
		}
		runMainWithVerify(t, raw, "dex.HelperColor")
	})

	t.Run("Kotlin $VALUES and $ENTRIES tail", func(t *testing.T) {
		const owner = "Ldex/KtColor;"
		const arrayOwner = "[Ldex/KtColor;"
		file := &model.File{
			Entry:   "classes.dex",
			Strings: []string{"ONE", "TWO"},
			Types: []model.Type{
				{Descriptor: owner},
				{Descriptor: "Ljava/lang/Enum;"},
				{Descriptor: "Ljava/lang/String;"},
				{Descriptor: "I"},
				{Descriptor: "V"},
				{Descriptor: arrayOwner},
				{Descriptor: "Ljava/lang/Object;"},
				{Descriptor: "Ljava/lang/Class;"},
				{Descriptor: "[Ljava/lang/Enum;"},
				{Descriptor: "Lkotlin/enums/EnumEntries;"},
			},
			Fields: []model.FieldID{
				{Class: owner, Name: "ONE", Type: owner},
				{Class: owner, Name: "TWO", Type: owner},
				{Class: owner, Name: "$VALUES", Type: arrayOwner},
				{Class: owner, Name: "$ENTRIES", Type: "Lkotlin/enums/EnumEntries;"},
			},
			Protos: []model.Proto{
				{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}},
				{ReturnType: "V"},
				{ReturnType: arrayOwner},
				{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
				{ReturnType: "Ljava/lang/Object;"},
				{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{7, 2}},
				{ReturnType: "Lkotlin/enums/EnumEntries;", Parameters: []string{"[Ljava/lang/Enum;"}, ParameterTypes: []uint32{8}},
			},
			Methods: []model.MethodID{
				{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 0},                  // 0
				{Class: owner, Name: "<init>", ProtoIndex: 0},                               // 1
				{Class: owner, Name: "<clinit>", ProtoIndex: 1},                             // 2
				{Class: owner, Name: "values", ProtoIndex: 2},                               // 3
				{Class: arrayOwner, Name: "clone", ProtoIndex: 4},                           // 4
				{Class: owner, Name: "valueOf", ProtoIndex: 3},                              // 5
				{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5},                 // 6
				{Class: "Lkotlin/enums/EnumEntriesKt;", Name: "enumEntries", ProtoIndex: 6}, // 7
			},
		}
		cls := &model.Class{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Enum;",
			AccessFlags: dexPublic | dexFinal | dexEnum,
			StaticFields: []model.EncodedField{
				{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[0]},
				{FieldIndex: 1, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[1]},
				{FieldIndex: 2, AccessFlags: dexPrivate | dexStatic | dexFinal | dexSynthetic, Field: file.Fields[2]},
				{FieldIndex: 3, AccessFlags: dexPrivate | dexStatic | dexFinal | dexSynthetic, Field: file.Fields[3]},
			},
			DirectMethods: []model.EncodedMethod{
				{
					MethodIndex: 1,
					AccessFlags: dexPrivate | dexConstructor,
					Method:      file.Methods[1],
					Code: &model.Code{
						RegistersSize: 3, InsSize: 3, OutsSize: 3,
						Instructions: []uint16{0x3070, 0x0000, 0x0210, 0x000e},
					},
				},
				{
					MethodIndex: 2, // <clinit> with inline $VALUES and $ENTRIES tail
					AccessFlags: dexStatic | dexConstructor,
					Method:      file.Methods[2],
					Code: &model.Code{
						RegistersSize: 4, InsSize: 0, OutsSize: 3,
						Instructions: []uint16{
							0x0022, 0x0000, // new-instance v0
							0x011a, 0x0000, // "ONE"
							0x0212,
							0x3070, 0x0001, 0x0210,
							0x0069, 0x0000, // sput ONE
							0x0022, 0x0000, // new-instance v0
							0x011a, 0x0001, // "TWO"
							0x1212,
							0x3070, 0x0001, 0x0210,
							0x0069, 0x0001, // sput TWO
							0x2312,         // const/4 v3, #2
							0x3023, 0x0005, // new-array v0, v3, [KtColor
							0x0162, 0x0000, // sget ONE
							0x0212,
							0x014d, 0x0200, // aput
							0x0162, 0x0001, // sget TWO
							0x1212,
							0x014d, 0x0200, // aput
							0x0069, 0x0002, // sput $VALUES
							0x0062, 0x0002, // sget $VALUES
							0x001f, 0x0008, // check-cast v0, [Ljava/lang/Enum;
							0x1071, 0x0007, 0x0000, // invoke-static {v0}, EnumEntriesKt.enumEntries
							0x000c,         // move-result-object v0
							0x0069, 0x0003, // sput $ENTRIES
							0x000e,
						},
					},
				},
				{
					MethodIndex: 3, // values()
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[3],
					Code: &model.Code{
						RegistersSize: 1, InsSize: 0, OutsSize: 1,
						Instructions: []uint16{0x0062, 0x0002, 0x106e, 0x0004, 0x0000, 0x000c, 0x001f, 0x0005, 0x0011},
					},
				},
				{
					MethodIndex: 5, // valueOf(String)
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[5],
					Code: &model.Code{
						RegistersSize: 2, InsSize: 1, OutsSize: 2,
						Instructions: []uint16{0x001c, 0x0000, 0x2071, 0x0006, 0x0010, 0x000c, 0x001f, 0x0000, 0x0011},
					},
				},
			},
		}
		raw, err := Class(file, cls)
		if err != nil {
			t.Fatalf("strict Class failed on KtColor: %v", err)
		}
		src, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Fatalf("strict Decompile failed on KtColor: %v", err)
		}
		if !strings.Contains(src, "public enum KtColor") || strings.Contains(src, "extends Enum") || strings.Contains(src, "ParseBytesCode failed") {
			t.Fatalf("unexpected KtColor decompile output:\n%s", src)
		}
		if !strings.Contains(src, "ONE") || !strings.Contains(src, "TWO") {
			t.Fatalf("missing constants in KtColor:\n%s", src)
		}
	})

	t.Run("Kotlin [Ljava/lang/Object;.clone()", func(t *testing.T) {
		const owner = "Ldex/KtCloneColor;"
		const arrayOwner = "[Ldex/KtCloneColor;"
		file := &model.File{
			Entry:   "classes.dex",
			Strings: []string{"ONE", "TWO"},
			Types: []model.Type{
				{Descriptor: owner},
				{Descriptor: "Ljava/lang/Enum;"},
				{Descriptor: "Ljava/lang/String;"},
				{Descriptor: "I"},
				{Descriptor: "V"},
				{Descriptor: arrayOwner},
				{Descriptor: "Ljava/lang/Object;"},
				{Descriptor: "Ljava/lang/Class;"},
				{Descriptor: "[Ljava/lang/Object;"},
			},
			Fields: []model.FieldID{
				{Class: owner, Name: "ONE", Type: owner},
				{Class: owner, Name: "TWO", Type: owner},
				{Class: owner, Name: "$VALUES", Type: arrayOwner},
			},
			Protos: []model.Proto{
				{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}},
				{ReturnType: "V"},
				{ReturnType: arrayOwner},
				{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
				{ReturnType: "Ljava/lang/Object;"},
				{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{7, 2}},
			},
			Methods: []model.MethodID{
				{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 0},   // 0
				{Class: owner, Name: "<init>", ProtoIndex: 0},                // 1
				{Class: owner, Name: "<clinit>", ProtoIndex: 1},              // 2
				{Class: owner, Name: "values", ProtoIndex: 2},                // 3
				{Class: "[Ljava/lang/Object;", Name: "clone", ProtoIndex: 4}, // 4
				{Class: owner, Name: "valueOf", ProtoIndex: 3},               // 5
				{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5},  // 6
			},
		}
		cls := &model.Class{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Enum;",
			AccessFlags: dexPublic | dexFinal | dexEnum,
			StaticFields: []model.EncodedField{
				{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[0]},
				{FieldIndex: 1, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[1]},
				{FieldIndex: 2, AccessFlags: dexPrivate | dexStatic | dexFinal | dexSynthetic, Field: file.Fields[2]},
			},
			DirectMethods: []model.EncodedMethod{
				{
					MethodIndex: 1,
					AccessFlags: dexPrivate | dexConstructor,
					Method:      file.Methods[1],
					Code: &model.Code{
						RegistersSize: 3, InsSize: 3, OutsSize: 3,
						Instructions: []uint16{0x3070, 0x0000, 0x0210, 0x000e},
					},
				},
				{
					MethodIndex: 2,
					AccessFlags: dexStatic | dexConstructor,
					Method:      file.Methods[2],
					Code: &model.Code{
						RegistersSize: 4, InsSize: 0, OutsSize: 3,
						Instructions: []uint16{
							0x0022, 0x0000,
							0x011a, 0x0000,
							0x0212,
							0x3070, 0x0001, 0x0210,
							0x0069, 0x0000,
							0x0022, 0x0000,
							0x011a, 0x0001,
							0x1212,
							0x3070, 0x0001, 0x0210,
							0x0069, 0x0001,
							0x2312,
							0x3023, 0x0005,
							0x0162, 0x0000,
							0x0212,
							0x014d, 0x0200,
							0x0162, 0x0001,
							0x1212,
							0x014d, 0x0200,
							0x0069, 0x0002,
							0x000e,
						},
					},
				},
				{
					MethodIndex: 3,
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[3],
					Code: &model.Code{
						RegistersSize: 1, InsSize: 0, OutsSize: 1,
						Instructions: []uint16{0x0062, 0x0002, 0x106e, 0x0004, 0x0000, 0x000c, 0x001f, 0x0005, 0x0011},
					},
				},
				{
					MethodIndex: 5,
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[5],
					Code: &model.Code{
						RegistersSize: 2, InsSize: 1, OutsSize: 2,
						Instructions: []uint16{0x001c, 0x0000, 0x2071, 0x0006, 0x0010, 0x000c, 0x001f, 0x0000, 0x0011},
					},
				},
			},
		}
		raw, err := Class(file, cls)
		if err != nil {
			t.Fatalf("strict Class failed on KtCloneColor: %v", err)
		}
		src, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Fatalf("strict Decompile failed on KtCloneColor: %v", err)
		}
		if !strings.Contains(src, "public enum KtCloneColor") || strings.Contains(src, "extends Enum") || strings.Contains(src, "ParseBytesCode failed") {
			t.Fatalf("unexpected KtCloneColor decompile output:\n%s", src)
		}
		if !strings.Contains(src, "ONE") || !strings.Contains(src, "TWO") {
			t.Fatalf("missing constants in KtCloneColor:\n%s", src)
		}
	})
}

func TestWave3MoveNullToPinnedIntNoStackMismatchVerify(t *testing.T) {
	const owner = "Ldex/MoveNullPinnedInt;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I"},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{3}},
			{ReturnType: "V"},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "legacy", ProtoIndex: 0},
			{Class: owner, Name: "main", ProtoIndex: 1},
			{Class: owner, Name: "ping", ProtoIndex: 2},
		},
	}
	var insns []uint16
	for i := 0; i < 101; i++ {
		insns = append(insns, 0x0128) // goto +1 -> exceed assignWebs cap (102*2048 > 200000)
	}
	base := uint32(len(insns))
	// v0 = const/4 0 (no pinned int demand, stays RefLocal in assignHomesLegacy);
	// move v1, v0 copies zero into v1 which is pinned Int via const/4 #3 + add-int/2addr.
	// Try/catch forces computeStackMapFrames() -> transferSlotFrame() -> verifyTagForLocalKind().
	insns = append(insns,
		0x0012,                 // +0: const/4 v0, #0
		0x0101,                 // +1: move v1, v0
		0x0071, 0x0002, 0x0000, // +2..+4: invoke-static {}, ping()V
		0x0138, 0x0006, // +5..+6: if-eqz v1, +6 -> +11 (const/4 v1, #4)
		0x3112, // +7: const/4 v1, #3
		0x5212, // +8: const/4 v2, #5
		0x21b0, // +9: add-int/2addr v1, v2
		0x010f, // +10: return v1
		0x4112, // +11: const/4 v1, #4
		0x010f, // +12: return v1
		0x000d, // +13: move-exception v0
		0x7112, // +14: const/4 v1, #7
		0x010f, // +15: return v1
	)
	catchAllAddr := base + 13
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{
					RegistersSize: 2048, InsSize: 0, OutsSize: 0,
					Instructions: insns,
					Tries: []model.Try{
						{
							StartAddress: base + 2, InstructionCount: 3,
							Handler: model.CatchHandler{CatchAllAddress: &catchAllAddr},
						},
					},
				},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // invoke-static {}, legacy()I
						0x000a, // move-result v0
						0x000e, // return-void
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 0,
					Instructions: []uint16{0x000e},
				},
			},
		},
	}
	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed: %v", err)
	}
	runMainWithVerify(t, raw, "dex.MoveNullPinnedInt")
}

func TestWave3NullMergedWithArrayAfterNonArrayRefUseVerify(t *testing.T) {
	const owner = "Ldex/NullMergedArray;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/CharSequence;"},
			{Descriptor: "[Ljava/lang/String;"},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: "Ljava/lang/NullPointerException;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V", Parameters: []string{"Ljava/lang/CharSequence;"}, ParameterTypes: []uint32{1}},
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "touch", ProtoIndex: 0},
			{Class: owner, Name: "probe", ProtoIndex: 1},
			{Class: owner, Name: "main", ProtoIndex: 2},
		},
	}
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 0, // touch(CharSequence)V
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{0x000e},
				},
			},
			{
				MethodIndex: 1, // probe(I)String
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 5, InsSize: 1, OutsSize: 1, // v4 = I arg
					Instructions: []uint16{
						0x0112,                 // 0: const/4 v1, #0
						0x1071, 0x0000, 0x0001, // 1..3: invoke-static {v1}, touch(CharSequence)V
						0x0438, 0x0004, // 4..5: if-eqz v4, +4 -> join at 8 (v1 stays Null)
						0x1312,         // 6: const/4 v3, #1
						0x3123, 0x0002, // 7..8? Wait: 6 is 1w, 7..8 is 2w -> target at 9!
					},
				},
			},
		},
	}
	// Exact word offsets for probe(I)String:
	// 0: const/4 v1, #0            (1w)
	// 1..3: touch(v1)              (3w)
	// 4..5: if-eqz v4, +5 -> 9     (2w)
	// 6: const/4 v3, #1            (1w)
	// 7..8: new-array v1, v3, [Str (2w)
	// 9: const/4 v2, #0            (1w) <- join
	// 10: array-length v0, v1      (1w)
	// 11..12: aget-object v0, v1, v2 (2w)
	// 13: return-object v0         (1w)
	cls.DirectMethods[1].Code.Instructions = []uint16{
		0x0112,                 // 0
		0x1071, 0x0000, 0x0001, // 1..3
		0x0438, 0x0005, // 4..5 -> 9
		0x1312,         // 6
		0x3123, 0x0002, // 7..8
		0x0212,         // 9
		0x1021,         // 10: array-length v0, v1
		0x0046, 0x0201, // 11..12: aget-object v0, v1, v2
		0x0011, // 13: return-object v0
	}
	cls.DirectMethods = append(cls.DirectMethods, model.EncodedMethod{
		MethodIndex: 2, // main([String;)V
		AccessFlags: dexPublic | dexStatic,
		Method:      file.Methods[2],
		Code: &model.Code{
			RegistersSize: 2, InsSize: 1, OutsSize: 1,
			Instructions: []uint16{
				0x1012,                 // 0: const/4 v0, #1 -> takes array branch
				0x1071, 0x0001, 0x0000, // 1..3: probe(1)
				0x0012,                 // 4: const/4 v0, #0 -> takes null branch
				0x1071, 0x0001, 0x0000, // 5..7: probe(0) -> throws NPE
				0x000e, // 8: return-void
				0x000d, // 9: move-exception v0
				0x000e, // 10: return-void
			},
			Tries: []model.Try{
				{
					StartAddress: 5, InstructionCount: 3,
					Handler: model.CatchHandler{Handlers: []model.TypedHandler{{TypeIndex: 5, Type: "Ljava/lang/NullPointerException;", Address: 9}}},
				},
			},
		},
	})
	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed on null merged with array after CharSequence use: %v", err)
	}
	runMainWithVerify(t, raw, "dex.NullMergedArray")
}
