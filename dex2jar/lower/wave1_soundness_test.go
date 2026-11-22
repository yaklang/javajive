package lower

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestLowerClampsDebugPositionsAndEmptyLocals(t *testing.T) {
	const owner = "Ldex/DebugClamp;"
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "I"}, {Descriptor: owner}, {Descriptor: "Ljava/lang/Object;"}},
		Protos: []model.Proto{{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}}},
		Methods: []model.MethodID{{
			Class:      owner,
			Name:       "probe",
			ProtoIndex: 0,
		}},
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 0,
			AccessFlags: dexPublic | dexStatic,
			Method:      file.Methods[0],
			Code: &model.Code{
				RegistersSize: 2,
				InsSize:       1,
				Instructions: []uint16{
					0x7012, // const/4 v0, #7
					0x000f, // return v0
				},
				Debug: &model.DebugInfo{
					LineStart:      10,
					ParameterNames: []string{"p"},
					Positions: []model.DebugPosition{
						{Address: 0, Line: 10},
						{Address: 1, Line: 11},
						{Address: 2, Line: 99},
						{Address: 50, Line: 100},
					},
					Locals: []model.DebugLocal{
						{Register: 0, Name: "valid", Type: "I", Start: 0, End: 2},
						{Register: 0, Name: "empty", Type: "I", Start: 1, End: 1},
						{Register: 1, Name: "oob", Type: "I", Start: 5, End: 10},
					},
				},
			},
		}},
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class failed on out-of-range debug entries: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse generated class failed: %v", err)
	}
	var code *javaclassparser.CodeAttribute
	for _, m := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)).Value == "probe" {
			for _, attr := range m.Attributes {
				if c, ok := attr.(*javaclassparser.CodeAttribute); ok {
					code = c
				}
			}
		}
	}
	if code == nil {
		t.Fatal("probe Code attribute missing")
	}

	var lines []uint16
	var localNames []string
	for _, attr := range code.Attributes {
		switch a := attr.(type) {
		case *javaclassparser.LineNumberTableAttribute:
			for _, entry := range a.LineNumberTable {
				if int(entry.StartPc) >= len(code.Code) {
					t.Fatalf("line entry start_pc %d >= code_length %d", entry.StartPc, len(code.Code))
				}
				lines = append(lines, entry.LineNumber)
			}
		case *javaclassparser.UnparsedAttribute:
			if a.Name == "LocalVariableTable" && len(a.Info) >= 2 {
				count := int(binary.BigEndian.Uint16(a.Info[:2]))
				for i := 0; i < count && 2+(i+1)*10 <= len(a.Info); i++ {
					entry := a.Info[2+i*10 : 2+(i+1)*10]
					startPC := int(binary.BigEndian.Uint16(entry[0:2]))
					length := int(binary.BigEndian.Uint16(entry[2:4]))
					nameIdx := int(binary.BigEndian.Uint16(entry[4:6]))
					if length == 0 || startPC+length > len(code.Code) {
						t.Fatalf("local entry has out-of-bounds range [%d, +%d) vs code_length %d", startPC, length, len(code.Code))
					}
					localNames = append(localNames, parsed.ConstantPoolManager.GetUtf8(nameIdx).Value)
				}
			}
		}
	}
	if len(lines) != 2 || lines[0] != 10 || lines[1] != 11 {
		t.Fatalf("LineNumberTable lines = %v, want [10 11]", lines)
	}
	joinedLocals := strings.Join(localNames, ",")
	if !strings.Contains(joinedLocals, "valid") || strings.Contains(joinedLocals, "empty") || strings.Contains(joinedLocals, "oob") {
		t.Fatalf("LocalVariableTable names = %v, want valid without empty/oob", localNames)
	}
}

func TestLowerNarrowsTryCatchExceptionTableAndStackmapOverNonThrowingMoves(t *testing.T) {
	const owner = "Ldex/TryNarrow;"
	protos := []model.Proto{
		{ReturnType: "V"},
		{ReturnType: "I"},
		{ReturnType: "Ljava/lang/Object;", Parameters: []string{"Ljava/lang/Object;"}, ParameterTypes: []uint32{0}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
		{ReturnType: "Z", Parameters: []string{"Ljava/lang/Object;"}, ParameterTypes: []uint32{0}},
	}
	methods := []model.MethodID{
		{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
		{Class: "Ljava/lang/Object;", Name: "hashCode", ProtoIndex: 1},
		{Class: owner, Name: "probe", ProtoIndex: 2},
		{Class: owner, Name: "main", ProtoIndex: 3},
		{Class: "Ljava/lang/Object;", Name: "equals", ProtoIndex: 4},
	}

	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"fallback", "ok", "x"},
		Types: []model.Type{
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "[Ljava/lang/String;"},
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/NullPointerException;"},
		},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[2],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x001a, 0x0000, // PC 0..1: const-string v0, "fallback"
						0x2107,                 // PC 2:    move-object v1, v2 (non-throwing leading insn inside try)
						0x106e, 0x0001, 0x0001, // PC 3..5: invoke-virtual {v1}, Object.hashCode()I (throwing)
						0x000a,         // PC 6:    move-result v0 (non-throwing overwrite of v0 from Object to int!)
						0x001a, 0x0001, // PC 7..8: const-string v0, "ok"
						0x0011, // PC 9:    return-object v0
						0x010d, // PC 10:   move-exception v1 (handler)
						0x0011, // PC 11:   return-object v0 (expects v0 still Object, NOT int)
					},
					Tries: []model.Try{{
						StartAddress:     2,
						InstructionCount: 5,
						Handler: model.CatchHandler{
							Handlers: []model.TypedHandler{{
								TypeIndex: 3,
								Type:      "Ljava/lang/NullPointerException;",
								Address:   10,
							}},
						},
					}},
				},
			},
			{
				MethodIndex: 3,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[3],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x001a, 0x0002, // PC 0..1: const-string v0, "x"
						0x1071, 0x0002, 0x0000, // PC 2..4: invoke-static {v0}, probe
						0x000c,         // PC 5:    move-result-object v0
						0x011a, 0x0001, // PC 6..7: const-string v1, "ok"
						0x206e, 0x0004, 0x0010, // PC 8..10: invoke-virtual {v0, v1}, Object.equals(Object)Z
						0x000a,         // PC 11:   move-result v0
						0x0039, 0x0004, // PC 12..13: if-nez v0, +4 (PC 16)
						0x0012,                 // PC 14: const/4 v0, #0
						0x0027,                 // PC 15: throw v0
						0x0012,                 // PC 16: const/4 v0, #0
						0x1071, 0x0002, 0x0000, // PC 17..19: invoke-static {v0}, probe(null)
						0x000c,         // PC 20: move-result-object v0
						0x011a, 0x0000, // PC 21..22: const-string v1, "fallback"
						0x206e, 0x0004, 0x0010, // PC 23..25: invoke-virtual {v0, v1}, Object.equals(Object)Z
						0x000a,         // PC 26: move-result v0
						0x0039, 0x0004, // PC 27..28: if-nez v0, +4 (PC 31)
						0x0012, // PC 29: const/4 v0, #0
						0x0027, // PC 30: throw v0
						0x000e, // PC 31: return-void
					},
				},
			},
		},
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class failed: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse generated class failed: %v", err)
	}
	var probeCode *javaclassparser.CodeAttribute
	for _, m := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)).Value == "probe" {
			for _, attr := range m.Attributes {
				if c, ok := attr.(*javaclassparser.CodeAttribute); ok {
					probeCode = c
				}
			}
		}
	}
	if probeCode == nil || len(probeCode.ExceptionTable) != 1 {
		t.Fatalf("probe ExceptionTable = %#v, want 1 narrowed entry", probeCode)
	}
	exc := probeCode.ExceptionTable[0]
	if int(exc.EndPc) >= len(probeCode.Code)-4 || exc.StartPc == 0 {
		t.Fatalf("ExceptionTable entry [%d, %d) not narrowed tightly around invokevirtual (code_length=%d)",
			exc.StartPc, exc.EndPc, len(probeCode.Code))
	}

	java, err := exec.LookPath("java")
	if err != nil {
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "TryNarrow.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.TryNarrow").CombinedOutput(); err != nil {
		t.Fatalf("java -Xverify:all execution failed: %v\n%s", err, out)
	}
}

func TestLowerConstructorThisAliasOverwriteAndDeferredIput(t *testing.T) {
	const owner = "Ldex/AliasCtor;"
	field := model.FieldID{Class: owner, Name: "x", Type: "I"}
	protos := []model.Proto{
		{ReturnType: "V"},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
	}
	methods := []model.MethodID{
		{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
		{Class: owner, Name: "<init>", ProtoIndex: 0},
		{Class: owner, Name: "main", ProtoIndex: 1},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "Ljava/lang/Object;"}, {Descriptor: "[Ljava/lang/String;"}, {Descriptor: owner}},
		Protos:  protos,
		Fields:  []model.FieldID{field},
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:     owner,
		Superclass:     "Ljava/lang/Object;",
		AccessFlags:    dexPublic,
		InstanceFields: []model.EncodedField{{FieldIndex: 0, AccessFlags: dexPrivate, Field: field}},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexConstructor,
				Method:      methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x2007,         // PC 0:    move-object v0, v2 (alias uninit this from thisRegister v2)
						0x5212,         // PC 1:    const/4 v2, #5 (overwrite thisRegister v2!)
						0x0259, 0x0000, // PC 2..3: iput v2, v0, Foo.x (deferred store on v0)
						0x1070, 0x0000, 0x0000, // PC 4..6: invoke-direct {v0}, Object.<init>()V
						0x000e, // PC 7:    return-void
					},
				},
			},
			{
				MethodIndex: 2,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[2],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // PC 0..1: new-instance v0, Ldex/AliasCtor;
						0x1070, 0x0001, 0x0000, // PC 2..4: invoke-direct {v0}, AliasCtor.<init>()V
						0x0052, 0x0000, // PC 5..6: iget v0, v0, Foo.x
						0x5112,         // PC 7:    const/4 v1, #5
						0x1032, 0x0004, // PC 8..9: if-eq v0, v1, +4 (PC 12)
						0x0012, // PC 10:   const/4 v0, #0
						0x0027, // PC 11:   throw v0
						0x000e, // PC 12:   return-void
					},
				},
			},
		},
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class failed on aliased/overwritten this constructor: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("Parse generated class failed: %v", err)
	}

	java, err := exec.LookPath("java")
	if err != nil {
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "AliasCtor.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.AliasCtor").CombinedOutput(); err != nil {
		t.Fatalf("java -Xverify:all execution failed: %v\n%s", err, out)
	}
}
