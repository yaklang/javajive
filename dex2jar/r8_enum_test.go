package dex2jar

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func readZipEntry(t *testing.T, jarBytes []byte, entry string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entry, err)
		}
		var buf bytes.Buffer
		_, copyErr := buf.ReadFrom(rc)
		_ = rc.Close()
		if copyErr != nil {
			t.Fatalf("read %s: %v", entry, copyErr)
		}
		return buf.Bytes()
	}
	t.Fatalf("entry %s not found in JAR", entry)
	return nil
}

func r8WidenedEnumFixture(withExternalValuesReader bool) *model.Application {
	const (
		enumDesc  = "Lr8/Mode;"
		otherDesc = "Lr8/OtherReader;"
	)
	file := &model.File{
		Entry: "classes.dex",
		Strings: []string{
			"FAST", "SAFE",
		},
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: enumDesc},
			{Descriptor: "[" + enumDesc},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "I"},
			{Descriptor: "V"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{3, 4}}, // 1: (String, I)V
			{ReturnType: "[" + enumDesc}, // 2: ()[Lr8/Mode;
			{ReturnType: "Ljava/lang/Object;"}, // 3
			{ReturnType: enumDesc, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{3}}, // 4: (String)Mode
			{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{0, 3}}, // 5: Enum.valueOf
		},
		Fields: []model.FieldID{
			{Class: enumDesc, Name: "FAST", Type: enumDesc},          // 0
			{Class: enumDesc, Name: "SAFE", Type: enumDesc},          // 1
			{Class: enumDesc, Name: "$VALUES", Type: "[" + enumDesc}, // 2
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 1}, // 0
			{Class: enumDesc, Name: "<init>", ProtoIndex: 1},           // 1
			{Class: enumDesc, Name: "<clinit>", ProtoIndex: 0},         // 2
			{Class: enumDesc, Name: "values", ProtoIndex: 2},           // 3
			{Class: otherDesc, Name: "read", ProtoIndex: 2},            // 4
			{Class: enumDesc, Name: "$values", ProtoIndex: 2},          // 5
		},
	}

	mode := model.Class{
		Descriptor:  enumDesc,
		Superclass:  "Ljava/lang/Enum;",
		AccessFlags: 0x4011, // public final enum
		StaticFields: []model.EncodedField{
			{FieldIndex: 0, AccessFlags: 0x4019, Field: file.Fields[0]},
			{FieldIndex: 1, AccessFlags: 0x4019, Field: file.Fields[1]},
			// R8 widened $VALUES to public static final synthetic (0x1019) instead of 0x101a.
			{FieldIndex: 2, AccessFlags: 0x1019, Field: file.Fields[2]},
		},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				// R8 widened enum constructor to public synthetic (0x1001) instead of private (0x2).
				AccessFlags: 0x11001,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 3, OutsSize: 3,
					Instructions: []uint16{
						0x3070, 0x0000, 0x0210, // invoke-direct {v0, v1, v2}, Enum.<init>(String, I)V
						0x000e,
					},
				},
			},
			{
				MethodIndex: 5,
				AccessFlags: 0x100a, // private static synthetic $values()
				Method:      file.Methods[5],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 0, OutsSize: 0,
					Instructions: []uint16{
						0x2012, // const/4 v0, #2
						0x0023, 0x0002, // new-array v0, v0, [Lr8/Mode;
						0x0112, // const/4 v1, #0
						0x0262, 0x0000, // sget-object v2, FAST
						0x014d, 0x0002, // aput-object v2, v0, v1 -> Wait: 23x 0x4d aa=v2 bb=v0 cc=v1 -> 0x024d, 0x0100
						0x000e,
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: 0x10008,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 0, OutsSize: 3,
					Instructions: []uint16{
						0x0022, 0x0001, // new-instance v0, Lr8/Mode;
						0x011a, 0x0000, // const-string v1, "FAST"
						0x0212,         // const/4 v2, #0
						0x3070, 0x0001, 0x0210, // invoke-direct {v0, v1, v2}, Mode.<init>
						0x0069, 0x0000, // sput-object v0, FAST
						0x0022, 0x0001, // new-instance v0, Lr8/Mode;
						0x011a, 0x0001, // const-string v1, "SAFE"
						0x1212,         // const/4 v2, #1
						0x3070, 0x0001, 0x0210,
						0x0069, 0x0001, // sput-object v0, SAFE
						0x0071, 0x0005, 0x0000, // invoke-static {}, $values()
						0x000c,         // move-result-object v0
						0x0069, 0x0002, // sput-object v0, $VALUES
						0x000e,
					},
				},
			},
			{
				MethodIndex: 3,
				AccessFlags: 0x0009,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 2, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0062, 0x0002, // sget-object v0, $VALUES
						0x106e, 0x0006, 0x0000, // placeholder; invoke-virtual clone appended below
						0x000e,
					},
				},
			},
		},
	}
	// Fix $values aput-object instructions cleanly:
	// 0: 0x2012 (const/4 v0, 2)
	// 1..2: 0x0023, 0x0002 (new-array v0, v0, [Lr8/Mode;)
	// 3: 0x0112 (const/4 v1, 0)
	// 4..5: 0x0262, 0x0000 (sget-object v2, FAST)
	// 6..7: 0x024d, 0x0100 (aput-object v2, v0, v1)
	// 8: 0x1112 (const/4 v1, 1)
	// 9..10: 0x0262, 0x0001 (sget-object v2, SAFE)
	// 11..12: 0x024d, 0x0100 (aput-object v2, v0, v1)
	// 13: 0x0011 (return-object v0)
	mode.DirectMethods[1].Code.Instructions = []uint16{
		0x2012,
		0x0023, 0x0002,
		0x0112,
		0x0262, 0x0000,
		0x024d, 0x0100,
		0x1112,
		0x0262, 0x0001,
		0x024d, 0x0100,
		0x0011,
	}
	// Fix values() clone + check-cast + return-object, plus canonical valueOf(String).
	file.Methods = append(file.Methods,
		model.MethodID{Class: "Ljava/lang/Object;", Name: "clone", ProtoIndex: 3},
		model.MethodID{Class: enumDesc, Name: "valueOf", ProtoIndex: 4},
		model.MethodID{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5},
	)
	cloneIdx := uint16(len(file.Methods) - 3)
	valueOfSelfIdx := uint16(len(file.Methods) - 2)
	enumValueOfIdx := uint16(len(file.Methods) - 1)
	mode.DirectMethods[3].Code.Instructions = []uint16{
		0x0062, 0x0002, // sget-object v0, $VALUES
		0x106e, cloneIdx, 0x0000, // invoke-virtual {v0}, Object.clone()Object
		0x000c,         // move-result-object v0
		0x001f, 0x0002, // check-cast v0, [Lr8/Mode;
		0x0011,         // return-object v0
	}
	mode.DirectMethods = append(mode.DirectMethods, model.EncodedMethod{
		MethodIndex: uint32(valueOfSelfIdx),
		AccessFlags: 0x0009,
		Method:      file.Methods[valueOfSelfIdx],
		Code: &model.Code{
			RegistersSize: 2, InsSize: 1, OutsSize: 2,
			Instructions: []uint16{
				0x001c, 0x0001, // const-class v0, Lr8/Mode;
				0x2071, enumValueOfIdx, 0x0010, // invoke-static {v0, v1}, Enum.valueOf(Class, String)Enum
				0x000c,         // move-result-object v0
				0x001f, 0x0001, // check-cast v0, Lr8/Mode;
				0x0011,         // return-object v0
			},
		},
	})

	classes := []model.Class{mode}
	if withExternalValuesReader {
		other := model.Class{
			Descriptor:  otherDesc,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: 0x0001,
			DirectMethods: []model.EncodedMethod{{
				MethodIndex: 4,
				AccessFlags: 0x0009,
				Method:      file.Methods[4],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 0,
					Instructions: []uint16{
						0x0062, 0x0002, // external sget-object v0, Mode.$VALUES
						0x0011,
					},
				},
			}},
		}
		classes = append(classes, other)
	}
	file.Classes = classes
	return &model.Application{Files: []*model.File{file}}
}

func TestNormalizeR8EnumDeclarationsInternalAndExternal(t *testing.T) {
	// Strict mode must leave widened flags untouched.
	strictApp := r8WidenedEnumFixture(false)
	if _, _, err := applicationToJarWithPolicy(strictApp, FailureStrict); err != nil {
		t.Fatalf("FailureStrict failed: %v", err)
	}
	if strictApp.Files[0].Classes[0].StaticFields[2].AccessFlags != 0x1019 {
		t.Fatalf("FailureStrict unexpectedly mutated $VALUES flags: 0x%x", strictApp.Files[0].Classes[0].StaticFields[2].AccessFlags)
	}

	// FailureRecover with zero external refs normalizes $VALUES -> 0x101a and <init> -> 0x10002,
	// allowing Stage B to decompile cleanly to `public enum Mode`.
	recApp := r8WidenedEnumFixture(false)
	jarBytes, _, err := applicationToJarWithPolicy(recApp, FailureRecover)
	if err != nil {
		t.Fatalf("FailureRecover failed: %v", err)
	}
	clsBytes := readZipEntry(t, jarBytes, "r8/Mode.class")
	res, err := javaclassparser.DecompileWithOptions(clsBytes, javaclassparser.DecompileOptions{
		Mode:           javaclassparser.Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("DecompileWithOptions(r8/Mode): %v", err)
	}
	if !strings.Contains(res.Source, "public enum Mode") {
		t.Fatalf("expected canonical `public enum Mode`, got:\n%s", res.Source)
	}

	// With an external $VALUES reader, FailureRecover MUST NOT narrow $VALUES to private (0x101a)
	// so external readers never hit IllegalAccessError.
	extApp := r8WidenedEnumFixture(true)
	if _, _, err := applicationToJarWithPolicy(extApp, FailureRecover); err != nil {
		t.Fatalf("FailureRecover with external $VALUES reader failed: %v", err)
	}
	if got := extApp.Files[0].Classes[0].StaticFields[2].AccessFlags; got != 0x1019 {
		t.Fatalf("$VALUES with external reader was narrowed to 0x%x, want 0x1019 preserved", got)
	}
}
