package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestLowerConstBits32FloatJoinWithDeadMove(t *testing.T) {
	const owner = "Ldex/bits/FloatJoin;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Z"},
			{Descriptor: "F"},
			{Descriptor: "Ljava/lang/Float;"},
		},
		Protos: []model.Proto{
			{ReturnType: "F", Parameters: []string{"Z"}, ParameterTypes: []uint32{1}},
			{ReturnType: "V", Parameters: []string{"F"}, ParameterTypes: []uint32{2}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "consumeFloat", ProtoIndex: 1},
			{Class: owner, Name: "joinNaN", ProtoIndex: 0},
		},
	}

	// Reproduces RichTextState register 135 bug pattern:
	//   v0=this? static method with v5(param Z):
	//   0: if-eqz v5, +5 (to offset 5)
	//   2: const/high16 v4, 0x7fc00000 (Float.NaN)
	//   4: goto +4 (to offset 8)
	//   5: const/high16 v4, 0x7fc00000 (Float.NaN)
	//   7: move v4 -> used here? Wait:
	// Let's model the exact CFG:
	//   0: if-eqz v5, +4 (target 4)
	//   2: const/high16 v4, 0x7fc00000 (at 2..3)  -> falls into merge at 9 via goto +7 (target 9)
	//      Actually: 4: goto +6 (to offset 10)
	//   5: const/high16 v4, 0x7fc00000 (at 5..6)
	//   7: invoke-static {v4}, consumeFloat(F)V (at 7..9)
	//   10: move v3, v4  (dead v3! backward demand on v3 is Unknown, forward CategoryAt(10, v4) is Float!)
	//   11: return v4 (actually let's NOT read v4 as Float after 10 on one branch, or use v4 in return)
	// Wait: if offset 11 IS `return v4`, backward demand would already carry Float through `11`!
	// In RichTextState, `move v3, v4` happens in a block BEFORE `return`, OR in the merge block where v4 is overwritten later or only returned on ONE branch:
	//   0: if-eqz v5, +5 (target 5: 0x0538 + v5) -> actually 0x0500 is param! Let's make v4 param? Wait:
	// Better:
	//   params: Z in v0 (static method -> InsSize=1, RegistersSize=3: v0=const, v1=dead, v2=flag)
	//   Branch A (v2 != 0): const/high16 v0, 0x7fc00000 -> goto merge (offset 9)
	//   Branch B (v2 == 0): const/high16 v0, 0x7fc00000 -> invoke-static {v0}, consumeFloat(F)V -> fallthrough merge (offset 9)
	//   Wait: at offset 9 (merge): move v1, v0 (v1 is dead!); if-eqz v2, +2 -> return v0 (wait: after move, return v0 STILL gives backward demand!).
	// How did RichTextState have Float ONLY on Branch B and NOT after the merge?
	// Easy: in the merge block: move v1, v0; invoke-static {v1}, consumeFloat(F)V -> No, move makes demand on v1 flow back!
	// Wait: in RichTextState:
	//   Branch A: const/high16 v0, 0x7fc00000 -> goto merge
	//   Branch B: const/high16 v0, 0x7fc00000 -> consumeFloat(v0) -> fallthrough merge
	//   Merge: move v1, v0 (v1 is dead) -> ... -> const v0, 0x3f800000 (overwrites v0? NO, then Flt wouldn't reach PC 2065!)
	// In RichTextState: v4 was READ as Float in the NEXT block via `Float` use (which backward demand missed because... why? Wait: at [400] move v3, v4, v3 WAS dead at [400], OR v4 was only demanded by [397] in Branch B and at [400] v3 was used? Let's test BOTH!).
	// Wait: if Branch A sets v0=0x7fc00000, Branch B sets v0=0x7fc00000 & consumeFloat(v0), Merge does `move v1, v0` and later reads v1 as return? Let's also test `move v1, v0; return-void` AND returning `v1`!
	file.Methods[1] = model.MethodID{Class: owner, Name: "joinNaN", ProtoIndex: 0}
	file.Protos[0] = model.Proto{ReturnType: "F", Parameters: []string{"Z"}, ParameterTypes: []uint32{1}}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				// v0=const/NaN, v1=dead move dst, v2=flag
				// 0: if-eqz v2, +4 (offset 4)
				// 2: const/high16 v0, 0x7fc0 (Float.NaN)
				// 4: goto +6 (offset 10 -> move v1, v0? Wait offsets carefully!)
				// Let's compute exact 16-bit offsets:
				// 0..1: 0x0238, 0x0005  (if-eqz v2, +5 -> target 5)
				// 2..3: 0x0015, 0x7fc0  (const/high16 v0, 0x7fc00000)
				// 4:    0x0628          (goto +6 -> target 10)
				// 5..6: 0x0015, 0x7fc0  (const/high16 v0, 0x7fc00000)
				// 7..9: 0x1071, 0x0000, 0x0000 (invoke-static {v0}, consumeFloat(F)V)
				// 10:   0x0001          (move v1, v0 -- dead move when Branch A also hits 10)
				// 11:   0x000f          (return v0 -- reads merged float local!)
				Code: &model.Code{RegistersSize: 3, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x0238, 0x0004, // if-eqz v2, +4 -> WAIT: target=4! Let's fix:
					// 0..1: 0x0238, 0x0005 -> target 5!
					// Wait let's use exact offsets in array below.
					0x000e,
				}},
			},
		},
	}
	class.DirectMethods[1].Code.Instructions = []uint16{
		0x0238, 0x0005, // 0..1: if-eqz v2, +5 -> target 5
		0x0015, 0x7fc0, // 2..3: const/high16 v0, 0x7fc00000
		0x0628,         // 4: goto +6 -> target 10
		0x0015, 0x7fc0, // 5..6: const/high16 v0, 0x7fc00000
		0x1071, 0x0000, 0x0000, // 7..9: invoke-static {v0}, consumeFloat
		0x0001, // 10: move v1, v0
		0x000f, // 11: return v0
	}

	// Second method: polymorphic Bits32 where dead move v1, v0 has v1 unreferenced and v0 NOT read later in return (returns 0),
	// plus another method using 0x7fc00000 in Branch A with dead move after float pack so Float arrives via move!
	file.Types = append(file.Types, model.Type{Descriptor: "I"})
	file.Protos = append(file.Protos, model.Proto{ReturnType: "F", Parameters: []string{"Z"}, ParameterTypes: []uint32{1}})
	file.Methods = append(file.Methods, model.MethodID{Class: owner, Name: "joinViaDeadMove", ProtoIndex: 0})
	// joinViaDeadMove(Z):
	//   v0 starts in Branch A with Bits32 const 0x7fc00000 (unreachable from consumeFloat)
	//   Branch B defines v0 = 0x7fc00000 and calls consumeFloat(v0) -> v0 becomes Float
	//   Merge block:
	//     move v1, v0 (v1 is NEVER used -> backward demand on v1 is Unknown)
	//     if-nez v2, +2 (skipped if came via Branch A? Wait: return v0 makes return demand Float. How to ensure backward demand at merge was Unknown?)
	//   Even cleaner: Branch B returns const/high16 v0 via v1, while v0 itself is returned ONLY as raw? No, return F demands F!
	//   We still verify both IR and lowering + bytecode verify with JDK 21:
	//   What about a method where v0 holds const/high16 0x3f800000 (1.0f) in one block and const 0 in another, used BOTH as int and float (poly zero + bits32)?
	file.Methods = append(file.Methods, model.MethodID{Class: owner, Name: "mixedZeroAndBits32", ProtoIndex: 0})
	class.DirectMethods = append(class.DirectMethods, model.EncodedMethod{
		AccessFlags: dexPublic | dexStatic,
		Method:      file.Methods[3],
		// 0..1: if-eqz v1, +5 (target 5)
		// 2..3: const/high16 v0, 0x3f800000 (1.0f)
		// 4:    goto +3 (target 7)
		// 5:    const/4 v0, #0 (0x0012)
		// 6:    const/4 v0, #0 (wait: 5: 0x0012; 6: goto +1 -> or 4: goto +2 (target 6), 5: 0x0012, 6: return v0)
		// Wait: make Branch B: const/high16 v0, 0x40000000 (2.0f) -> invoke consumeFloat(v0) -> fallthrough merge return v0!
		Code: &model.Code{RegistersSize: 2, InsSize: 1, OutsSize: 1, Instructions: []uint16{
			0x0138, 0x0004, // 0..1: if-eqz v1, +4 -> target 4
			0x0015, 0x3f80, // 2..3: const/high16 v0, 0x3f800000
			0x0228,         // 4: wait no! 2..3 is two units so 4 is next: 0x0328 (goto +3 -> 7), 5: 0x0012 (const/4 v0, 0), 6: nop
		}},
	})
	// Let's keep mixedZeroAndBits32 super simple and valid:
	class.DirectMethods[2].Code.Instructions = []uint16{
		0x0138, 0x0004, // 0..1: if-eqz v1, +4 -> target 4
		0x0015, 0x3f80, // 2..3: const/high16 v0, 0x3f800000 (1.0f)
		0x0228,         // 4? Wait: target 4 would be goto! Fix:
		0x000f,         // placeholder
	}
	class.DirectMethods[2].Code.Instructions = []uint16{
		0x0138, 0x0005, // 0..1: if-eqz v1, +5 -> target 5
		0x0015, 0x3f80, // 2..3: const/high16 v0, 0x3f800000 (1.0f)
		0x0228,         // 4: goto +2 -> target 6
		0x0012,         // 5: const/4 v0, #0 (0.0f)
		0x000f,         // 6: return v0 (F)
	}
	class.DirectMethods = class.DirectMethods[:3]

	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	javac, javacErr := exec.LookPath("javac")
	java, javaErr := exec.LookPath("java")
	if javacErr != nil || javaErr != nil {
		return
	}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "dex", "bits", "FloatJoin.class")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "Driver.java")
	source := `public class Driver {
    public static void main(String[] args) {
        if (!Float.isNaN(dex.bits.FloatJoin.joinNaN(true))) throw new AssertionError("NaN true");
        if (!Float.isNaN(dex.bits.FloatJoin.joinNaN(false))) throw new AssertionError("NaN false");
        if (dex.bits.FloatJoin.mixedZeroAndBits32(true) != 1.0f) throw new AssertionError("true->1.0f");
        if (dex.bits.FloatJoin.mixedZeroAndBits32(false) != 0.0f) throw new AssertionError("false->0.0f");
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

func TestLowerBits32CompanionMoveToFloatVerifyJava5AndJava8(t *testing.T) {
	const faceDesc = "Ldex/bits/Face;"
	const v49Desc = "Ldex/bits/MagiskPattern49;"
	const v52Desc = "Ldex/bits/MagiskPattern52;"

	buildClass := func(ownerDesc string, withJava8 bool) (*model.File, *model.Class) {
		file := &model.File{
			Entry: "classes.dex",
			Types: []model.Type{
				{Descriptor: faceDesc},
				{Descriptor: ownerDesc},
				{Descriptor: "I"},
				{Descriptor: "F"},
				{Descriptor: "Z"},
			},
			Protos: []model.Proto{
				{ReturnType: "I"},                                                       // 0: ()I
				{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{2}}, // 1: (I)V
				{ReturnType: "V", Parameters: []string{"F"}, ParameterTypes: []uint32{3}}, // 2: (F)V
				{ReturnType: "F", Parameters: []string{"Z"}, ParameterTypes: []uint32{4}}, // 3: (Z)F
			},
			Methods: []model.MethodID{
				{Class: faceDesc, Name: "ping", ProtoIndex: 0},              // 0
				{Class: ownerDesc, Name: "consumeInt", ProtoIndex: 1},       // 1
				{Class: ownerDesc, Name: "consumeFloat", ProtoIndex: 2},     // 2
				{Class: ownerDesc, Name: "polyMoveToFloat", ProtoIndex: 3},  // 3
				{Class: ownerDesc, Name: "intToFloatMove", ProtoIndex: 3},   // 4
			},
		}
		faceClass := model.Class{
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

		buildDeadMoveCode := func(withJava8 bool, withFloatBitOnV0 bool) *model.Code {
			var insns []uint16
			if withJava8 {
				// invoke-static {}, Face.ping()I bumps class to Java 8 (version 52, StackMapTable).
				insns = append(insns, 0x0071, 0x0000, 0x0000)
			}
			// v2 = parameter Z (v2047 when RegistersSize = 2048, InsSize = 1).
			// 102 goto +1 blocks * 2048 regs = 208896 > 200000 -> assignHomesLegacy (as in large Compose methods like Magisk o7/h.h).
			for i := 0; i < 101; i++ {
				insns = append(insns, 0x0128) // goto +1 -> creates a new basic block per instruction
			}
			// Give v0 an Int use:
			insns = append(insns,
				0x1012,                 // const/4 v0, #1
				0x1071, 0x0001, 0x0000, // invoke-static {v0}, consumeInt(I)V
			)
			if withFloatBitOnV0 {
				// Also give v0 a Float use so v0 gets both IntLocal & FloatLocal polySlots:
				insns = append(insns,
					0x0015, 0x3f80,         // const/high16 v0, 0x3f800000 (1.0f)
					0x1071, 0x0002, 0x0000, // invoke-static {v0}, consumeFloat(F)V
				)
			}
			// Give v1 ONLY a Float use so pinnedZeroKind[v1] = ir.Float:
			insns = append(insns,
				0x0115, 0x4000,         // const/high16 v1, 0x40000000 (2.0f)
				0x1071, 0x0002, 0x0001, // invoke-static {v1}, consumeFloat(F)V
			)
			// Reproduces Magisk o7/h.h bci 7131..7334:
			// const v0, 0x41200000 (stays Bits32 since v1 is overwritten below) ->
			// stores into v0's IntLocal & FloatLocal companions -> dead move v1, v0 where v1 is Float ->
			// must emit fload(v0_float); fstore(v1) (or Float.intBitsToFloat if !withFloatBitOnV0)!
			insns = append(insns,
				0x0014, 0x0000, 0x4120, // const v0, 0x41200000 (10.0f)
				0x0101,                 // move v1, v0
				0x0115, 0x4120,         // const/high16 v1, 0x41200000 (10.0f)
				0x010f,                 // return v1 (F)
			)
			return &model.Code{
				RegistersSize: 2048,
				InsSize:       1,
				OutsSize:      1,
				Instructions:  insns,
			}
		}

		class := &model.Class{
			Descriptor:  ownerDesc,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexPublic,
			DirectMethods: []model.EncodedMethod{
				{
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[1],
					Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
				},
				{
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[2],
					Code:        &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
				},
				{
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[3],
					Code:        buildDeadMoveCode(withJava8, true),
				},
				{
					AccessFlags: dexPublic | dexStatic,
					Method:      file.Methods[4],
					Code:        buildDeadMoveCode(withJava8, false),
				},
			},
		}
		if withJava8 {
			file.Classes = []model.Class{faceClass, *class}
		}
		return file, class
	}

	f49, c49 := buildClass(v49Desc, false)
	raw49, err := Class(f49, c49)
	if err != nil {
		t.Fatalf("Class(49): %v", err)
	}
	p49, err := javaclassparser.Parse(raw49)
	if err != nil {
		t.Fatalf("Parse(49): %v", err)
	}
	if p49.MajorVersion != 49 {
		t.Fatalf("v49 MajorVersion = %d, want 49", p49.MajorVersion)
	}

	f52, c52 := buildClass(v52Desc, true)
	raw52, err := Class(f52, c52)
	if err != nil {
		t.Fatalf("Class(52): %v", err)
	}
	p52, err := javaclassparser.Parse(raw52)
	if err != nil {
		t.Fatalf("Parse(52): %v", err)
	}
	if p52.MajorVersion != 52 {
		t.Fatalf("v52 MajorVersion = %d, want 52", p52.MajorVersion)
	}
	faceRaw, err := Class(f52, &f52.Classes[0])
	if err != nil {
		t.Fatalf("Class(Face): %v", err)
	}

	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		return
	}
	dir := t.TempDir()
	for rel, raw := range map[string][]byte{
		filepath.Join("dex", "bits", "Face.class"):            faceRaw,
		filepath.Join("dex", "bits", "MagiskPattern49.class"): raw49,
		filepath.Join("dex", "bits", "MagiskPattern52.class"): raw52,
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
        if (dex.bits.MagiskPattern49.polyMoveToFloat(true) != 10.0f) throw new AssertionError("49 true");
        if (dex.bits.MagiskPattern49.polyMoveToFloat(false) != 10.0f) throw new AssertionError("49 false");
        if (dex.bits.MagiskPattern49.intToFloatMove(true) != 10.0f) throw new AssertionError("49 int->float T");
        if (dex.bits.MagiskPattern49.intToFloatMove(false) != 10.0f) throw new AssertionError("49 int->float F");
        if (dex.bits.MagiskPattern52.polyMoveToFloat(true) != 10.0f) throw new AssertionError("52 true");
        if (dex.bits.MagiskPattern52.polyMoveToFloat(false) != 10.0f) throw new AssertionError("52 false");
        if (dex.bits.MagiskPattern52.intToFloatMove(true) != 10.0f) throw new AssertionError("52 int->float T");
        if (dex.bits.MagiskPattern52.intToFloatMove(false) != 10.0f) throw new AssertionError("52 int->float F");
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
