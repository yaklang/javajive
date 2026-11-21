package ssalower

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

func buildSSAForBytes(t *testing.T, code []byte, desc string, ex []*core.ExceptionTableEntry) *ssabuild.Function {
	t.Helper()
	ir, err := methodir.BuildFromBytes(code, ex, methodir.MethodMeta{
		ClassName:  "AdvExPhi",
		Name:       "m",
		Descriptor: desc,
		IsStatic:   true,
		Bytecode:   code,
	}, nil)
	if err != nil {
		t.Fatalf("BuildFromBytes: %v", err)
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatalf("ssabuild.Build: %v", err)
	}
	return fn
}

// TestAdversarialExceptionPhiWideningAndSwap verifies that try/catch handler
// local phis accept lattice widenings (Null -> Ref, String -> Integer -> Object)
// while still rejecting unsafe narrowings and category punning.
func TestAdversarialExceptionPhiWideningAndSwap(t *testing.T) {
	t.Run("null_to_string_try_catch_phi", func(t *testing.T) {
		// static (String s, int d)Object:
		// local2 = null; 1/d; local2 = s; 1/d; return local2
		// catch -> astore_3; aload_2; areturn
		code := []byte{
			core.OP_ACONST_NULL, core.OP_ASTORE_2, // pc 0..1
			core.OP_ICONST_1, core.OP_ILOAD_1, core.OP_IDIV, core.OP_POP, // pc 2..5 (idiv@4)
			core.OP_ALOAD_0, core.OP_ASTORE_2, // pc 6..7
			core.OP_ICONST_1, core.OP_ILOAD_1, core.OP_IDIV, core.OP_POP, // pc 8..11 (idiv@10)
			core.OP_ALOAD_2, core.OP_ARETURN, // pc 12..13
			core.OP_ASTORE_3, core.OP_ALOAD_2, core.OP_ARETURN, // handler pc 14..16
		}
		fn := buildSSAForBytes(t, code, "(Ljava/lang/String;I)Ljava/lang/Object;", []*core.ExceptionTableEntry{
			{StartPc: 2, EndPc: 12, HandlerPc: 14, CatchType: 0},
		})
		low, err := Destroy(fn)
		if err != nil {
			t.Fatalf("Destroy(null->String handler phi): %v", err)
		}
		if !low.ThrowsUnmoved() {
			t.Fatal("ThrowsUnmoved returned false")
		}
		if len(low.Exception.Sites) != 2 {
			t.Fatalf("expected 2 spill sites, got %+v", low.Exception.Sites)
		}
	})

	t.Run("disjoint_ref_types_widen_to_object_try_catch_phi", func(t *testing.T) {
		// static (String s, Integer n, int d)Object:
		// local3 = s; 1/d; local3 = n; 1/d; return local3
		// catch -> astore 4; aload_3; areturn
		code := []byte{
			core.OP_ALOAD_0, core.OP_ASTORE_3, // pc 0..1
			core.OP_ICONST_1, core.OP_ILOAD_2, core.OP_IDIV, core.OP_POP, // pc 2..5 (idiv@4)
			core.OP_ALOAD_1, core.OP_ASTORE_3, // pc 6..7
			core.OP_ICONST_1, core.OP_ILOAD_2, core.OP_IDIV, core.OP_POP, // pc 8..11 (idiv@10)
			core.OP_ALOAD_3, core.OP_ARETURN, // pc 12..13
			core.OP_ASTORE, 4, core.OP_ALOAD_3, core.OP_ARETURN, // handler pc 14..17
		}
		fn := buildSSAForBytes(t, code, "(Ljava/lang/String;Ljava/lang/Integer;I)Ljava/lang/Object;", []*core.ExceptionTableEntry{
			{StartPc: 2, EndPc: 12, HandlerPc: 14, CatchType: 0},
		})
		low, err := Destroy(fn)
		if err != nil {
			t.Fatalf("Destroy(String->Integer handler phi): %v", err)
		}
		if !low.ThrowsUnmoved() {
			t.Fatal("ThrowsUnmoved returned false")
		}
		if len(low.Exception.Sites) != 2 {
			t.Fatalf("expected 2 spill sites, got %+v", low.Exception.Sites)
		}
	})

	t.Run("try_catch_with_swap_cycle_and_critical_edge", func(t *testing.T) {
		// Loop that swaps local1 and local2 while a protected idiv feeds catch handler reading local1:
		// d=arg0, a=1, b=2, i=arg0
		var c []byte
		c = append(c, core.OP_ICONST_1, core.OP_ISTORE_1, core.OP_ICONST_2, core.OP_ISTORE_2)
		tryStart := uint16(len(c))
		loopHdr := len(c)
		c = append(c, core.OP_ILOAD_0)
		ifPC := len(c)
		c = append(c, core.OP_IFLE, 0, 0)
		// protected body: 1/d; swap (a,b)=(b,a); d--
		c = append(c, core.OP_ICONST_1, core.OP_ILOAD_0, core.OP_IDIV, core.OP_POP)
		c = append(c, core.OP_ILOAD_1, core.OP_ISTORE_3, core.OP_ILOAD_2, core.OP_ISTORE_1, core.OP_ILOAD_3, core.OP_ISTORE_2)
		c = append(c, core.OP_IINC, 0, 0xff) // arg0--
		gotoPC := len(c)
		c = append(c, core.OP_GOTO, 0, 0)
		exitPC := len(c)
		c = append(c, core.OP_ILOAD_1, core.OP_IRETURN)
		handlerPC := len(c)
		c = append(c, core.OP_ASTORE_3, core.OP_ILOAD_1, core.OP_IRETURN)

		binary.BigEndian.PutUint16(c[ifPC+1:], uint16(exitPC-ifPC))
		binary.BigEndian.PutUint16(c[gotoPC+1:], uint16(int16(loopHdr-gotoPC)))

		fn := buildSSAForBytes(t, c, "(I)I", []*core.ExceptionTableEntry{
			{StartPc: tryStart, EndPc: uint16(exitPC), HandlerPc: uint16(handlerPC), CatchType: 0},
		})
		low, err := Destroy(fn)
		if err != nil {
			t.Fatalf("Destroy(try/catch + swap cycle): %v", err)
		}
		if !low.ThrowsUnmoved() {
			t.Fatal("ThrowsUnmoved returned false")
		}
	})

	t.Run("rejects_narrowing_and_category_punning", func(t *testing.T) {
		if compatibleExceptionSpillType(frametransfer.RefOf("java/lang/String"), frametransfer.RefOf("java/lang/Object")) {
			t.Fatal("compatibleExceptionSpillType accepted unsafe narrowing Object -> String")
		}
		if compatibleExceptionSpillType(frametransfer.T(frametransfer.Float), frametransfer.T(frametransfer.Int)) {
			t.Fatal("compatibleExceptionSpillType accepted category punning Int -> Float")
		}

		// Forge a narrowed phi type inside a real 2-throw-site SSA function and confirm Destroy rejects it.
		code := []byte{
			core.OP_ALOAD_0, core.OP_ASTORE_3,
			core.OP_ICONST_1, core.OP_ILOAD_2, core.OP_IDIV, core.OP_POP,
			core.OP_ALOAD_1, core.OP_ASTORE_3,
			core.OP_ICONST_1, core.OP_ILOAD_2, core.OP_IDIV, core.OP_POP,
			core.OP_ALOAD_3, core.OP_ARETURN,
			core.OP_ASTORE, 4, core.OP_ALOAD_3, core.OP_ARETURN,
		}
		fn := buildSSAForBytes(t, code, "(Ljava/lang/String;Ljava/lang/Integer;I)Ljava/lang/Object;", []*core.ExceptionTableEntry{
			{StartPc: 2, EndPc: 12, HandlerPc: 14, CatchType: 0},
		})
		mutated := false
		for i := range fn.Phis {
			if fn.Phis[i].Slot.Local {
				fn.Phis[i].Type = frametransfer.RefOf("java/lang/String")
				mutated = true
			}
		}
		if !mutated {
			t.Fatal("expected local phi at catch handler")
		}
		if _, err := Destroy(fn); err == nil || !strings.Contains(err.Error(), "requires type conversion") {
			t.Fatalf("Destroy(narrowed handler phi) error = %v, want type conversion refusal", err)
		}
	})
}
