package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const hierarchyJoinSrc = `public class HierarchyJoin {
  static java.util.List<String> f(boolean c){java.util.List<String> x;if(c)x=new java.util.ArrayList<String>();else x=new java.util.LinkedList<String>();x.add("ok");return x;}
  static Number g(boolean c){Number n;if(c)n=Integer.valueOf(3);else n=Long.valueOf(4);return n;}
  public static void main(String[] a){System.out.println(f(true).get(0));System.out.println(f(false).get(0));System.out.println(g(true).longValue());System.out.println(g(false).longValue());}
}
`

const diamondISrc = `interface DiamondI { void m(); }
`
const diamondASrc = `class DiamondA {}
`
const diamondBSrc = `class DiamondB {}
`
const diamondCSrc = `class DiamondC extends DiamondA implements DiamondI { public void m(){ System.out.print("C"); } }
`
const diamondDSrc = `class DiamondD extends DiamondB implements DiamondI { public void m(){ System.out.print("D"); } }
`
const diamondMainSrc = `public class HierarchyDiamond {
  static DiamondI pick(boolean c){ DiamondI x; if(c) x=new DiamondC(); else x=new DiamondD(); x.m(); return x; }
  public static void main(String[] a){ pick(true); pick(false); System.out.println(); }
}
`

const memberJoinSrc = `import java.lang.reflect.*;
public class MemberJoin {
  static String n(boolean c, Method m, Field f){
    Member x = c ? m : f;
    return x.getName();
  }
  public static void main(String[] a) throws Exception {
    Method m = String.class.getMethod("length");
    Field f = String.class.getDeclaredField("hash");
    System.out.println(n(true, m, f));
    System.out.println(n(false, m, f));
  }
}
`

const hiddenMemberSrc = `package hidden;
class HiddenMember implements java.lang.reflect.Member {
  public Class<?> getDeclaringClass(){ return HiddenMember.class; }
  public String getName(){ return "hidden"; }
  public int getModifiers(){ return 0; }
  public boolean isSynthetic(){ return false; }
}
`

func TestT16_C01_HierarchyJoinListNumber(t *testing.T) {
	t.Run("T16-C01", func(t *testing.T) { testT16C01(t) })
}

func testT16C01(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"HierarchyJoin.java": hierarchyJoinSrc}, "HierarchyJoin", "8")
	want := "ok\nok\n3\n4\n"
	if orig != want {
		t.Fatalf("T16-C01 original=%q want %q", orig, want)
	}
	if rebuilt != orig {
		t.Fatalf("T16-C01 rebuilt=%q source:\n%s", rebuilt, decompiled["HierarchyJoin"])
	}
	src := decompiled["HierarchyJoin"]
	if strings.Contains(src, "ArrayList") && strings.Contains(src, "ArrayList<") && strings.Contains(src, "x=") {
		if strings.Contains(src, "ArrayList x") || strings.Contains(src, "ArrayList var") {
			t.Fatalf("T16-C01 locked to first-arm ArrayList:\n%s", src)
		}
	}
	if strings.Contains(src, "Integer var") && strings.Contains(src, "Long.valueOf") {
		t.Fatalf("T16-C01 locked to first-arm Integer:\n%s", src)
	}
}

func TestT16_C02_CustomDiamond(t *testing.T) {
	t.Run("T16-C02", func(t *testing.T) { testT16C02(t) })
}

func testT16C02(t *testing.T) {
	sources := map[string]string{
		"DiamondI.java":         diamondISrc,
		"DiamondA.java":         diamondASrc,
		"DiamondB.java":         diamondBSrc,
		"DiamondC.java":         diamondCSrc,
		"DiamondD.java":         diamondDSrc,
		"HierarchyDiamond.java": diamondMainSrc,
	}
	orig, rebuilt, decompiled := roundTripFamily(t, sources, "HierarchyDiamond", "8")
	if orig != "CD\n" {
		t.Fatalf("T16-C02 original=%q", orig)
	}
	if rebuilt != orig {
		t.Fatalf("T16-C02 rebuilt=%q\n%s", rebuilt, decompiled["HierarchyDiamond"])
	}
	dir := t.TempDir()
	compileJavaRelease(t, dir, sources, "8")
	classes := classMapFromDir(t, dir)
	p, provider := SuperTypeProviderFromClassBytes(classes, "8", "diamond", nil)
	got := types.MergeTypesVia(provider, types.NewJavaClass("DiamondC"), types.NewJavaClass("DiamondD"))
	if n, _ := types.RawClassFQN(got); n != "DiamondI" {
		t.Fatalf("T16-C02 join=%s want DiamondI (not a name heuristic)", n)
	}
	ident, ok, _ := p.Lookup("DiamondC")
	if !ok || ident.SuperClass == "" || len(ident.Interfaces) == 0 {
		t.Fatalf("T16-C02 metadata missing: %+v", ident)
	}
	src := decompiled["HierarchyDiamond"]
	if strings.Contains(src, "DiamondC x") || strings.Contains(src, "DiamondC var") {
		t.Fatalf("T16-C02 locked to first-arm DiamondC:\n%s", src)
	}
}

func TestT16_C03_SameSimpleNameDifferentIdentity(t *testing.T) {
	t.Run("T16-C03", func(t *testing.T) { testT16C03(t) })
}

func testT16C03(t *testing.T) {
	v1 := map[string]string{
		"a/X.java": `package a; public class X { public int n(){ return 1; } }`,
		"b/X.java": `package b; public class X { public int n(){ return 2; } }`,
		"app/SameName.java": `package app; public class SameName {
  public static void main(String[] args) {
    a.X ax = new a.X(); b.X bx = new b.X();
    System.out.println(ax.n()); System.out.println(bx.n());
  }
}`,
	}
	orig, rebuilt, decompiled := roundTripFamily(t, v1, "app.SameName", "8")
	if orig != "1\n2\n" {
		t.Fatalf("T16-C03 original=%q", orig)
	}
	if rebuilt != orig {
		t.Fatalf("T16-C03 rebuilt=%q src=%s", rebuilt, decompiled["app/SameName"])
	}
	src := decompiled["app/SameName"]
	if strings.Contains(src, "import b.X") && !strings.Contains(src, "a.X") {
		t.Fatalf("T16-C03 wrong import/cast:\n%s", src)
	}

	dir1 := t.TempDir()
	compileJavaRelease(t, dir1, map[string]string{"a/X.java": `package a; public class X { public int n(){ return 1; } }`}, "8")
	dir2 := t.TempDir()
	compileJavaRelease(t, dir2, map[string]string{"a/X.java": `package a; public class X { public int n(){ return 99; } }`}, "8")
	c1 := classMapFromDir(t, dir1)
	c2 := classMapFromDir(t, dir2)
	p1, _ := SuperTypeProviderFromClassBytes(c1, "8", "v1", nil)
	p2, _ := SuperTypeProviderFromClassBytes(c2, "8", "v2", nil)
	i1, ok1, _ := p1.Lookup("a/X")
	i2, ok2, _ := p2.Lookup("a/X")
	if !ok1 || !ok2 {
		t.Fatal("T16-C03 both versions should resolve")
	}
	if i1.ContentHash == i2.ContentHash {
		t.Fatal("T16-C03 different class bytes must not share content identity")
	}
	if _, ok, _ := p1.Lookup("b/X"); ok {
		t.Fatal("T16-C03 v1 provider must not resolve a name from the other classpath")
	}
}

func TestT16_C06_MemberJoinAndInaccessible(t *testing.T) {
	t.Run("T16-C06", func(t *testing.T) { testT16C06Seed(t) })
}

func testT16C06Seed(t *testing.T) {
	orig, rebuilt, decompiled := roundTripFamily(t, map[string]string{"MemberJoin.java": memberJoinSrc}, "MemberJoin", "8")
	if orig != rebuilt {
		t.Fatalf("T16-C06 rebuilt diverged orig=%q rebuilt=%q\n%s", orig, rebuilt, decompiled["MemberJoin"])
	}
	src := decompiled["MemberJoin"]
	if strings.Contains(src, "AccessibleObject ") && !strings.Contains(src, "Member ") {
		t.Fatalf("T16-C06 used non-denotable AccessibleObject:\n%s", src)
	}
	if strings.Contains(src, ".getName()") && strings.Contains(src, "AccessibleObject") && !strings.Contains(src, "Member") {
		t.Fatalf("T16-C06 invoke target broken:\n%s", src)
	}

	dir := t.TempDir()
	compileJavaRelease(t, dir, map[string]string{"hidden/HiddenMember.java": hiddenMemberSrc}, "8")
	classes := classMapFromDir(t, dir)
	p, provider := SuperTypeProviderFromClassBytes(classes, "8", "hidden", nil)
	access := func(name string) (bool, bool) {
		if name == "hidden/HiddenMember" {
			return false, true
		}
		if strings.HasPrefix(name, "java/lang/reflect/") {
			return true, true
		}
		return false, false
	}
	got := types.JoinTypesAccessible(types.NewJavaClass("hidden.HiddenMember"), types.NewJavaClass("java.lang.reflect.Method"), provider, access)
	if n, _ := types.RawClassFQN(got.Source); n == "hidden.HiddenMember" {
		t.Fatal("T16-C06 inaccessible impl used as source declaration")
	}
	ident, ok, _ := p.Lookup("hidden/HiddenMember")
	if !ok {
		t.Fatal("hidden class should parse without initialization")
	}
	if ident.ContentHash == "" {
		t.Fatal("missing content identity")
	}
}
