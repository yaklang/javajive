package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
)

func findWave2MethodCode(t *testing.T, raw []byte, name string) *javaclassparser.CodeAttribute {
	t.Helper()
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	for _, m := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)).Value != name {
			continue
		}
		for _, attr := range m.Attributes {
			if c, ok := attr.(*javaclassparser.CodeAttribute); ok {
				return c
			}
		}
	}
	t.Fatalf("method %q Code attribute not found", name)
	return nil
}

func runMainWithVerify(t *testing.T, raw []byte, className string) {
	t.Helper()
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("Parse(%s) failed: %v", className, err)
	}
	java, err := exec.LookPath("java")
	if err != nil {
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, strings.ReplaceAll(className, ".", "/")+".class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(java, "-Xverify:all", "-cp", dir, className).CombinedOutput(); err != nil {
		t.Fatalf("java -Xverify:all %s failed: %v\n%s", className, err, out)
	}
}

func TestWave2ExceptionTableCoalescingAndIncompatibleSplit(t *testing.T) {
	const owner = "Ldex/CoalesceTry;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Throwable;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "V"},
			{Descriptor: "I"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{4}},
			{ReturnType: "V"},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}},
			{ReturnType: "I", Parameters: []string{"Ljava/lang/Object;", "Ljava/lang/String;", "I"}, ParameterTypes: []uint32{6, 2, 4}},
			{ReturnType: "I"},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "coalesceSafe", ProtoIndex: 0},
			{Class: owner, Name: "splitIncompatible", ProtoIndex: 3},
			{Class: owner, Name: "boom", ProtoIndex: 1},
			{Class: owner, Name: "main", ProtoIndex: 2},
			{Class: "Ljava/lang/String;", Name: "length", ProtoIndex: 4},
			{Class: "Ljava/lang/String;", Name: "<init>", ProtoIndex: 1},
		},
	}
	file.Types = append(file.Types, model.Type{Descriptor: "Ljava/lang/Object;"})

	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 2,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 1,
					Instructions: []uint16{
						0x0012, // const/4 v0, 0 (null)
						0x0027, // throw v0
					},
				},
			},
			// coalesceSafe(I)I: two protected slices separated by a non-throwing `const/4 v0, 1`
			// where v1 (the handler-live local) stays VerifyInt before, during, and after the gap.
			{
				MethodIndex: 0,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[0],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x0112,                 // 0: const/4 v1, #0
						0x0071, 0x0002, 0x0000, // 1..3: invoke-static {}, boom()V
						0x1012,                 // 4: const/4 v0, #1 (non-throwing intermediate outside try)
						0x0071, 0x0002, 0x0000, // 5..7: invoke-static {}, boom()V
						0x000f, // 8: return v0
						0x000d, // 9: move-exception v0 (handler)
						0x010f, // 10: return v1
					},
					Tries: []model.Try{
						{
							StartAddress: 1, InstructionCount: 7,
							Handler: model.CatchHandler{Handlers: []model.TypedHandler{{TypeIndex: 1, Type: "Ljava/lang/Throwable;", Address: 9}}},
						},
					},
				},
			},
			// splitIncompatible(I)I: gap writes a String (reference) into v1 while handler uses v1 after
			// catch; coalescing across the intermediate range would make the handler frame invalid.
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 0,
					Instructions: []uint16{
						0x1112,                 // 0: const/4 v1, #1 (int)
						0x0071, 0x0002, 0x0000, // 1..3: invoke-static {}, boom()V (try 1)
						0x0112, // 4: const/4 v1, #0 (null -> overwritten with String below)
						0x1207, // 5: move-object v1, v2 (v2=incoming int? Wait: use const-string for incompatible Ref!)
						0x000f, // 6: return v0 (replaced below)
					},
				},
			},
			{
				MethodIndex: 3,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 4, InsSize: 1, OutsSize: 3,
					Instructions: []uint16{
						0x2012,                 // 0:     const/4 v0, #2
						0x1071, 0x0000, 0x0000, // 1..3:  invoke-static {v0}, coalesceSafe
						0x0012,         // 4:     const/4 v0, #0 (null Object)
						0x011a, 0x0000, // 5..6:  const-string v1, "x"
						0x1212,                 // 7:     const/4 v2, #1 (flag=1)
						0x3071, 0x0001, 0x0210, // 8..10: invoke-static {v0, v1, v2}, splitIncompatible
						0x000e, // 11:    return-void
					},
				},
			},
		},
	}

	file.Strings = []string{"x"}
	cls.DirectMethods[2].Code = &model.Code{
		RegistersSize: 5, InsSize: 3, OutsSize: 1, // v2=Object, v3=String, v4=flag
		Instructions: []uint16{
			0x0439, 0x0009, // 0..1:   if-nez v4, +9 -> 9
			0x0122, 0x0002, // 2..3:   new-instance v1, String (pins v1!)
			0x1070, 0x0005, 0x0001, // 4..6:   invoke-direct {v1}, String.<init>()V
			0x0012,                 // 7:      const/4 v0, #0
			0x000f,                 // 8:      return v0
			0x3107,                 // 9:      move-object v1, v3 (v1=String)
			0x0071, 0x0002, 0x0000, // 10..12: invoke-static {}, boom()V (Try start = 10)
			0x2107,                 // 13:     move-object v1, v2 (gap: v1 becomes java/lang/Object!)
			0x3107,                 // 14:     move-object v1, v3 (gap: v1 restored to String without throwing)
			0x0071, 0x0002, 0x0000, // 15..17: invoke-static {}, boom()V (Try end = 18)
			0x0012,                 // 18:     const/4 v0, #0
			0x000f,                 // 19:     return v0
			0x000d,                 // 20:     move-exception v0 (handler!)
			0x106e, 0x0004, 0x0001, // 21..23: invoke-virtual {v1}, String.length()I
			0x000a, // 24:     move-result v0
			0x000f, // 25:     return v0
		},
		Tries: []model.Try{
			{
				StartAddress: 10, InstructionCount: 8,
				Handler: model.CatchHandler{Handlers: []model.TypedHandler{{TypeIndex: 1, Type: "Ljava/lang/Throwable;", Address: 20}}},
			},
		},
	}

	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed: %v", err)
	}
	safeCode := findWave2MethodCode(t, raw, "coalesceSafe")
	if len(safeCode.ExceptionTable) != 1 {
		t.Fatalf("coalesceSafe ExceptionTable len = %d, want 1 coalesced entry", len(safeCode.ExceptionTable))
	}
	splitCode := findWave2MethodCode(t, raw, "splitIncompatible")
	if len(splitCode.ExceptionTable) != 2 {
		t.Fatalf("splitIncompatible ExceptionTable len = %d, want 2 split entries across incompatible Ref store", len(splitCode.ExceptionTable))
	}
	runMainWithVerify(t, raw, "dex.CoalesceTry")
}

func TestWave2MultiKindWebPrologueAndUntypedZeroVerify(t *testing.T) {
	// Checks that an incoming float parameter register overwritten with int AND untyped-zero `0`
	// across float/int/ref uses gets distinct companion slots (prologueStoreSlot + resolvedZeroKind)
	// and passes `java -Xverify:all`.
	const owner = "Ldex/PolyPrologue;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "F"},
			{Descriptor: "I"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"F", "I"}, ParameterTypes: []uint32{1, 2}},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "poly", ProtoIndex: 0},
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
				// v2=float p, v3=int flag; reuse p (v2) as int AND return float/int/null via shared zero reg v0
				Code: &model.Code{
					RegistersSize: 4, InsSize: 2, OutsSize: 0,
					Instructions: []uint16{
						0x0012,         // 0: const/4 v0, #0 (untyped zero used as both 0F and 0 int)
						0x0338, 0x0004, // 1..2: if-eqz v3, +4 -> 5
						0x0087, // 3: wait? No, float-to-int v2, v2 (0x87: float-to-int vA, vB => (B<<12)|(A<<8)|0x87)
						0x2287, // replace above word cleanly below
					},
				},
			},
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x0015, 0x3f80, // 0..1: const/high16 v0, 1.0f
						0x1112,                 // 2: const/4 v1, #1
						0x2071, 0x0000, 0x0010, // 3..5: invoke-static {v0, v1}, poly(FI)I
						0x000e, // 6: return-void
					},
				},
			},
		},
	}
	cls.DirectMethods[0].Code.Instructions = []uint16{
		0x0012,         // 0: const/4 v0, #0 (untyped 0!)
		0x0338, 0x0004, // 1..2: if-eqz v3, +4 -> 5
		0x2287, // 3: float-to-int v2, v2 (reads v2 as float, overwrites v2 with int!)
		0x0228, // 4: goto +2 -> 6
		0x02b0, // 5: add-int/2addr v2, v0 (reads v0 as INT zero AND overwrites v2) -> actually cmpg-float? let's use add-float/2addr v2, v0 (0xa6: add-float/2addr v2, v0 => 0x02a6)!
		0x000f, // 6: return v0 (returns v0 as INT!)
	}
	cls.DirectMethods[0].Code.Instructions[4] = 0x0228 // goto +2 -> 6
	cls.DirectMethods[0].Code.Instructions[5] = 0x02a6 // add-float/2addr v2, v0 (uses v0 as FLOAT zero + v2 as float!)
	// Wait: after branch, merge on v2: add 0x2001 move v0, v2 (uses v2? Wait in branch 1 v2 is int, branch 2 v2 is float? Don't merge different types directly in v2; instead v1 receives zero float vs int!)
	cls.DirectMethods[0].Code.Instructions = []uint16{
		0x0012,         // 0: const/4 v0, #0 (untyped zero used as BOTH float 0.0f and int 0!)
		0x0338, 0x0004, // 1..2: if-eqz v3, +4 -> 5
		0x2287,         // 3: float-to-int v2, v2 (v2 reused from Float param -> Int)
		0x0328,         // 4: goto +3 -> 7
		0x02c6,         // 5: add-float/2addr v2, v0 (reads v0 as Float)
		0x2287,         // 6: float-to-int v2, v2 (v2 now Int on both paths)
		0x0090, 0x0002, // 7..8: add-int v0, v2, v0 (reads v0 as Int!)
		0x000f, // 9: return v0
	}

	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed on poly prologue / untyped zero: %v", err)
	}
	runMainWithVerify(t, raw, "dex.PolyPrologue")
}

func TestWave2CFGConstructorShapesAliasNonLeakThrowOnlyAndBranching(t *testing.T) {
	const owner = "Ldex/BranchCtor;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/RuntimeException;"},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: "[Ljava/lang/String;"},
		},
		Fields: []model.FieldID{
			{Class: owner, Name: "flag", Type: "I"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{3}},
			{ReturnType: "V", Parameters: []string{"I", "I"}, ParameterTypes: []uint32{3, 3}},
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{5}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
			{Class: "Ljava/lang/RuntimeException;", Name: "<init>", ProtoIndex: 0},
			{Class: owner, Name: "<init>", ProtoIndex: 1}, // branching + deferred store + goto non-leak
			{Class: owner, Name: "<init>", ProtoIndex: 2}, // throw-only ctor
			{Class: owner, Name: "main", ProtoIndex: 3},
		},
	}

	cls := &model.Class{
		Descriptor:     owner,
		Superclass:     "Ljava/lang/Object;",
		AccessFlags:    dexPublic,
		InstanceFields: []model.EncodedField{{FieldIndex: 0, AccessFlags: dexPublic, Field: file.Fields[0]}},
		DirectMethods: []model.EncodedMethod{
			// Branching <init>(I) with:
			// - deferred store `iput v4, v2, flag` before `if-eqz`
			// - branch 1 aliasing v1=v2 (`this`) and jumping around branch 2 (`goto +3`)
			// - branch 2 moving `v0 = v1` (v1 is NOT uninitialized `this` in branch 2!)
			// - mutually exclusive `Object.<init>(v2)` on both branches!
			{
				MethodIndex: 2,
				AccessFlags: dexPublic | dexConstructor,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 4, InsSize: 2, OutsSize: 1, // v2=this, v3=int p
					Instructions: []uint16{
						0x0012,         // 0: const/4 v0, #0
						0x0112,         // 1: const/4 v1, #0
						0x2359, 0x0000, // 2..3: iput v3, v2, BranchCtor.flag (deferred store)
						0x0338, 0x0006, // 4..5: if-eqz v3, +6 -> 10 (branch 2)
						0x2107,                 // 6: move-object v1, v2 (Branch 1 ONLY: v1 aliases this!)
						0x1070, 0x0000, 0x0001, // 7..9: invoke-direct {v1}, Object.<init>()V
						0x000e, // 10-? Wait: branch 1 needs goto over branch 2 -> adjust offsets below!
					},
				},
			},
			// Throw-only <init>(I, I): 0 delegations, 0 reachable return-voids -> throws RuntimeException.
			{
				MethodIndex: 3,
				AccessFlags: dexPublic | dexConstructor,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 4, InsSize: 3, OutsSize: 1,
					Instructions: []uint16{
						0x0022, 0x0002, // 0..1: new-instance v0, RuntimeException
						0x1070, 0x0001, 0x0000, // 2..4: invoke-direct {v0}, RuntimeException.<init>()V
						0x0027, // 5: throw v0
					},
				},
			},
			{
				MethodIndex: 4,
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[4],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 1, OutsSize: 2,
					Instructions: []uint16{
						0x0022, 0x0000, // 0..1: new-instance v0, BranchCtor
						0x5112,                 // 2: const/4 v1, #5
						0x2070, 0x0002, 0x0010, // 3..5: wait regs D=1, C=0 -> 0x10
						0x000e, // 6: return-void
					},
				},
			},
		},
	}
	cls.DirectMethods[0].Code.Instructions = []uint16{
		0x0012,         // 0: const/4 v0, #0
		0x0112,         // 1: const/4 v1, #0
		0x2359, 0x0000, // 2..3: iput v3, v2, BranchCtor.flag
		0x0338, 0x0007, // 4..5: if-eqz v3, +7 -> 11 (branch 2)
		0x2107,                 // 6: move-object v1, v2 (branch 1 only!)
		0x1070, 0x0000, 0x0001, // 7..9: invoke-direct {v1}, Object.<init>()V
		0x0528,                 // 10: goto +5 -> 15 (exit)
		0x1001,                 // 11: move v0, v1 (branch 2: reads v1 which was NOT this in branch 2!)
		0x1070, 0x0000, 0x0002, // 12..14: invoke-direct {v2}, Object.<init>()V
		0x000e, // 15: return-void
	}
	cls.DirectMethods[2].Code.Instructions = []uint16{
		0x0022, 0x0000, // 0..1: new-instance v0, BranchCtor
		0x5112,                 // 2: const/4 v1, #5
		0x2070, 0x0002, 0x0010, // 3..5: invoke-direct {v0, v1}, BranchCtor.<init>(I)V
		0x000e, // 6: return-void
	}

	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed on CFG branching/alias/throw-only ctors: %v", err)
	}
	runMainWithVerify(t, raw, "dex.BranchCtor")
}

func TestWave2DiscardedFilledNewArray(t *testing.T) {
	const owner = "Ldex/DiscardFilledArray;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "[I"},
			{Descriptor: "[Ljava/lang/String;"},
			{Descriptor: "V"},
		},
		Protos: []model.Proto{
			{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
		},
		Methods: []model.MethodID{
			{Class: owner, Name: "main", ProtoIndex: 0},
		},
	}
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 0,
			AccessFlags: dexPublic | dexStatic,
			Method:      file.Methods[0],
			Code: &model.Code{
				RegistersSize: 2, InsSize: 1, OutsSize: 0,
				Instructions: []uint16{
					0x7012,                 // 0: const/4 v0, #7
					0x1024, 0x0001, 0x0000, // 1..3: filled-new-array {v0}, [I (result discarded!)
					0x000e, // 4: return-void
				},
			},
		}},
	}
	raw, err := Class(file, cls)
	if err != nil {
		t.Fatalf("Class failed on discarded filled-new-array: %v", err)
	}
	runMainWithVerify(t, raw, "dex.DiscardFilledArray")
}

func TestWave2RecoverConstructorParameterAnnotationsAndStrictRejection(t *testing.T) {
	const owner = "Ldex/ParamAnnot;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/Deprecated;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "I"},
			{Descriptor: "V"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{3, 4}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
			{Class: owner, Name: "<init>", ProtoIndex: 1},
		},
	}
	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			MethodIndex: 1,
			AccessFlags: dexPublic | dexConstructor,
			Method:      file.Methods[1],
			Code: &model.Code{
				RegistersSize: 3, InsSize: 3, OutsSize: 1,
				Instructions: []uint16{
					0x1070, 0x0000, 0x0000,
					0x000e,
				},
			},
		}},
		ParameterAnnotations: []model.ParameterAnnotation{{
			MethodIndex: 1,
			Parameters: [][]model.Annotation{
				// Only 1 entry for a 2-param constructor (synthetic outer-this entry stripped by R8):
				{{Visibility: model.AnnotationRuntime, Type: "Ljava/lang/Deprecated;"}},
			},
		}},
		ParameterAnnotationIndex: map[uint32]int{1: 0},
	}

	if _, err := ClassWithPolicy(file, cls, ClassOptions{}, Strict); err == nil {
		t.Fatal("Strict unexpectedly accepted mismatched constructor parameter annotations")
	}
	res, err := ClassWithPolicy(file, cls, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	parsed, err := javaclassparser.Parse(res.Bytes)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	var rvpa *javaclassparser.RuntimeVisibleParameterAnnotationsAttribute
	for _, m := range parsed.Methods {
		if parsed.ConstantPoolManager.GetUtf8(int(m.NameIndex)).Value != "<init>" {
			continue
		}
		for _, attr := range m.Attributes {
			if pa, ok := attr.(*javaclassparser.RuntimeVisibleParameterAnnotationsAttribute); ok && !pa.IsInvisible {
				rvpa = pa
			}
		}
	}
	if rvpa == nil {
		t.Fatal("expected right-aligned RuntimeVisibleParameterAnnotations attribute")
	}
	if len(rvpa.ParameterAnnotations) != 2 {
		t.Fatalf("num_parameters = %d, want 2 (prepended empty entry for leading param)", len(rvpa.ParameterAnnotations))
	}
	if len(rvpa.ParameterAnnotations[0]) != 0 || len(rvpa.ParameterAnnotations[1]) != 1 {
		t.Fatalf("aligned annotation counts = [%d, %d], want [0, 1]", len(rvpa.ParameterAnnotations[0]), len(rvpa.ParameterAnnotations[1]))
	}
}

func TestWave2EnumOrdinalOrderRestoresRealEnum(t *testing.T) {
	const owner = "Ldex/OrderColor;"
	const arrayOwner = "[Ldex/OrderColor;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"ZEBRA", "ALPHA"},
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Enum;"},
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "I"},
			{Descriptor: "V"},
			{Descriptor: arrayOwner},
		},
		Fields: []model.FieldID{
			// Alphabetically sorted in DEX: ALPHA (field 0) before ZEBRA (field 1), plus $VALUES (field 2),
			// while <clinit> constructs ZEBRA at ordinal 0 and ALPHA at ordinal 1!
			{Class: owner, Name: "ALPHA", Type: owner},
			{Class: owner, Name: "ZEBRA", Type: owner},
			{Class: owner, Name: "$VALUES", Type: arrayOwner},
		},
		Protos: []model.Proto{
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;", "I"}, ParameterTypes: []uint32{2, 3}},
			{ReturnType: "V"},
			{ReturnType: arrayOwner},
			{ReturnType: owner, Parameters: []string{"Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Enum;", Name: "<init>", ProtoIndex: 0},
			{Class: owner, Name: "<init>", ProtoIndex: 0},
			{Class: owner, Name: "<clinit>", ProtoIndex: 1},
			{Class: owner, Name: "values", ProtoIndex: 2},
			{Class: arrayOwner, Name: "clone", ProtoIndex: 1}, // placeholder replaced below
			{Class: owner, Name: "valueOf", ProtoIndex: 3},
			{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 3},
		},
	}
	// Replace file.Types/Protos/Methods cleanly for clone():()Ljava/lang/Object; and Enum.valueOf(Class, String)
	file.Types = append(file.Types, model.Type{Descriptor: "Ljava/lang/Object;"}, model.Type{Descriptor: "Ljava/lang/Class;"})
	file.Protos = append(file.Protos,
		model.Proto{ReturnType: "Ljava/lang/Object;"},
		model.Proto{ReturnType: "Ljava/lang/Enum;", Parameters: []string{"Ljava/lang/Class;", "Ljava/lang/String;"}, ParameterTypes: []uint32{7, 2}},
	)
	file.Methods[4] = model.MethodID{Class: arrayOwner, Name: "clone", ProtoIndex: 4}
	file.Methods[6] = model.MethodID{Class: "Ljava/lang/Enum;", Name: "valueOf", ProtoIndex: 5}

	cls := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Enum;",
		AccessFlags: dexPublic | dexFinal | dexEnum,
		StaticFields: []model.EncodedField{
			{FieldIndex: 0, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[0]}, // ALPHA first in DEX
			{FieldIndex: 1, AccessFlags: dexPublic | dexStatic | dexFinal | dexEnum, Field: file.Fields[1]}, // ZEBRA second in DEX
			{FieldIndex: 2, AccessFlags: dexPrivate | dexStatic | dexFinal | dexSynthetic, Field: file.Fields[2]},
		},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1, // <init>(String, I)
				AccessFlags: dexPrivate | dexConstructor,
				Method:      file.Methods[1],
				Code: &model.Code{
					RegistersSize: 3, InsSize: 3, OutsSize: 3, // v0=this, v1=name, v2=ord
					Instructions: []uint16{
						0x3070, 0x0000, 0x0100, // invoke-direct {v0, v1, v2}, Enum.<init>(String,I)V
						0x000e, // return-void
					},
				},
			},
			{
				MethodIndex: 2, // <clinit>()
				AccessFlags: dexStatic | dexConstructor,
				Method:      file.Methods[2],
				Code: &model.Code{
					RegistersSize: 4, InsSize: 0, OutsSize: 3,
					Instructions: []uint16{
						// ZEBRA at ordinal 0
						0x0022, 0x0000, // 0..1: new-instance v0, OrderColor
						0x011a, 0x0000, // 2..3: const-string v1, "ZEBRA"
						0x0212,                 // 4:    const/4 v2, #0
						0x3070, 0x0001, 0x1000, // 5..7: invoke-direct {v0, v1, v2}, OrderColor.<init>(String,I)V -> G=2
						0x0069, 0x0001, // 8..9: sput-object v0, ZEBRA (field 1)
						// ALPHA at ordinal 1
						0x0022, 0x0000, // 10..11: new-instance v0, OrderColor
						0x011a, 0x0001, // 12..13: const-string v1, "ALPHA"
						0x1212,                 // 14:     const/4 v2, #1
						0x3070, 0x0001, 0x1000, // 15..17: invoke-direct {v0, v1, v2}, OrderColor.<init>(String,I)V
						0x0069, 0x0000, // 18..19: sput-object v0, ALPHA (field 0)
						// $VALUES = new OrderColor[]{ZEBRA, ALPHA}
						0x2312,         // 20:     const/4 v3, #2
						0x3023, 0x0005, // 21..22: new-array v0, v3, [Ldex/OrderColor;
						0x0162, 0x0001, // 23..24: sget-object v1, ZEBRA
						0x0212,         // 25:     const/4 v2, #0
						0x014d, 0x0200, // 26..27: aput-object v1, v0[v2]
						0x0162, 0x0000, // 28..29: sget-object v1, ALPHA
						0x1212,         // 30:     const/4 v2, #1
						0x014d, 0x0200, // 31..32: aput-object v1, v0[v2]
						0x0069, 0x0002, // 33..34: sput-object v0, $VALUES
						0x000e, // 35: return-void
					},
				},
			},
			{
				MethodIndex: 3, // values()
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[3],
				Code: &model.Code{
					RegistersSize: 1, InsSize: 0, OutsSize: 1,
					Instructions: []uint16{
						0x0062, 0x0002, // sget-object v0, $VALUES
						0x106e, 0x0004, 0x0000, // invoke-virtual {v0}, clone()
						0x000c,         // move-result-object v0
						0x001f, 0x0005, // check-cast v0, [Ldex/OrderColor;
						0x0011, // return-object v0
					},
				},
			},
			{
				MethodIndex: 5, // valueOf(String)
				AccessFlags: dexPublic | dexStatic,
				Method:      file.Methods[5],
				Code: &model.Code{
					RegistersSize: 2, InsSize: 1, OutsSize: 2, // v1=String
					Instructions: []uint16{
						0x001c, 0x0000, // const-class v0, OrderColor
						0x2071, 0x0006, 0x0010, // invoke-static {v0, v1}, Enum.valueOf(Class, String)
						0x000c,         // move-result-object v0
						0x001f, 0x0000, // check-cast v0, OrderColor
						0x0011, // return-object v0
					},
				},
			},
		},
	}
	// Fix 35c encoding on words with count=3: byte1 = (G<<4)|count=2?!
	// DEX 35c format: word0 = (count<<12)|(G<<8)|op, word1 = methodIdx, word2 = (F<<12)|(E<<8)|(D<<4)|C
	// For count=3 {v0, v1, v2}: count=3, G unused (0), C=0, D=1, E=2 -> word0=(3<<12)|op, word2=(2<<8)|(1<<4)|0 = 0x0210!
	cls.DirectMethods[0].Code.Instructions[0] = 0x3070
	cls.DirectMethods[0].Code.Instructions[2] = 0x0210
	cls.DirectMethods[1].Code.Instructions[5] = 0x3070
	cls.DirectMethods[1].Code.Instructions[7] = 0x0210
	cls.DirectMethods[1].Code.Instructions[15] = 0x3070
	cls.DirectMethods[1].Code.Instructions[17] = 0x0210

	res, err := ClassWithPolicy(file, cls, ClassOptions{}, Recover)
	if err != nil {
		t.Fatalf("ClassWithPolicy failed on enum: %v", err)
	}
	parsed, err := javaclassparser.Parse(res.Bytes)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(parsed.Fields) < 2 {
		t.Fatalf("expected >= 2 fields, got %d", len(parsed.Fields))
	}
	f0 := parsed.ConstantPoolManager.GetUtf8(int(parsed.Fields[0].NameIndex)).Value
	f1 := parsed.ConstantPoolManager.GetUtf8(int(parsed.Fields[1].NameIndex)).Value
	if f0 != "ZEBRA" || f1 != "ALPHA" {
		t.Fatalf("reordered static fields = [%q, %q], want [ZEBRA, ALPHA]", f0, f1)
	}
	src, err := javaclassparser.Decompile(res.Bytes)
	if err != nil {
		t.Fatalf("strict Decompile failed: %v", err)
	}
	if !strings.Contains(src, "public enum OrderColor") {
		t.Fatalf("expected `public enum OrderColor` in strict decompiled source, got:\n%s", src)
	}
	if strings.Contains(src, "extends Enum") {
		t.Fatalf("expected real enum declaration without `extends Enum`:\n%s", src)
	}
	if strings.Contains(src, "ParseBytesCode failed") {
		t.Fatalf("unexpected stub in strict enum decompilation:\n%s", src)
	}
	if strings.Contains(src, "static {") {
		t.Fatalf("expected enum constants to lift cleanly with zero `static {` block:\n%s", src)
	}
	zebraIdx := strings.Index(src, "ZEBRA")
	alphaIdx := strings.Index(src, "ALPHA")
	if zebraIdx < 0 || alphaIdx < 0 || zebraIdx > alphaIdx {
		t.Fatalf("expected ZEBRA declared before ALPHA in decompiled source:\n%s", src)
	}
	_ = bytes.TrimSpace(res.Bytes)
}
