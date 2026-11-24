package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestLowerSkipsUnreachableTrailingNopAndDeadMiddleBlocks(t *testing.T) {
	const faceDesc = "Ldex/unreach/Face;"
	const ownerDesc = "Ldex/unreach/TrailingNop;"

	catchAll := uint32(7)
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: faceDesc},
			{Descriptor: ownerDesc},
			{Descriptor: "I"},
			{Descriptor: "Z"},
			{Descriptor: "Ljava/lang/RuntimeException;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I"},                                                       // 0: ()I
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{2}}, // 1: (I)I
			{ReturnType: "I", Parameters: []string{"Z"}, ParameterTypes: []uint32{3}}, // 2: (Z)I
		},
		Methods: []model.MethodID{
			{Class: faceDesc, Name: "ping", ProtoIndex: 0},                   // 0
			{Class: ownerDesc, Name: "switchTrailingNop", ProtoIndex: 1},     // 1
			{Class: ownerDesc, Name: "trampolineTrailingNop", ProtoIndex: 1}, // 2
			{Class: ownerDesc, Name: "middleUnreachableNop", ProtoIndex: 2},  // 3
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
				Code: &model.Code{
					RegistersSize: 1,
					Instructions: []uint16{
						0x7012, // const/4 v0, #7
						0x000f, // return v0
					},
				},
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
				Method:      file.Methods[1], // switchTrailingNop(I)I -> packed-switch + trailing alignment nop
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x000a,                 // 3: move-result v0
						0x012b, 0x0008, 0x0000, // 4..6: packed-switch v1, +8 (payload at 12)
						0x000f,                 // 7: return v0 (default)
						0x0090, 0x0100,         // 8..9: add-int v0, v0, v1
						0x000f,                 // 10: return v0 (last reachable)
						0x0000,                 // 11: nop (unreachable alignment pad before payload)
						0x0100, 0x0001, 0x0000, 0x0000, 0x0004, 0x0000, // 12..17: switch payload (key 0 -> 4+4=8)
					},
				},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2], // trampolineTrailingNop(I)I -> POP;GOTO handler + trailing nop
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x000a,         // 3: move-result v0
						0x0093, 0x0100, // 4..5: div-int v0, v0, v1
						0x000f,         // 6: return v0
						0x000f,         // 7: handler (no move-exception -> needs POP trampoline!): return v0
						0x0000,         // 8: trailing unreachable nop right before handler trampoline
					},
					Tries: []model.Try{
						{
							StartAddress:     4,
							InstructionCount: 2,
							Handler: model.CatchHandler{
								CatchAllAddress: &catchAll,
							},
						},
					},
				},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[3], // middleUnreachableNop(Z)I -> return | unreachable nop | target reading v1
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0071, 0x0000, 0x0000, // 0..2: invoke-static {}, Face.ping()I
						0x000a,         // 3: move-result v0
						0x5112,         // 4: const/4 v1, #5
						0x0238, 0x0004, // 5..6: if-eqz v2, +4 (target 9)
						0x000f,         // 7: return v0
						0x0000,         // 8: middle unreachable nop
						0x010f,         // 9: return v1 (must NOT have entryFrame joined from unreachable block 8)
					},
				},
			},
		},
	}

	faceRaw, err := Class(file, faceClass)
	if err != nil {
		t.Fatalf("Class(face): %v", err)
	}
	ownerRaw, err := func() ([]byte, error) {
		file.Classes = []model.Class{*faceClass, *ownerClass}
		return Class(file, ownerClass)
	}()
	if err != nil {
		t.Fatalf("Class(owner): %v", err)
	}
	parsed, err := javaclassparser.Parse(ownerRaw)
	if err != nil {
		t.Fatalf("Parse(owner): %v", err)
	}
	if parsed.MajorVersion != 52 {
		t.Fatalf("owner MajorVersion = %d, want 52 (Java 8 StackMapTable active)", parsed.MajorVersion)
	}

	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		return
	}
	dir := t.TempDir()
	for rel, raw := range map[string][]byte{
		filepath.Join("dex", "unreach", "Face.class"):        faceRaw,
		filepath.Join("dex", "unreach", "TrailingNop.class"): ownerRaw,
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
        if (dex.unreach.TrailingNop.switchTrailingNop(0) != 7) throw new AssertionError("switch 0");
        if (dex.unreach.TrailingNop.switchTrailingNop(2) != 7) throw new AssertionError("switch 2");
        if (dex.unreach.TrailingNop.trampolineTrailingNop(1) != 7) throw new AssertionError("trampoline normal");
        if (dex.unreach.TrailingNop.trampolineTrailingNop(0) != 7) throw new AssertionError("trampoline catch");
        if (dex.unreach.TrailingNop.middleUnreachableNop(false) != 5) throw new AssertionError("middle false");
        if (dex.unreach.TrailingNop.middleUnreachableNop(true) != 7) throw new AssertionError("middle true");
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
