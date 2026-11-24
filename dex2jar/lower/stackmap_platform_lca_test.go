package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestStackMapPlatformHierarchyLCA(t *testing.T) {
	lowerer := &methodLowerer{
		hierarchy: map[string]*model.Class{
			"Lapp/CustomViewGroup;": {
				Descriptor: "Lapp/CustomViewGroup;",
				Superclass: "Landroid/view/ViewGroup;",
			},
			"Lapp/CustomReflectiveException;": {
				Descriptor: "Lapp/CustomReflectiveException;",
				Superclass: "Ljava/lang/IllegalAccessException;",
			},
		},
	}

	if got := lowerer.commonVerifyClass("app/CustomViewGroup", "android/view/View"); got != "android/view/View" {
		t.Fatalf("LCA(app/CustomViewGroup, android/view/View) = %q, want android/view/View", got)
	}
	if got := lowerer.commonVerifyClass("java/lang/IllegalAccessException", "java/lang/NoSuchMethodException"); got != "java/lang/ReflectiveOperationException" {
		t.Fatalf("LCA(IllegalAccessException, NoSuchMethodException) = %q, want ReflectiveOperationException", got)
	}
	if got := lowerer.commonVerifyClass("java/lang/Error", "java/lang/RuntimeException"); got != "java/lang/Throwable" {
		t.Fatalf("LCA(Error, RuntimeException) = %q, want java/lang/Throwable", got)
	}
	if got := lowerer.commonVerifyClass("java/lang/Integer", "java/lang/Float"); got != "java/lang/Number" {
		t.Fatalf("LCA(Integer, Float) = %q, want java/lang/Number", got)
	}
	if !lowerer.isVerifySubtype("app/CustomViewGroup", "android/view/View") {
		t.Fatal("expected app/CustomViewGroup to be a subtype of android/view/View")
	}
	if !lowerer.isVerifySubtype("java/lang/ReflectiveOperationException", "java/lang/Throwable") {
		t.Fatal("expected ReflectiveOperationException to be a subtype of java/lang/Throwable")
	}
	if !lowerer.isProvablyThrowable("app/CustomReflectiveException") {
		t.Fatal("expected app/CustomReflectiveException (via IllegalAccessException) to be provably throwable")
	}
}

func TestLowerPlatformHierarchyMergesVerifyWithJDK21(t *testing.T) {
	const faceDesc = "Ldex/lca/Face;"
	const ownerDesc = "Ldex/lca/PlatformLca;"

	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: faceDesc},
			{Descriptor: ownerDesc},
			{Descriptor: "I"},
			{Descriptor: "F"},
			{Descriptor: "Z"},
			{Descriptor: "Ljava/lang/Error;"},
			{Descriptor: "Ljava/lang/RuntimeException;"},
			{Descriptor: "Ljava/lang/IllegalAccessException;"},
			{Descriptor: "Ljava/lang/NoSuchMethodException;"},
			{Descriptor: "Ljava/lang/Throwable;"},
			{Descriptor: "Ljava/lang/Number;"},
			{Descriptor: "Ljava/lang/Integer;"},
			{Descriptor: "Ljava/lang/Float;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I"},                                                                            // 0: ()I
			{ReturnType: "V", Parameters: []string{"Ljava/lang/Throwable;"}, ParameterTypes: []uint32{9}}, // 1: (Throwable)V
			{ReturnType: "V", Parameters: []string{"Ljava/lang/Number;"}, ParameterTypes: []uint32{10}},   // 2: (Number)V
			{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{2}},                     // 3: (I)V
			{ReturnType: "V", Parameters: []string{"Z"}, ParameterTypes: []uint32{4}},                     // 4: (Z)V
			{ReturnType: "Ljava/lang/Integer;", Parameters: []string{"I"}, ParameterTypes: []uint32{2}},   // 5: (I)Integer
			{ReturnType: "Ljava/lang/Float;", Parameters: []string{"F"}, ParameterTypes: []uint32{3}},     // 6: (F)Float
		},
		Methods: []model.MethodID{
			{Class: faceDesc, Name: "ping", ProtoIndex: 0},                        // 0
			{Class: ownerDesc, Name: "consumeThrowable", ProtoIndex: 1},           // 1
			{Class: ownerDesc, Name: "consumeNumber", ProtoIndex: 2},              // 2
			{Class: ownerDesc, Name: "catchErrorAndRuntime", ProtoIndex: 3},       // 3
			{Class: ownerDesc, Name: "catchReflectiveMultiCatch", ProtoIndex: 3},  // 4
			{Class: ownerDesc, Name: "mergeIntegerAndFloat", ProtoIndex: 4},       // 5
			{Class: "Ljava/lang/Integer;", Name: "valueOf", ProtoIndex: 5},        // 6
			{Class: "Ljava/lang/Float;", Name: "valueOf", ProtoIndex: 6},          // 7
		},
	}

	faceClass := &model.Class{
		Descriptor:  faceDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic | dexInterface | dexAbstract,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code:        &model.Code{RegistersSize: 1, Instructions: []uint16{0x7012, 0x000f}},
			},
		},
	}

	ownerClass := &model.Class{
		Descriptor:  ownerDesc,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1], // consumeThrowable(Throwable)V
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2], // consumeNumber(Number)V
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[3], // catchErrorAndRuntime(I)V (branch-merge Error + RuntimeException -> Throwable)
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x000a,                 // 3: move-result v0
						0x0093, 0x0200,         // 4..5: div-int v0, v0, v2
						0x000e,                 // 6: return-void
						0x000d,                 // 7: move-exception v0 (Error)
						0x0228,                 // 8: goto +2 (target 10)
						0x000d,                 // 9: move-exception v0 (RuntimeException)
						0x1071, 0x0001, 0x0000, // 10..12: invoke-static {v0}, consumeThrowable(Throwable)V
						0x000e,                 // 13: return-void
					},
					Tries: []model.Try{
						{
							StartAddress:     4,
							InstructionCount: 2,
							Handler: model.CatchHandler{
								Handlers: []model.TypedHandler{
									{Type: "Ljava/lang/Error;", Address: 7},
									{Type: "Ljava/lang/RuntimeException;", Address: 9},
								},
							},
						},
					},
				},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[4], // catchReflectiveMultiCatch(I)V (multi-catch IllegalAccessException + NoSuchMethodException -> ReflectiveOperationException)
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x000a,                 // 3: move-result v0
						0x0093, 0x0200,         // 4..5: div-int v0, v0, v2
						0x000e,                 // 6: return-void
						0x000d,                 // 7: move-exception v0 (ReflectiveOperationException!)
						0x1071, 0x0001, 0x0000, // 8..10: invoke-static {v0}, consumeThrowable(Throwable)V
						0x000e,                 // 11: return-void
					},
					Tries: []model.Try{
						{
							StartAddress:     4,
							InstructionCount: 2,
							Handler: model.CatchHandler{
								Handlers: []model.TypedHandler{
									{Type: "Ljava/lang/IllegalAccessException;", Address: 7},
									{Type: "Ljava/lang/NoSuchMethodException;", Address: 7},
								},
							},
						},
					},
				},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[5], // mergeIntegerAndFloat(Z)V -> Number
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 1,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x1112,                 // 3: const/4 v1, #1
						0x0238, 0x0007,         // 4..5: if-eqz v2, +7 (target 11)
						0x1071, 0x0006, 0x0001, // 6..8: invoke-static {v1}, Integer.valueOf(I)Integer
						0x000c,                 // 9: move-result-object v0
						0x0728,                 // 10: goto +7 (target 17)
						0x0115, 0x3f80,         // 11..12: const/high16 v1, 0x3f800000
						0x1071, 0x0007, 0x0001, // 13..15: invoke-static {v1}, Float.valueOf(F)Float
						0x000c,                 // 16: move-result-object v0
						0x1071, 0x0002, 0x0000, // 17..19: invoke-static {v0}, consumeNumber(Number)V
						0x000e,                 // 20: return-void
					},
				},
			},
		},
	}

	file.Classes = []model.Class{*faceClass, *ownerClass}
	faceRaw, err := Class(file, faceClass)
	if err != nil {
		t.Fatalf("Class(face): %v", err)
	}
	ownerRaw, err := Class(file, ownerClass)
	if err != nil {
		t.Fatalf("Class(owner): %v", err)
	}
	parsed, err := javaclassparser.Parse(ownerRaw)
	if err != nil {
		t.Fatalf("Parse(owner): %v", err)
	}
	if parsed.MajorVersion != 52 {
		t.Fatalf("owner MajorVersion = %d, want 52", parsed.MajorVersion)
	}

	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		return
	}
	dir := t.TempDir()
	for rel, raw := range map[string][]byte{
		filepath.Join("dex", "lca", "Face.class"):        faceRaw,
		filepath.Join("dex", "lca", "PlatformLca.class"): ownerRaw,
	} {
		out := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        dex.lca.PlatformLca.catchErrorAndRuntime(0);
        dex.lca.PlatformLca.catchReflectiveMultiCatch(1);
        dex.lca.PlatformLca.mergeIntegerAndFloat(false);
        dex.lca.PlatformLca.mergeIntegerAndFloat(true);
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
