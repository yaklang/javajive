package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialAnonymousCovariantBoxedAndArrayCalls(t *testing.T) {
	cases := []struct{ name, typ, values, equal, zero string }{
		{"integer", "int", "new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}", "%s==%s", "0"},
		{"wide", "long", "new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}", "%s==%s", "0L"},
		{"floating", "double", "new double[]{Double.longBitsToDouble(0x7ff8000000001234L),Double.NEGATIVE_INFINITY,-0.0d,0.0d,Double.POSITIVE_INFINITY}", "Double.doubleToRawLongBits(%s)==Double.doubleToRawLongBits(%s)", "0.0d"},
		{"boolean", "boolean", "new boolean[]{false,true}", "%s==%s", "false"},
		{"reference", "Object", "new Object[]{null,new Object(),new StringBuilder(\"identity\")}", "%s==%s", "null"},
		{"array", "Object[]", "new Object[][]{null,new Object[0],new Object[]{new Object()}}", "%s==%s", "null"},
	}
	for _, row := range cases {
		for _, shape := range []string{"anonymous"} {
			for _, storage := range []string{"inherited"} {
				for _, prefix := range []string{"Holder", "Cove"} {
					t.Run(row.name+"/"+shape+"/"+storage+"/"+prefix, func(t *testing.T) {
						T := row.typ
						consumer := fmt.Sprintf("class Writer{%s put(%s next){return p.Effects.next(next);}}public %s put(%s next){return new Writer().put(next);}", T, T, T, T)
						if shape == "deep named" {
							consumer = fmt.Sprintf("class Writer{class Leaf{%s put(%s next){return p.Effects.next(next);}}%s put(%s next){return new Leaf().put(next);}}public %s put(%s next){return new Writer().put(next);}", T, T, T, T, T, T)
						}
						if shape == "anonymous" {
							box := T
							if T == "int" {
								box = "Integer"
							}
							if T == "long" {
								box = "Long"
							}
							if T == "double" {
								box = "Double"
							}
							if T == "boolean" {
								box = "Boolean"
							}
							consumer = fmt.Sprintf("public %s put(final %s next){return new java.util.function.Supplier<%s>(){public %s get(){return p.Effects.next(next);}}.get();}", T, T, box, box)
						}
						own := ""
						if storage == "own private" {
							own = "private " + T + " var0;"
						}
						equal := func(a, b string) string { return fmt.Sprintf(row.equal, a, b) }
						base := fmt.Sprintf("package p;public abstract class Base{protected %s var0;public abstract %s put(%s next);public abstract %s get();public %s inherited(){return var0;}}", T, T, T, T, T)
						effects := fmt.Sprintf("package p;public class Effects{public static int calls;public static boolean fail;public static final RuntimeException failure=new IllegalArgumentException(\"identity\");public static %s next(%s value){calls++;if(fail)throw failure;return value;}}", T, T)
						owner := "package q;public class HolderOwner{public static p.Base build(){return new p.Base(){" + own + consumer + "public " + T + " get(){return var0;}};}}"
						driver := fmt.Sprintf(`package q;public class HolderDriver{public static void main(String[]args)throws Exception{p.Base owner=HolderOwner.build();if(!owner.getClass().isAnonymousClass()||owner.getClass().getEnclosingMethod().getDeclaringClass()!=HolderOwner.class)throw new AssertionError("original anonymous source scope");int rows=0;for(%s value:%s){p.Effects.calls=0;if(!(%s)||p.Effects.calls!=1)throw new AssertionError("original covariant forwarding identity/effect");rows++;}System.out.println(rows+":typed:anonymous:storage:identity:order");}}`, T, row.values, equal("owner.put(value)", "value"))

						sources := map[string]string{"p/Base.java": base, "p/Effects.java": effects, "q/HolderOwner.java": owner, "q/HolderDriver.java": driver}
						ss := map[string]string{}
						for n, s := range sources {
							ss[strings.ReplaceAll(n, "Holder", prefix)] = strings.ReplaceAll(s, "Holder", prefix)
						}
						count := 5
						if row.name == "boolean" {
							count = 2
						}
						if row.name == "reference" || row.name == "array" {
							count = 3
						}
						testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/"+prefix+"Owner", "q."+prefix+"Driver", fmt.Sprintf("%d:typed:anonymous:storage:identity:order\n", count), func(t *testing.T, debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, ss, debug, "8")
						}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
					})
				}
			}
		}
	}
}
