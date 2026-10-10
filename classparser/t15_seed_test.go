package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const effectOrderSrc = `public class EffectOrder {
  static String trace=""; static int i=0;
  static int f(){trace += "f"; return 7;}
  static boolean yes(){trace += "y";return true;}
  static boolean no(){trace += "n";return false;}
  public static void main(String[] args) {
    int[] a={0,0}; a[i++]=f(); boolean b=no()&&yes(); boolean c=yes()||no();
    System.out.println(trace);System.out.println(i);System.out.println(a[0]);System.out.println(b);System.out.println(c);
    try { int x=a[99]+f(); System.out.println(x); } catch (ArrayIndexOutOfBoundsException e) { System.out.println(trace); }
  }
}
`

const effectOrderInBoundSrc = `public class EffectOrderInBound {
  static String trace="";
  static int f(){trace += "f"; return 7;}
  public static void main(String[] args) {
    int[] a={0,0}; int x=a[0]+f();
    System.out.println(trace);System.out.println(x);System.out.println(a[0]);
  }
}
`

const monitorReleaseSrc = `public class MonitorRelease {
  static final Object LOCK=new Object();
  static boolean held(){return Thread.holdsLock(LOCK);}
  static void f(){synchronized(LOCK){System.out.println(held());throw new IllegalStateException("lock");}}
  public static void main(String[] a){System.out.println(held());try{f();}catch(IllegalStateException e){System.out.println(e.getMessage());}System.out.println(held());synchronized(LOCK){System.out.println(held());}System.out.println(held());}
}
`

const partialArrayInitSrc = `public class PartialArrayInit {
  static int boom(){ throw new RuntimeException("boom"); }
  public static void main(String[] args) {
    int[] a = new int[3];
    try {
      a[0] = 1;
      a[1] = boom();
      a[2] = 3;
    } catch (RuntimeException e) {
      System.out.println(a[0]);
      System.out.println(a[1]);
      System.out.println(a[2]);
    }
  }
}
`

const clinitVolatileSrc = `public class EffectClinit {
  static int f(){ System.out.print("F"); return 1; }
  public static void main(String[] args) {
    int x = ClinitSide.V + f();
    System.out.println();
    System.out.println(x);
    VolatileBox.v = f();
    System.out.println(VolatileBox.v);
    synchronized(VolatileBox.lock){ System.out.println("S"); }
  }
}
`
const clinitSideSrc = `public class ClinitSide {
  static { System.out.print("C"); }
  static int V = 7;
}
`
const volatileBoxSrc = `public class VolatileBox {
  static volatile int v;
  static final Object lock = new Object();
}
`

func TestT15_C01_EffectOrderEvalAndShortCircuit(t *testing.T) {
	t.Run("T15-C01", func(t *testing.T) { testT15C01(t) })
}

func testT15C01(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"EffectOrder.java": effectOrderSrc}, "EffectOrder", "8")
	want := "fny\n1\n7\nfalse\ntrue\nfny\n"
	if orig != want {
		t.Fatalf("T15-C01 original stdout=%q want %q", orig, want)
	}
	if rebuilt != orig {
		t.Fatalf("T15-C01 rebuilt stdout=%q want original %q\nsource:\n%s", rebuilt, orig, decompiled["EffectOrder"])
	}
	src := decompiled["EffectOrder"]
	if strings.Count(src, "i++") == 0 && strings.Count(src, "i += 1") == 0 && !strings.Contains(src, "i = i + 1") {
		// postfix may render as i++ or a slot temp; require the trace still matches
		t.Logf("T15-C01 decompiled source (i++ form not obvious):\n%s", src)
	}
}

func TestT15_C02_OOBBlocksCallVsInBoundNeighbor(t *testing.T) {
	t.Run("T15-C02", func(t *testing.T) { testT15C02(t) })
}

func testT15C02(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"EffectOrder.java": effectOrderSrc}, "EffectOrder", "8")
	if orig != rebuilt {
		t.Fatalf("T15-C02 OOB path diverged orig=%q rebuilt=%q\n%s", orig, rebuilt, decompiled["EffectOrder"])
	}
	if !strings.HasSuffix(strings.TrimSpace(orig), "fny") {
		t.Fatalf("T15-C02 catch trace changed, f ran after AIOOBE: %q", orig)
	}
	nOrig, nRebuilt, nDec := roundTripFamily(t, map[string]string{"EffectOrderInBound.java": effectOrderInBoundSrc}, "EffectOrderInBound", "8")
	if nOrig != "f\n7\n0\n" {
		t.Fatalf("T15-C02 inbound original=%q", nOrig)
	}
	if nRebuilt != nOrig {
		t.Fatalf("T15-C02 inbound rebuilt=%q source:\n%s", nRebuilt, nDec["EffectOrderInBound"])
	}
}

func TestT15_C03_PartialArrayInitNotFolded(t *testing.T) {
	t.Run("T15-C03", func(t *testing.T) { testT15C03(t) })
}

func testT15C03(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"PartialArrayInit.java": partialArrayInitSrc}, "PartialArrayInit", "8")
	want := "1\n0\n0\n"
	if orig != want {
		t.Fatalf("T15-C03 original=%q want %q", orig, want)
	}
	if rebuilt != orig {
		t.Fatalf("T15-C03 rebuilt=%q (folded initializer would hide the store of 1)\nsource:\n%s", rebuilt, decompiled["PartialArrayInit"])
	}
	src := decompiled["PartialArrayInit"]
	if strings.Contains(src, "new int[]{1") || strings.Contains(src, "new int[] {1") {
		t.Fatalf("T15-C03 folded mid-throw init into one initializer:\n%s", src)
	}
}

func TestT15_C04_ClassInitAndVolatile(t *testing.T) {
	t.Run("T15-C04", func(t *testing.T) { testT15C04(t) })
}

func testT15C04(t *testing.T) {
	sources := map[string]string{
		"EffectClinit.java": clinitVolatileSrc,
		"ClinitSide.java":   clinitSideSrc,
		"VolatileBox.java":  volatileBoxSrc,
	}
	orig, rebuilt, decompiled := roundTripFamily(t, sources, "EffectClinit", "8")
	if orig != "CF\n8\nF1\nS\n" {
		t.Fatalf("T15-C04 original=%q (clinit must precede f)", orig)
	}
	if rebuilt != orig {
		t.Fatalf("T15-C04 rebuilt=%q source:\n%s", rebuilt, decompiled["EffectClinit"])
	}
}

func TestT15_C06_MonitorRelease(t *testing.T) {
	t.Run("T15-C06", func(t *testing.T) { testT15C06(t) })
}

func testT15C06(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"MonitorRelease.java": monitorReleaseSrc}, "MonitorRelease", "8")
	want := "false\ntrue\nlock\nfalse\ntrue\nfalse\n"
	if orig != want {
		t.Fatalf("T15-C06 original=%q want %q", orig, want)
	}
	if rebuilt != orig {
		t.Fatalf("T15-C06 rebuilt=%q source:\n%s", rebuilt, decompiled["MonitorRelease"])
	}
}

func TestT15_C05_ProductionArrayFoldRejectsOpaque(t *testing.T) {
	t.Run("T15-C05", func(t *testing.T) { testT15C05Production(t) })
}

func testT15C05Production(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	arrType := types.NewJavaArrayType(intType)
	root := utils.NewRootVariableId()
	ref := values.NewJavaRef(root.Next(), nil, arrType)

	newGraph := func(rhs1 values.JavaValue) (*core.Node, *values.NewExpression) {
		length := values.NewJavaLiteral(2, intType)
		newExp := values.NewNewArrayExpression(arrType, length)
		n0 := core.NewNode(statements.NewAssignStatement(ref, newExp, true))
		n1 := core.NewNode(&statements.AssignStatement{
			ArrayMember: values.NewJavaArrayMember(ref, values.NewJavaLiteral(0, intType)),
			JavaValue:   values.NewJavaLiteral(1, intType),
		})
		n2 := core.NewNode(&statements.AssignStatement{
			ArrayMember: values.NewJavaArrayMember(ref, values.NewJavaLiteral(1, intType)),
			JavaValue:   rhs1,
		})
		n3 := core.NewNode(statements.NewCustomStatement(func(*class_context.ClassContext) string { return ";" }, func(_, _ *utils.VariableId) {}))
		n0.AddNext(n1)
		n1.AddNext(n2)
		n2.AddNext(n3)
		return n0, newExp
	}

	pure, exp := newGraph(values.NewJavaLiteral(2, intType))
	core.RewriteNewArrayList(pure, map[string][3]int{})
	if len(exp.Initializer) != 2 {
		t.Fatalf("pure sequential fill should fold, initializer=%d", len(exp.Initializer))
	}

	opaque := values.NewCustomValue(func(*class_context.ClassContext) string { return "side()" }, func() types.JavaType { return intType })
	if values.MayFold(opaque) || values.IsPure(opaque) {
		t.Fatal("opaque CustomValue must be a production barrier")
	}
	eff, _ := values.InspectValue(opaque)
	if eff&values.EffectOpaque == 0 {
		t.Fatal("InspectValue (used by RewriteNewArrayList) must set EffectOpaque")
	}
	bad, exp2 := newGraph(opaque)
	core.RewriteNewArrayList(bad, map[string][3]int{})
	if len(exp2.Initializer) != 0 {
		t.Fatalf("opaque RHS must not fold to an array initializer, got %d", len(exp2.Initializer))
	}
}
