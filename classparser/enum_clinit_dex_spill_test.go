package javaclassparser

import (
	"strings"
	"testing"
)

func TestFixEnumClinitIllegalNewDexSpilledLocals(t *testing.T) {
	t.Run("state_constraint_dex_spilled_locals_and_dead_alias", func(t *testing.T) {
		in := `package androidx.constraintlayout.core.state;

public enum State$Constraint {
	LEFT_TO_LEFT,
	LEFT_TO_RIGHT,
	TOP_TO_BOTTOM,
	CENTER_VERTICALLY;

	static  {
		String var0 = "LEFT_TO_LEFT";
		int var1 = 0;
		State$Constraint var2 = new State$Constraint(var0,var1);
		LEFT_TO_LEFT = var2;
		String var3 = "LEFT_TO_RIGHT";
		int var4 = 1;
		State$Constraint var5 = new State$Constraint(var3,var4);
		LEFT_TO_RIGHT = var5;
		int var6 = 8;
		String var7 = "TOP_TO_BOTTOM";
		int var8 = 2;
		int var9 = 9;
		State$Constraint var10 = new State$Constraint(var7,var9);
		TOP_TO_BOTTOM = var10;
		int var11 = 15;
		String var12 = "CENTER_VERTICALLY";
		State$Constraint var13 = var10;
		var1 = 16;
		State$Constraint var14 = new State$Constraint(var12,var1);
		CENTER_VERTICALLY = var14;
		$VALUES = new State$Constraint[]{LEFT_TO_LEFT,LEFT_TO_RIGHT,TOP_TO_BOTTOM,CENTER_VERTICALLY};
	}
	private State$Constraint() {
	}
}
`
		got := fixEnumClinitIllegalNew(in)
		if strings.Contains(got, "new State$Constraint(") {
			t.Fatalf("illegal new Enum remained:\n%s", got)
		}
		if strings.Contains(got, "LEFT_TO_LEFT = ") || strings.Contains(got, "CENTER_VERTICALLY = ") || strings.Contains(got, "$VALUES") {
			t.Fatalf("illegal static constant assignment remained:\n%s", got)
		}
		if strings.Contains(got, "var0") || strings.Contains(got, "var1") || strings.Contains(got, "var10") || strings.Contains(got, "var13") {
			t.Fatalf("dead spilled locals remained:\n%s", got)
		}
	})

	t.Run("dex_spilled_with_payload_arg", func(t *testing.T) {
		in := `enum Kind {
	A,
	B;
	static {
		String var0 = "A";
		int var1 = 0;
		int var2 = 42;
		Kind var3 = new Kind(var0,var1,var2);
		A = var3;
		String var4 = "B";
		int var5 = 1;
		int var6 = 99;
		B = new Kind(var4,var5,var6);
		$VALUES = new Kind[]{A,B};
	}
}
`
		got := fixEnumClinitIllegalNew(in)
		if !strings.Contains(got, "\tA(42),\n") || !strings.Contains(got, "\tB(99);\n") {
			t.Fatalf("payload arguments were not folded into enum constants:\n%s", got)
		}
		if strings.Contains(got, "new Kind(") || strings.Contains(got, "var0") || strings.Contains(got, "var6") {
			t.Fatalf("spilled enum locals/news remained:\n%s", got)
		}
	})

	t.Run("surviving_read_rewrites_temp_to_constant_instead_of_dropping", func(t *testing.T) {
		in := `enum Mode {
	FAST;
	static {
		String var0 = "FAST";
		int var1 = 0;
		Mode var2 = new Mode(var0,var1);
		FAST = var2;
		CACHE.put("fast", var2);
		$VALUES = new Mode[]{FAST};
	}
}
`
		got := fixEnumClinitIllegalNew(in)
		if strings.Contains(got, "new Mode(") || strings.Contains(got, "FAST = var2;") {
			t.Fatalf("illegal new/assignment remained:\n%s", got)
		}
		if !strings.Contains(got, "Mode var2 = FAST;") {
			t.Fatalf("expected surviving temp binding `Mode var2 = FAST;`, got:\n%s", got)
		}
		if strings.Contains(got, "var0") || strings.Contains(got, "var1") {
			t.Fatalf("consumed name/ordinal locals remained:\n%s", got)
		}
	})

	t.Run("kill_switch_preserves_input", func(t *testing.T) {
		in := `enum X {
	A;
	static {
		String var0 = "A";
		int var1 = 0;
		X var2 = new X(var0,var1);
		A = var2;
	}
}
`
		t.Setenv("JDEC_ENUM_CLINIT_NEW_OFF", "1")
		if got := fixEnumClinitIllegalNew(in); got != in {
			t.Fatalf("kill-switch modified source:\n%s", got)
		}
	})
}
