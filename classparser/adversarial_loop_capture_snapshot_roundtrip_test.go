package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const loopSelectedCaptureSnapshotSource = `
interface SnapshotView { int read(); }
class SnapshotOwner {
 static SnapshotView make(long[] costs) {
  int lead = -1;
  for (int i = 0; i < costs.length; i++) if (lead == -1 || costs[i] < costs[lead]) lead = i;
  final int snapshot = lead;
  return new SnapshotView() { public int read() { return snapshot; } };
 }
}
class SnapshotDriver {
 public static void main(String[] args) throws Exception {
  int rows = 0, cases = 1;
  long[] words = {Long.MIN_VALUE, -1, 0, 1, Long.MAX_VALUE};
  for (int length = 0; length <= 4; length++, cases *= 5) for (int code = 0; code < cases; code++) {
   long[] costs = new long[length]; int value = code;
   java.util.TreeMap<java.math.BigInteger, Integer> order = new java.util.TreeMap<>();
   for (int i = 0; i < length; i++) {
    costs[i] = words[value % 5]; value /= 5;
    java.math.BigInteger word = java.math.BigInteger.valueOf(costs[i]);
    if (!order.containsKey(word)) order.put(word, i);
   }
   int expected = order.isEmpty() ? -1 : order.firstEntry().getValue();
   SnapshotView view = SnapshotOwner.make(costs);
   for (int i = 0; i < length; i++) costs[i] = Long.MAX_VALUE - i;
   if (!view.getClass().isAnonymousClass() || view.getClass().getEnclosingClass() != SnapshotOwner.class || !view.getClass().getDeclaredField("val$snapshot").isSynthetic()) throw new AssertionError("original anonymous capture identity");
   if (view.read() != expected || view.read() != expected) throw new AssertionError("selected original value must be captured once");
   rows++;
  }
  try { SnapshotOwner.make(null); throw new AssertionError("null input must fail in the factory"); }
  catch (NullPointerException expected) { rows++; }
  System.out.println(rows + ":loop:selection:snapshot:mutation:ties:overflow");
 }
}
`

// The arbitrary-precision oracle masks each operation to the JVM word width;
// the reference oracle searches from the opposite end. Neither executes a
// second copy of the decompiled algorithm as its expected result. Reflection
// is performed by the unchanged binary caller, before the ABI comparisons.
const loopIntCaptureSnapshotSource = `
interface SnapshotView { int read(); }
class SnapshotOwner {
 static SnapshotView make(int[] words, int seed) {
  int total = seed;
  for (int i = 0; i < words.length; i++) total = (total ^ words[i]) + i;
  final int snapshot = total;
  return new SnapshotView() { public int read() { return snapshot; } };
 }
}
class SnapshotDriver {
 public static void main(String[] args) throws Exception {
  int rows = 0;
  int[] domain = {Integer.MIN_VALUE, -1, 0, 1, Integer.MAX_VALUE};
  java.math.BigInteger mask = java.math.BigInteger.ONE.shiftLeft(32).subtract(java.math.BigInteger.ONE);
  int cases = 1;
  for (int length = 0; length <= 3; length++, cases *= 5) {
   for (int code = 0; code < cases; code++) for (int seed : domain) {
    int[] input = new int[length]; int value = code;
    java.math.BigInteger oracle = java.math.BigInteger.valueOf(seed).and(mask);
    for (int i = 0; i < length; i++) {
     input[i] = domain[value % 5]; value /= 5;
     oracle = oracle.xor(java.math.BigInteger.valueOf(input[i]).and(mask)).add(java.math.BigInteger.valueOf(i)).and(mask);
    }
    SnapshotView view = SnapshotOwner.make(input, seed);
    java.util.Arrays.fill(input, seed);
    if (!view.getClass().isAnonymousClass() || view.getClass().getEnclosingClass() != SnapshotOwner.class || !view.getClass().getDeclaredField("val$snapshot").isSynthetic()) throw new AssertionError("int anonymous identity");
    if (view.read() != oracle.intValue() || view.read() != oracle.intValue()) throw new AssertionError("int snapshot word");
    rows++;
   }
  }
  try { SnapshotOwner.make(null, 0); throw new AssertionError("null array"); }
  catch (NullPointerException expected) { rows++; }
  System.out.println(rows + ":int:snapshot:signed:overflow:mutation");
 }
}
`

const loopWideCaptureSnapshotSource = `
interface SnapshotView { long read(); }
class SnapshotOwner {
 static SnapshotView make(long[] words, long seed) {
  long total = seed;
  for (int i = 0; i < words.length; i++) total = (total ^ words[i]) + i;
  final long snapshot = total;
  return new SnapshotView() { public long read() { return snapshot; } };
 }
}
class SnapshotDriver {
 public static void main(String[] args) throws Exception {
  int rows = 0;
  long[] domain = {Long.MIN_VALUE, -1, 0, 1, Long.MAX_VALUE};
  java.math.BigInteger mask = java.math.BigInteger.ONE.shiftLeft(64).subtract(java.math.BigInteger.ONE);
  int cases = 1;
  for (int length = 0; length <= 3; length++, cases *= 5) {
   for (int code = 0; code < cases; code++) for (long seed : domain) {
    long[] input = new long[length];
    int value = code;
    java.math.BigInteger oracle = java.math.BigInteger.valueOf(seed).and(mask);
    for (int i = 0; i < length; i++) {
     input[i] = domain[value % 5]; value /= 5;
     oracle = oracle.xor(java.math.BigInteger.valueOf(input[i]).and(mask)).add(java.math.BigInteger.valueOf(i)).and(mask);
    }
    SnapshotView view = SnapshotOwner.make(input, seed);
    java.util.Arrays.fill(input, seed);
    if (!view.getClass().isAnonymousClass() || view.getClass().getEnclosingClass() != SnapshotOwner.class || !view.getClass().getDeclaredField("val$snapshot").isSynthetic()) throw new AssertionError("wide anonymous identity");
    if (view.read() != oracle.longValue() || view.read() != oracle.longValue()) throw new AssertionError("wide snapshot word");
    rows++;
   }
  }
  try { SnapshotOwner.make(null, 0); throw new AssertionError("null array"); }
  catch (NullPointerException expected) { rows++; }
  System.out.println(rows + ":wide:snapshot:signed:overflow:mutation");
 }
}
`

const loopReferenceCaptureSnapshotSource = `
interface SnapshotView { Object read(); }
class SnapshotOwner {
 static SnapshotView make(Object[] input, Object fallback) {
  Object selected = fallback;
  for (int i = 0; i < input.length; i++) if (input[i] != null) selected = input[i];
  final Object snapshot = selected;
  return new SnapshotView() { public Object read() { return snapshot; } };
 }
}
class SnapshotDriver {
 public static void main(String[] args) throws Exception {
  Object first = new String("equal"), second = new String("equal"), third = new Object();
  Object[] domain = {null, first, second, third};
  Object[] fallbacks = {null, first, third};
  int rows = 0, cases = 1;
  for (int length = 0; length <= 4; length++, cases *= 4) {
   for (int code = 0; code < cases; code++) for (Object fallback : fallbacks) {
    Object[] input = new Object[length]; int value = code;
    for (int i = 0; i < length; i++) { input[i] = domain[value % 4]; value /= 4; }
    Object expected = fallback;
    for (int i = length - 1; i >= 0; i--) if (input[i] != null) { expected = input[i]; break; }
    SnapshotView view = SnapshotOwner.make(input, fallback);
    java.util.Arrays.fill(input, fallback);
    if (!view.getClass().isAnonymousClass() || view.getClass().getEnclosingClass() != SnapshotOwner.class || !view.getClass().getDeclaredField("val$snapshot").isSynthetic()) throw new AssertionError("reference anonymous identity");
    if (view.read() != expected || view.read() != expected) throw new AssertionError("reference identity and captured value");
    rows++;
   }
  }
  try { SnapshotOwner.make(null, null); throw new AssertionError("null array"); }
  catch (NullPointerException expected) { rows++; }
  System.out.println(rows + ":reference:snapshot:null:identity:mutation");
 }
}
`

func TestAdversarialLoopSelectedCapturePreservesAnonymousIdentity(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"SnapshotOwner", "RenamedSnapshot"} {
			for _, fixture := range []struct{ name, source, want string }{
				{"selected-index", loopSelectedCaptureSnapshotSource, "782:loop:selection:snapshot:mutation:ties:overflow\n"},
				{"int-word", loopIntCaptureSnapshotSource, "781:int:snapshot:signed:overflow:mutation\n"},
				{"wide-word", loopWideCaptureSnapshotSource, "781:wide:snapshot:signed:overflow:mutation\n"},
				{"reference", loopReferenceCaptureSnapshotSource, "1024:reference:snapshot:null:identity:mutation\n"},
			} {
				t.Run(compiler.name+"/"+owner+"/"+fixture.name, func(t *testing.T) {
					source := strings.ReplaceAll(fixture.source, "SnapshotOwner", owner)
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return nativePrivateEnumCompile(t, source, owner, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, "SnapshotDriver", fixture.want, nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
