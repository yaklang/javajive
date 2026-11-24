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

func TestLowerCanonicalEnumHoistedOrdinalConstants(t *testing.T) {
	const owner = "Ldex/enumcanon/State;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"DESTROYED", "INITIALIZED"},
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "[" + owner},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "I"},
			{Descriptor: "Ljava/lang/Object;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}}, // 1: (String, I)V
			{ReturnType: "[" + owner}, // 2: ()[State
			{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}}, // 3: (String)State
			{ReturnType: "Ljava/lang/Object;"}, // 4: ()Object
			{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{4, 2}}, // 5: valueOf
		},
		Fields: []model.FieldID{
			{Class: owner, Name: "DESTROYED", Type: owner},     // 0
			{Class: owner, Name: "INITIALIZED", Type: owner},   // 1
			{Class: owner, Name: "$VALUES", Type: "[" + owner}, // 2
			{Class: owner, Name: "COUNT", Type: "I"},           // 3
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 1},    // 0
			{Class: owner, Name: "<init>", ProtoIndex: 1},                 // 1
			{Class: owner, Name: "<clinit>", ProtoIndex: 0},               // 2
			{Class: owner, Name: "$values", ProtoIndex: 2},                // 3
			{Class: owner, Name: "values", ProtoIndex: 2},                 // 4
			{Class: owner, Name: "valueOf", ProtoIndex: 3},                // 5
			{Class: "Ljava/lang/Object;", Name: "clone", ProtoIndex: 4},   // 6
			{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5},   // 7
		},
	}

	// <clinit> with R8 hoisted ordinal constants (const/4 v1=0; const/4 v2=1 BEFORE new-instance)
	// plus an additional non-enum static field sharing hoisted const v3=2 (must NOT be skipped!).
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Enum;",
		AccessFlags: 0x4011,
		StaticFields: []model.EncodedField{
			{FieldIndex: 0, AccessFlags: 0x4019, Field: file.Fields[0]},
			{FieldIndex: 1, AccessFlags: 0x4019, Field: file.Fields[1]},
			{FieldIndex: 2, AccessFlags: 0x101a, Field: file.Fields[2]},
			{FieldIndex: 3, AccessFlags: 0x0019, Field: file.Fields[3]},
		},
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPrivate | 0x10000,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 3, OutsSize: 3,
					Instructions: []uint16{0x3070, 0x0000, 0x0210, 0x000e},
				},
			},
			{
				AccessFlags: 0x100a,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 0, OutsSize: 0,
					Instructions: []uint16{
						0x2012,
						0x0023, 0x0001,
						0x0112,
						0x0262, 0x0000,
						0x024d, 0x0100,
						0x1112,
						0x0262, 0x0001,
						0x024d, 0x0100,
						0x0011,
					},
				},
			},
			{
				AccessFlags: 0x0009,
				Method:      file.Methods[4], // values()
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0062, 0x0002,
						0x106e, 0x0006, 0x0000,
						0x000c,
						0x001f, 0x0001,
						0x0011,
					},
				},
			},
			{
				AccessFlags: 0x0009,
				Method:      file.Methods[5], // valueOf(String)
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x001c, 0x0000, // const-class v0, State
						0x2071, 0x0007, 0x0010, // invoke-static {v0, v1}, Enum.valueOf(Class, String)Enum
						0x000c,         // move-result-object v0
						0x001f, 0x0000, // check-cast v0, State
						0x0011,         // return-object v0
					},
				},
			},
			{
				AccessFlags: dexStatic | 0x10008,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 5, InsSize: 0, OutsSize: 3,
					Instructions: []uint16{
						0x0112,                 // const/4 v1, #0  (hoisted ordinal 0, dead outside spans)
						0x1212,                 // const/4 v2, #1  (hoisted ordinal 1, dead outside spans)
						0x2312,                 // const/4 v3, #2  (shared non-enum constant, live at sput!)
						0x0022, 0x0000,         // new-instance v0, State
						0x041a, 0x0000,         // const-string v4, "DESTROYED"
						0x3070, 0x0001, 0x0140, // invoke-direct {v0, v4, v1}, State.<init>
						0x0069, 0x0000,         // sput-object v0, DESTROYED
						0x0022, 0x0000,         // new-instance v0, State
						0x041a, 0x0001,         // const-string v4, "INITIALIZED"
						0x3070, 0x0001, 0x0240, // invoke-direct {v0, v4, v2}, State.<init>
						0x0069, 0x0001,         // sput-object v0, INITIALIZED
						0x0071, 0x0003, 0x0000, // invoke-static {}, $values()
						0x000c,                 // move-result-object v0
						0x0069, 0x0002,         // sput-object v0, $VALUES
						0x0367, 0x0003,         // sput v3, COUNT
						0x000e,                 // return-void
					},
				},
			},
		},
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	res, err := javaclassparser.DecompileWithOptions(raw, javaclassparser.DecompileOptions{
		Mode:           javaclassparser.Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("DecompileWithOptions: %v", err)
	}
	if !strings.Contains(res.Source, "public enum State") || !strings.Contains(res.Source, "DESTROYED") || !strings.Contains(res.Source, "INITIALIZED") {
		t.Fatalf("expected canonical `public enum State`, got:\n%s", res.Source)
	}

	// Real JVM check: hoisted ordinal spans initialize both enum constants AND non-fused COUNT=2 remains intact!
	if javac, err1 := exec.LookPath("javac"); err1 == nil {
		if java, err2 := exec.LookPath("java"); err2 == nil {
			dir := t.TempDir()
			outPath := filepath.Join(dir, "dex", "enumcanon", "State.class")
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outPath, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			driver := filepath.Join(dir, "Driver.java")
			source := `public class Driver {
    public static void main(String[] args) {
        if (dex.enumcanon.State.DESTROYED.ordinal() != 0) throw new AssertionError("DESTROYED ordinal");
        if (dex.enumcanon.State.INITIALIZED.ordinal() != 1) throw new AssertionError("INITIALIZED ordinal");
        if (dex.enumcanon.State.COUNT != 2) throw new AssertionError("COUNT=" + dex.enumcanon.State.COUNT);
    }
}`
			if err := os.WriteFile(driver, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
				t.Fatalf("javac driver: %v\n%s", err, output)
			}
			if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
				t.Fatalf("java -Xverify:all failed: %v\n%s", err, output)
			}
		}
	}
}

func TestLowerCanonicalEnumFilledNewArrayAndKotlinEntries(t *testing.T) {
	const owner = "Ldex/enumcanon/ResolvedDir;"
	const entriesIface = "Lkotlin/enums/EnumEntries;"
	const entriesKt = "Lkotlin/enums/EnumEntriesKt;"

	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"Ltr", "Rtl"},
		Types: []model.Type{
			{Descriptor: owner},                         // 0
			{Descriptor: "[" + owner},                   // 1
			{Descriptor: "Ljava/lang/String;"},          // 2
			{Descriptor: "I"},                           // 3
			{Descriptor: "Ljava/lang/Object;"},          // 4
			{Descriptor: "[Ljava/lang/Enum;"},           // 5
			{Descriptor: entriesIface},                  // 6
		},
		Protos: []model.Proto{
			{ReturnType: "V"}, // 0: ()V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}}, // 1: (String, I)V
			{ReturnType: "[" + owner}, // 2: ()[ResolvedDir
			{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}}, // 3: (String)ResolvedDir
			{ReturnType: "Ljava/lang/Object;"}, // 4: ()Object
			{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{4, 2}}, // 5: valueOf
			{ReturnType: entriesIface, Parameters: []string{"[Ljava/lang/Enum;"}, ParameterTypes: []uint32{5}}, // 6: enumEntries([Enum)EnumEntries
		},
		Fields: []model.FieldID{
			{Class: owner, Name: "Ltr", Type: owner},           // 0
			{Class: owner, Name: "Rtl", Type: owner},           // 1
			{Class: owner, Name: "$VALUES", Type: "[" + owner}, // 2
			{Class: owner, Name: "$ENTRIES", Type: entriesIface}, // 3
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 1},        // 0
			{Class: owner, Name: "<init>", ProtoIndex: 1},                     // 1
			{Class: owner, Name: "<clinit>", ProtoIndex: 0},                   // 2
			{Class: owner, Name: "values", ProtoIndex: 2},                     // 3
			{Class: owner, Name: "valueOf", ProtoIndex: 3},                    // 4
			{Class: "Ljava/lang/Object;", Name: "clone", ProtoIndex: 4},       // 5
			{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5},       // 6
			{Class: entriesKt, Name: "enumEntries", ProtoIndex: 6},            // 7
		},
	}

	// Models Kotlin/R8 inlined $values() using filled-new-array {v0, v1} + $ENTRIES.
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Enum;",
		AccessFlags: 0x4011,
		StaticFields: []model.EncodedField{
			{FieldIndex: 0, AccessFlags: 0x4019, Field: file.Fields[0]}, // Ltr
			{FieldIndex: 1, AccessFlags: 0x4019, Field: file.Fields[1]}, // Rtl
			{FieldIndex: 2, AccessFlags: 0x101a, Field: file.Fields[2]}, // $VALUES
			{FieldIndex: 3, AccessFlags: 0x101a, Field: file.Fields[3]}, // $ENTRIES
		},
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPrivate | 0x10000,
				Method:      file.Methods[1], // <init>(String, I)
				Code: &model.Code{
					RegistersSize: 3, InsSize: 3, OutsSize: 3,
					Instructions: []uint16{0x3070, 0x0000, 0x0210, 0x000e},
				},
			},
			{
				AccessFlags: 0x0009,
				Method:      file.Methods[3], // values()
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0062, 0x0002,
						0x106e, 0x0005, 0x0000,
						0x000c,
						0x001f, 0x0001,
						0x0011,
					},
				},
			},
			{
				AccessFlags: 0x0009,
				Method:      file.Methods[4], // valueOf(String)
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x001c, 0x0000,
						0x2071, 0x0006, 0x0010,
						0x000c,
						0x001f, 0x0000,
						0x0011,
					},
				},
			},
			{
				AccessFlags: dexStatic | 0x10008,
				Method:      file.Methods[2], // <clinit>
				Code: &model.Code{
					RegistersSize: 4, InsSize: 0, OutsSize: 3,
					Instructions: []uint16{
						0x0022, 0x0000,         // new-instance v0, ResolvedDir
						0x021a, 0x0000,         // const-string v2, "Ltr"
						0x0312,                 // const/4 v3, #0
						0x3070, 0x0001, 0x0320, // invoke-direct {v0, v2, v3}, ResolvedDir.<init>
						0x0069, 0x0000,         // sput-object v0, Ltr
						0x0122, 0x0000,         // new-instance v1, ResolvedDir
						0x021a, 0x0001,         // const-string v2, "Rtl"
						0x1312,                 // const/4 v3, #1
						0x3070, 0x0001, 0x0321, // invoke-direct {v1, v2, v3}, ResolvedDir.<init>
						0x0169, 0x0001,         // sput-object v1, Rtl
						0x2024, 0x0001, 0x0010, // filled-new-array {v0, v1}, [ResolvedDir
						0x000c,                 // move-result-object v0
						0x0069, 0x0002,         // sput-object v0, $VALUES
						0x1071, 0x0007, 0x0000, // invoke-static {v0}, EnumEntriesKt.enumEntries([Enum)EnumEntries
						0x000c,                 // move-result-object v0
						0x0069, 0x0003,         // sput-object v0, $ENTRIES
						0x000e,                 // return-void
					},
				},
			},
		},
	}

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	res, err := javaclassparser.DecompileWithOptions(raw, javaclassparser.DecompileOptions{
		Mode:           javaclassparser.Compatibility,
		EnableShadowIR: true,
	})
	if err != nil {
		t.Fatalf("DecompileWithOptions: %v", err)
	}
	if !strings.Contains(res.Source, "public enum ResolvedDir") || !strings.Contains(res.Source, "Ltr") || !strings.Contains(res.Source, "Rtl") {
		t.Fatalf("expected canonical `public enum ResolvedDir`, got:\n%s", res.Source)
	}

	if javac, err1 := exec.LookPath("javac"); err1 == nil {
		if java, err2 := exec.LookPath("java"); err2 == nil {
			dir := t.TempDir()
			kotlinStubDir := filepath.Join(dir, "kotlin", "enums")
			if err := os.MkdirAll(kotlinStubDir, 0o755); err != nil {
				t.Fatal(err)
			}
			stubCode := `package kotlin.enums;
public interface EnumEntries {}
`
			ktSrc := filepath.Join(kotlinStubDir, "EnumEntriesKt.java")
			ktCode := `package kotlin.enums;
public final class EnumEntriesKt {
    public static EnumEntries enumEntries(Enum<?>[] entries) {
        return new EnumEntries() {};
    }
}
`
			if err := os.WriteFile(filepath.Join(kotlinStubDir, "EnumEntries.java"), []byte(stubCode), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(ktSrc, []byte(ktCode), 0o644); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(javac, "-d", dir, filepath.Join(kotlinStubDir, "EnumEntries.java"), ktSrc).CombinedOutput(); err != nil {
				t.Fatalf("javac kotlin stubs: %v\n%s", err, output)
			}
			outPath := filepath.Join(dir, "dex", "enumcanon", "ResolvedDir.class")
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outPath, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			driver := filepath.Join(dir, "Driver.java")
			driverSrc := `public class Driver {
    public static void main(String[] args) {
        dex.enumcanon.ResolvedDir[] vals = dex.enumcanon.ResolvedDir.values();
        if (vals.length != 2 || vals[0] != dex.enumcanon.ResolvedDir.Ltr || vals[1] != dex.enumcanon.ResolvedDir.Rtl) {
            throw new AssertionError("unexpected enum values");
        }
    }
}`
			if err := os.WriteFile(driver, []byte(driverSrc), 0o644); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(javac, "-cp", dir, "-d", dir, driver).CombinedOutput(); err != nil {
				t.Fatalf("javac driver: %v\n%s", err, output)
			}
			if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "Driver").CombinedOutput(); err != nil {
				t.Fatalf("java -Xverify:all failed: %v\n%s", err, output)
			}
		}
	}
}
