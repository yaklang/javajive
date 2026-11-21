package dex2jar_test

import (
	"strings"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/dex2jar/lower"
	"github.com/yaklang/javajive/dex2jar/model"
)

func TestAdversarialIfEqzUntypedZeroRefinement(t *testing.T) {
	const owner = "Ladv/ZeroEqz;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"ok"},
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "I", Parameters: []string{"I"}},
			{ReturnType: "Ljava/lang/Object;", Parameters: []string{"I"}},
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"Ljava/lang/Object;"}},
			{ReturnType: "Ljava/lang/Object;", Parameters: []string{"I", "I"}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/String;", Name: "valueOf", ProtoIndex: 2},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			DirectMethods: []model.EncodedMethod{
				{
					AccessFlags: dexAccessPublic | dexAccessStatic,
					Method:      model.MethodID{Class: owner, Name: "intZeroEqz", ProtoIndex: 0},
					Code: &model.Code{
						RegistersSize: 2, // v0 local, v1 param
						InsSize:       1,
						Instructions: []uint16{
							0x0012,    // 0: const/4 v0, 0
							0x0138, 2, // 1: if-eqz v1, +2 -> 3
							0x1012, // 3 (Wait: fallthrough is 3! Jump target should be 4!): fix below
							0x0038, 3,
							0x7012, 0x000f,
							0x9012, 0x000f,
						},
					},
				},
			},
		}},
	}
	// Precise offsets:
	// intZeroEqz(I)I:
	// 0: const/4 v0, 0      (0x0012, 1u)
	// 1: if-eqz v1, +3      (0x0138, 3 -> 4)
	// 3: const/4 v0, 1      (0x1012, 1u)
	// 4: if-eqz v0, +3      (0x0038, 3 -> 7)
	// 6: const/4 v0, 5      (0x5012, 1u)
	// 7: return v0          (0x000f, 1u)
	file.Classes[0].DirectMethods[0].Code.Instructions = []uint16{
		0x0012,    // 0: v0 = 0
		0x0138, 3, // 1: if-eqz v1 -> 4
		0x1012,    // 3: v0 = 1
		0x0038, 3, // 4: if-eqz v0 -> 7
		0x5012, // 6: v0 = 5
		0x000f, // 7: return v0
	}

	// loopZeroEqz(I)I:
	// v0 = flag (0 or 1), v1 = i (0..v2), v2 = param n
	// 0: const/4 v0, 0      (1u)
	// 1: const/4 v1, 0      (1u)
	// 2: if-ge v1, v2, +7   (2u -> 9)
	// 4: if-eqz v1, +2      (2u -> 6)
	// 6: const/4 v0, 1      (1u)  Wait: if v1 != 0 set v0=1; actually 4: if-eqz v1 -> +3 (7); 6: const/4 v0, 1; 7: add-int/lit8 v1, v1, 1 (2u); 9: goto -7 (0xf928) -> exit at 10!
	// Recompute:
	// 0: const/4 v0, 0
	// 1: const/4 v1, 0
	// 2: if-ge v1, v2, +8   (target 10)
	// 4: if-eqz v1, +3      (target 7)
	// 6: const/4 v0, 1
	// 7: add-int/lit8 v1, v1, 1 (0x01d8, 0x0101 -> 2u)
	// 9: goto -7            (0xf928 -> 2)
	// 10: if-eqz v0, +3     (target 13)
	// 12: const/4 v0, 7
	// 13: return v0
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: owner, Name: "loopZeroEqz", ProtoIndex: 0},
		Code: &model.Code{
			RegistersSize: 3, // v0, v1, v2=n
			InsSize:       1,
			Instructions: []uint16{
				0x0012,    // 0: v0 = 0
				0x0112,    // 1: v1 = 0
				0x2135, 8, // 2: if v1 >= v2 goto 10
				0x0138, 3, // 4: if-eqz v1 goto 7
				0x1012,         // 6: v0 = 1
				0x01d8, 0x0101, // 7: v1++
				0xf928,    // 9: goto 2
				0x0038, 3, // 10: if-eqz v0 goto 13
				0x7012, // 12: v0 = 7
				0x000f, // 13: return v0
			},
		},
	})

	// refNullEqz(I)Ljava/lang/Object;:
	// 0: const/4 v0, 0
	// 1: if-eqz v1, +3 -> 4
	// 3: const-string v0, "ok" (2u -> off 5! So 1 should target +4 -> 5!)
	// 5: if-eqz v0, +3 -> 8
	// 7: return-object v0
	// 8: return-object v0
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: owner, Name: "refNullEqz", ProtoIndex: 1},
		Code: &model.Code{
			RegistersSize: 2,
			InsSize:       1,
			Instructions: []uint16{
				0x0012,    // 0: v0 = null
				0x0138, 4, // 1: if-eqz v1 goto 5
				0x001a, 0, // 3: const-string v0, "ok"
				0x0038, 3, // 5: if-eqz v0 goto 8
				0x0011, // 7: return-object v0
				0x0011, // 8: return-object v0
			},
		},
	})

	// polyZeroEqzAndCall(II)Ljava/lang/Object;:
	// 0: const/4 v0, 0
	// 1: if-eqz v1, +5 -> 6
	// 3: invoke-static {v0}, String.valueOf (3u)
	// 6: move-result-object v0
	// 7: return-object v0
	// Wait: put polyZero path where v1 != 0 merges v0 (0) with const/4 1 -> if-eqz v0!
	// 0: const/4 v0, 0
	// 1: if-eqz v1, +7 -> 8
	// 3: invoke-static {v0}, String.valueOf(Object) (0x0071? 1 arg in reg v0: 0x1071, 0x0000, 0x0000 -> 3u)
	// 6: move-result-object v0 (1u)
	// 7: return-object v0 (1u)
	// 8: if-eqz v2, +2 -> 10
	// 10... wait 8 is 2u (8,9), so const/4 v0, 1 is at 10, target 11!
	// 8: if-eqz v2, +3 -> 11
	// 10: const/4 v0, 1
	// 11: if-eqz v0, +3 -> 14
	// 13: const-string v0, "ok" (2u -> next 15, Wait: branch else!)
	// Actually:
	// 11: if-eqz v0, +3 -> 14
	// 13: const/4 v0, 0  (Wait: must return object)
	// 11: const-string? No, v0 before 11 is int! Re-write v3 as String return:
	// 11: if-eqz v0, +4 -> 15
	// 13: const-string v2, "ok"
	// 15: const/4 v2, 0 (No, v2 is param -> put into v1)
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: owner, Name: "polyZeroEqzAndCall", ProtoIndex: 3},
		Code: &model.Code{
			RegistersSize: 3, // v0 local, v1 p0, v2 p1
			InsSize:       2,
			OutsSize:      1,
			Instructions: []uint16{
				0x0012,    // 0: v0 = 0
				0x0138, 6, // 1: if-eqz v1 -> 7
				0x1071, 0x0000, 0, // 3: invoke-static {v0}, String.valueOf
				0x000c,    // 6: move-result-object v0
				0x0011,    // 7: return-object v0
				0x0238, 3, // 8: if-eqz v2 -> 11? Target 8+3=11! Wait: v0 is 0 from block 0!
				0x1012,    // 10: v0 = 1
				0x0038, 4, // 11: if-eqz v0 -> 15
				0x011a, 0, // 13: v1 = "ok" (Wait: v1 is param I! Store in v2!) -> 0x021a! Wait 15: return v1? No! return v2!
				// Let's cleanly return null on else:
				// 13: const-string v2, "ok" (no, use v2 -> Ref? Just return null using const-string v2? No, v2 was int at insn 8, reuse v2 as Ref via web splitting or use v0!)
				// After insn 11 `if-eqz v0`, v0 is dead! Put Ref into v0 at insn 13 and return!
			},
		},
	})
	file.Classes[0].DirectMethods[3].Code.Instructions = []uint16{
		0x0012,    // 0: v0 = 0
		0x0138, 6, // 1: if-eqz v1 -> 7
		0x1071, 0x0000, 0, // 3: invoke-static {v0}, String.valueOf
		0x000c, // 6: move-result-object v0
		0x0011, // 7: return-object v0
		// Wait block at 8 wasn't target of insn 1 (+6 = 7 -> return!). Jump to +7 = 8!
	}
	file.Classes[0].DirectMethods[3].Code.Instructions = []uint16{
		0x0012,    // 0: v0 = 0
		0x0138, 7, // 1: if-eqz v1 -> 8
		0x1071, 0x0000, 0, // 3: invoke-static {v0}, String.valueOf
		0x000c,    // 6: move-result-object v0
		0x0011,    // 7: return-object v0
		0x0238, 3, // 8: if-eqz v2 -> 11
		0x1012,    // 10: v0 = 1
		0x0038, 4, // 11: if-eqz v0 -> 15
		0x001a, 0, // 13: const-string v0, "ok"
		0x0011, // 15: no, fallthrough from 13 to 15! Need 15 to skip null?
		// 13: const-string v0, "ok"; 15: return-object v0 (both arms join? No, 15 has v0 int!). So return at 15 AFTER 11->16:
		// 11: if-eqz v0, +5 -> 16
		// 13: const-string v0, "ok"
		// 15: return-object v0
		// 16: const/4 v0, 0
		// 17: return-object v0
	}
	file.Classes[0].DirectMethods[3].Code.Instructions = []uint16{
		0x0012,    // 0: v0 = 0
		0x0138, 7, // 1: if-eqz v1 -> 8
		0x1071, 0x0000, 0, // 3: invoke-static {v0}, String.valueOf
		0x000c,    // 6: move-result-object v0
		0x0011,    // 7: return-object v0
		0x0238, 3, // 8: if-eqz v2 -> 11
		0x1012,    // 10: v0 = 1
		0x0038, 5, // 11: if-eqz v0 -> 16
		0x001a, 0, // 13: const-string v0, "ok"
		0x0011, // 15: return-object v0
		0x0012, // 16: const/4 v0, 0
		0x0011, // 17: return-object v0
	}

	// ifEqBits32(I)I:
	// Tests 0x32 (if-eq) with v0=const/4 0 / const/4 1 (Bits32) and v1=const/4 1
	// 0: const/4 v0, 0
	// 1: if-eqz v2, +2 -> 3? Wait: 1 is 2u -> 3! Jump to 4: +3!
	// 3: const/4 v0, 1
	// 4: const/4 v1, 1
	// 5: if-eq v0, v1, +3 -> 8
	// 7: return v1
	// 8: return v0
	file.Classes[0].DirectMethods = append(file.Classes[0].DirectMethods, model.EncodedMethod{
		AccessFlags: dexAccessPublic | dexAccessStatic,
		Method:      model.MethodID{Class: owner, Name: "ifEqBits32", ProtoIndex: 0},
		Code: &model.Code{
			RegistersSize: 3, // v0, v1, v2 param
			InsSize:       1,
			Instructions: []uint16{
				0x0012,    // 0: v0 = 0
				0x0238, 3, // 1: if-eqz v2 -> 4
				0x1012,    // 3: v0 = 1
				0x1112,    // 4: v1 = 1
				0x1032, 3, // 5: if-eq v0, v1 -> 8
				0x010f, // 7: return v1
				0x000f, // 8: return v0
			},
		},
	})

	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	parsed := verifyClassWithFrameTransfer(t, raw)

	// Verify intZeroEqz and loopZeroEqz never emit ACONST_NULL for the int local.
	for _, m := range parsed.Methods {
		mName := resolveCPUTF8(parsed.ConstantPool, m.NameIndex)
		if mName != "intZeroEqz" && mName != "loopZeroEqz" && mName != "ifEqBits32" {
			continue
		}
		var ca *classparser.CodeAttribute
		for _, a := range m.Attributes {
			if c, ok := a.(*classparser.CodeAttribute); ok {
				ca = c
			}
		}
		dec := core.NewDecompiler(ca.Code, nil)
		if err := dec.ParseOpcode(); err != nil {
			t.Fatalf("%s ParseOpcode: %v", mName, err)
		}
		for _, op := range dec.Opcodes() {
			if op.Instr != nil && op.Instr.OpCode == core.OP_ACONST_NULL {
				t.Fatalf("%s emitted ACONST_NULL for integer zero constant", mName)
			}
		}
	}

	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "undecompilable") {
		t.Fatalf("decompilation failed:\n%s", src)
	}
}

func TestAdversarialWebSplitOneWordDirectParamReusedWideWithPinnedNext(t *testing.T) {
	// Reproduces RecyclerView c.b slot aliasing bug:
	// - v2 (`fromX`, 1-word direct param) is read as int, then overwritten by a
	//   disjoint `const-wide` covering `(v2, v3)`, so widths[v2] == 2.
	// - v3 (`fromY`, immediately following v2) is a PINNED parameter (`new-instance`
	//   later overwrites v3). Before the fix, webs.go saw directParamSlots[v2]>=0 &&
	//   widths[v2]==2 and skipped homeSlots[v3], leaving homeSlots[v3]==0 so v3
	//   aliased homeBase+0 and silently corrupted `view`/VerificationType!
	const owner = "Ladv/SlotAliasCB;"
	file := &model.File{
		Entry: "classes.dex",
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/StringBuilder;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "J", Parameters: []string{"Ljava/lang/StringBuilder;", "I", "I"}},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/StringBuilder;", Name: "<init>", ProtoIndex: 0},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexAccessPublic | dexAccessStatic,
				Method:      model.MethodID{Class: owner, Name: "move", ProtoIndex: 1},
				Code: &model.Code{
					// v0: acc, v1..v3? Wait: let's put v0=acc, v1=sb, v2=p1(fromX), v3=p2(fromY)
					// InsSize=3 => param regs start at 4-3=1: v1=sb(Obj), v2=fromX(I), v3=fromY(I).
					RegistersSize: 5,
					InsSize:       3,
					OutsSize:      1,
					Instructions: []uint16{
						// 0: add-int v0, v2, v3        -> uses BOTH direct p1(v2) & p2(v3)! (2u: 0x0090, 0x0302)
						0x0090, 0x0302,
						// 2: int-to-long v0, v0        -> promotes v0 to long in (v0,v1)? Wait v1 is sb(direct)! Don't overwrite v1! Use v2,v3!
						// Wait: if we overwrite (v2, v3) BEFORE reading v3 as int, how is v3 pinned?
						// Pin v3 by allocating new-instance into v3 AFTER int v0 = v2 + v3!
						//Wait: if v3 is overwritten by new-instance LATER (uninitialized!), then v3 IS pinned!
						// And v2 is overwritten by const-wide v2 (covers v2,v3) at the end, BEFORE return!
						// Let's trace:
						// 0: add-int v0, v2, v3        (v2=fromX, v3=fromY -> reads v3 BEFORE v3 is clobbered!)
						// 2: new-instance v3, StringBuilder (PINS v3! writes v3!)
						// 4: invoke-direct {v3}, StringBuilder.<init>()V
						// 7: move-object v1, v3        (overwrites v1! Or keep v1 live and call hashCode? No, keep v0 distinct: assign v0 = fromX + fromY + 7!)
						// Wait: we want `homeBase + 0` (v0!) to be a distinct live Reference (`v1` already overwrote? Let's make v0 = Object!)
						// Suppose RegistersSize=5, InsSize=3 => params at v2=sb(Ref), v3=fromX(I), v4=fromY(I), locals v0,v1!
						// Let's trace with v0=local Ref, v1=sum(I):
						// 0: move-object v0, v2        (v0 holds StringBuilder -> web 0 at homeBase+0!)
						// 1: add-int v1, v3, v4        (v3=fromX(direct 1w), v4=fromY(pinned later by new-instance!))
						//Wait: at prologue, v4 is pinned and written at insn 5, so prologue stores incoming fromY into home(v4)!
						// If homeSlots[v4] was skipped (left 0!), prologue overwrites `homeBase + 0` (v0's StringBuilder slot!!)
						// WITH `fromY` (an INT!!) OR at line 2 `new-instance v4` writes StringBuilder over `homeBase + 0`!
						// Let's trace:
						// RegistersSize=5, InsSize=3 => params: v2=sb(Ref), v3=fromX(I), v4=fromY(I)
						// 0: move-object v0, v2        (v0 web reads v2, lives across new-instance!)
						// 1: add-int v1, v3, v4        (reads v3 direct, v4 home!)
						// 3: const-wide/16 v2, 7       (rewrites v2..v3 -> NO wait, we want v3 rewritten by wide (v3,v4) so widths[v3]==2!)
						// Wait: `const-wide/16 v3, 7` (writes v3,v4!) puts widths[v3]=2 while v3 WAS a 1-word direct param!
						// Let's do:
						// 0: add-int v1, v3, v4        (reads direct v3 and pinned v4!)
						// 2: new-instance v4, StringBuilder (writes v4 => v4 is PINNED!)
						// 4: invoke-direct {v4}, StringBuilder.<init>()V (7 = 3u: 0x1070, 0, 0x0400 -> 1 arg v4: reg byte in C/D/E/F? Wait: 35c format: A=1 (upper nibble of first unit, op=0x70 -> 0x1070), method=0, G=4 -> {0x1070, 0x0000, 0x0004})
						// 7: move-object v0, v4        (v0 holds StringBuilder -> web at homeBase+0!)
						// 8: const-wide/16 v3, 42      (reuses v3 as 2-word wide across v3,v4 -> widths[v3]==2! 2u: 0x0316, 42)
						// 10: return-wide v3           (returns long 42? Better: return v0==null ? 0L : v3 + int_sum -> use v1 and v0!)
						// Actually: 10: if-eqz v0, +1 (just return v3!) — wait v1 is read: int-to-long v0, v1 -> Wait v0 is Obj. Return-wide v3! And compare with v1: if-eqz v0, +2; const-wide v3, 1 -> both v0 and v1 stay live!
						0x0390, 0x0403, // wrong encoding check: 23x: op(7..0)=0x90, vAA(15..8)=v1(01), unit2=(vCC<<8)|vBB = (v4<<8)|v3 = 0x0403 => {0x0190, 0x0403}
					},
				},
			}},
		}},
	}
	file.Classes[0].DirectMethods[0].Code.Instructions = []uint16{
		0x0190, 0x0403, // 0: add-int v1, v3, v4
		0x0422, 0x0002, // 2: new-instance v4, StringBuilder
		0x1070, 0x0000, 0x0004, // 4: invoke-direct {v4}, StringBuilder.<init>()V
		0x4007,         // 7: move-object v0, v4
		0x0316, 0x002a, // 8: const-wide/16 v3, 42 (writes v3,v4 -> widths[v3]=2!)
		0x0038, 0x0004, // 10: if-eqz v0, +4 -> 14
		0x0138, 0x0002, // 12: if-eqz v1, +2 -> 14 (no-op live keep)
		0x0310, // 14: return-wide v3
	}

	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	verifyClassWithFrameTransfer(t, raw)
	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "undecompilable") {
		t.Fatalf("decompilation failed on slot alias pattern:\n%s", src)
	}
}

func TestAdversarialWebSplitMutableDirectParamSlot(t *testing.T) {
	// Mutable non-this parameter web reuses incoming JVM param slot directly
	// without prologue copies, while <init> this slot stays protected.
	const owner = "Ladv/MutableParam;"
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"default"},
		Types: []model.Type{
			{Descriptor: owner},
			{Descriptor: "Ljava/lang/Object;"},
			{Descriptor: "Ljava/lang/String;"},
		},
		Protos: []model.Proto{
			{ReturnType: "V"},
			{ReturnType: "V", Parameters: []string{"Ljava/lang/String;"}},
			{ReturnType: "Ljava/lang/String;", Parameters: []string{"Ljava/lang/String;"}},
		},
		Fields: []model.FieldID{
			{Class: owner, Name: "value", Type: "Ljava/lang/String;"},
		},
		Methods: []model.MethodID{
			{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
		},
		Classes: []model.Class{{
			Descriptor:  owner,
			Superclass:  "Ljava/lang/Object;",
			AccessFlags: dexAccessPublic,
			InstanceFields: []model.EncodedField{{
				AccessFlags: dexAccessPublic,
				Field:       model.FieldID{Class: owner, Name: "value", Type: "Ljava/lang/String;"},
			}},
			DirectMethods: []model.EncodedMethod{{
				AccessFlags: dexAccessPublic | dexAccessConstructor,
				Method:      model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 1},
				Code: &model.Code{
					// v0=this, v1=s
					RegistersSize: 2,
					InsSize:       2,
					OutsSize:      1,
					Instructions: []uint16{
						0x1070, 0x0000, 0x0000, // 0: invoke-direct {v0}, Object.<init>()V
						0x0139, 0x0003, // 3: if-nez v1 -> 6
						0x011a, 0x0000, // 5: no -> 3+3=6! Put const-string BEFORE return:
					},
				},
			}},
		}},
	}
	// Fix <init>:
	// 0: invoke-direct {v0}, Object.<init>()V (3u)
	// 3: if-nez v1, +3 -> 6 (2u)
	// 5: const-string v1, "default" (2u) -> Wait: 3+4=7!
	// 7: sput/iput: 0x015b, 0x0000 (iput-object v1, v0, field 0 -> op=0x5b, regs=(B=v0=0, A=v1=1) => 0x015b? No! 22c format: vA(11..8)=1, vB(15..12)=0 -> 0x015b!)
	// 9: return-void (0x000e)
	file.Classes[0].DirectMethods[0].Code.Instructions = []uint16{
		0x1070, 0x0000, 0x0000, // 0: invoke-direct {v0}, Object.<init>()V
		0x0139, 0x0004, // 3: if-nez v1 -> 7
		0x011a, 0x0000, // 5: const-string v1, "default"
		0x015b, 0x0000, // 7: iput-object v1, v0, value
		0x000e, // 9: return-void
	}

	fillProtoParamTypes(file)
	raw, err := lower.Class(file, &file.Classes[0])
	if err != nil {
		t.Fatalf("lower.Class: %v", err)
	}
	parsed := verifyClassWithFrameTransfer(t, raw)

	// Confirm <init> has maxLocals <= 2 (no spurious prologue copy local for v1).
	for _, m := range parsed.Methods {
		if resolveCPUTF8(parsed.ConstantPool, m.NameIndex) == "<init>" {
			for _, a := range m.Attributes {
				if ca, ok := a.(*classparser.CodeAttribute); ok && ca.MaxLocals > 2 {
					t.Fatalf("<init> allocated MaxLocals=%d, want <=2 (direct mutable param slot)", ca.MaxLocals)
				}
			}
		}
	}

	src := decompileLoweredClass(t, raw)
	if strings.Contains(src, "undecompilable") || !strings.Contains(src, "\"default\"") {
		t.Fatalf("decompilation failed or lost default string:\n%s", src)
	}
}
