package javaclassparser

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const sharedLoopExitFixture = `class SharedExitCursor{final int[]keys;int pos;int doc=-1;int consumed;SharedExitCursor(int[]input){keys=input;}int next(){for(;;){if(pos==keys.length){doc=Integer.MAX_VALUE;break;}int key=keys[pos];if(key!=doc){ASSERTdoc=key;break;}if(++pos==keys.length){consumed++;}else{consumed++;}}return doc;}}
class SharedExitDriver{public static void main(String[]args){int rows=0;for(int[]input:new int[][]{new int[]{},new int[]{0},new int[]{1,1,2},new int[]{0,0,1,1,1,31,Integer.MAX_VALUE-1}}){SharedExitCursor cursor=new SharedExitCursor(input);java.util.LinkedHashSet<Integer> wanted=new java.util.LinkedHashSet<Integer>();for(int x:input)wanted.add(x);int prior=0;for(int doc:wanted){if(cursor.next()!=doc||cursor.doc!=doc||cursor.keys!=input||cursor.consumed!=prior)throw new AssertionError("shared exit doc/effects");while(prior<input.length&&input[prior]==doc)prior++;rows++;}if(cursor.next()!=Integer.MAX_VALUE||cursor.doc!=Integer.MAX_VALUE||cursor.consumed!=input.length||cursor.next()!=Integer.MAX_VALUE)throw new AssertionError("terminal exit");rows++;}System.out.println(rows+":shared:exit:state:effects");}}`

func sharedLoopExitShape(shape string) (string, string) {
	f, owner := sharedLoopExitFixture, "SharedExitCursor"
	assertion := ""
	if shape == "assertion" {
		assertion = `assert key>doc:"doc="+doc+" key="+key;`
	}
	f = strings.Replace(f, "ASSERT", assertion, 1)
	switch shape {
	case "renamed":
		owner = "ExitRegionCursor"
		f = strings.ReplaceAll(f, "SharedExitCursor", owner)
		f = strings.ReplaceAll(f, "consumed", "originalStepEffects")
	case "wide":
		f = strings.Replace(f, "int next()", "long next()", 1)
		f = strings.Replace(f, "return doc;", "return (long)doc;", 1)
	case "reference":
		f = strings.Replace(f, "int next()", "Object next()", 1)
		f = strings.Replace(f, "return doc;", "return keys;", 1)
		f = strings.ReplaceAll(f, "cursor.next()!=doc", "cursor.next()!=input")
		f = strings.ReplaceAll(f, "cursor.next()!=Integer.MAX_VALUE", "cursor.next()!=input")
	case "nested":
		f = strings.Replace(f, "int next(){for(;;)", "int next(){for(int outer=0;outer<1;outer++){for(;;)", 1)
		f = strings.Replace(f, "}}return doc;", "}}}return doc;", 1)
	}
	return f, owner
}

// Original-first differential execution also independently checks each cursor
// result, shared-return identity, and consumption at every successful exit.
func TestAdversarialLoopSharedExitPreservesSelectedEffects(t *testing.T) {
	for _, shape := range []string{"plain", "assertion", "wide", "reference", "renamed", "nested"} {
		t.Run(shape, func(t *testing.T) {
			f, owner := sharedLoopExitShape(shape)
			testNativeIndependentFamilyFixture(t, f, []string{owner}, "SharedExitDriver", "11:shared:exit:state:effects\n")
		})
	}
}
func TestAdversarialLoopSharedExitOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, e := exec.Command(javac, "-version").CombinedOutput(); e != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", e, string(out))
	}
	for _, shape := range []string{"plain", "assertion", "wide", "reference", "renamed", "nested"} {
		t.Run(shape, func(t *testing.T) {
			f, owner := sharedLoopExitShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, f, owner, debug) }, NativeJavac8, javac, []string{owner}, "SharedExitDriver", "11:shared:exit:state:effects\n", nil)
		})
	}
}
