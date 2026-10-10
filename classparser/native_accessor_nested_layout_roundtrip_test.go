package javaclassparser

import (
	"strings"
	"testing"
)

// The original compiler first encounters left in Start, then right in
// Scope.Reader, then last in Scope. Appending Reader after Scope's own methods
// cannot reproduce these shared private-symbol ordinals by moving Scope alone.
const nativeAccessorNestedLayoutFixture = `class NestedLayoutEffects{static String trace="";static Object mark(String label){trace+=label;return new Object();}}
class NestedLayoutOwner{static Object first=NestedLayoutEffects.mark("L");private Object left,right,last;NestedLayoutOwner(Object left,Object right,Object last){this.left=left;this.right=right;this.last=last;}class Start{Object read(){return NestedLayoutOwner.this.left;}}class Scope{Object first=NestedLayoutEffects.mark("F");class Reader{Object read(){return NestedLayoutOwner.this.right;}}Object read(){return NestedLayoutOwner.this.last;}Object last=NestedLayoutEffects.mark("S");Reader reader(){return new Reader();}}static Object second=NestedLayoutEffects.mark("R");}
class NestedLayoutDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object left:new Object[]{null,new Object()})for(Object right:new Object[]{null,left,new Object()})for(Object last:new Object[]{null,right}){NestedLayoutOwner owner=new NestedLayoutOwner(left,right,last);if(!NestedLayoutEffects.trace.startsWith("LR"))throw new AssertionError("root initialization order");NestedLayoutEffects.trace="LR";NestedLayoutOwner.Start start=owner.new Start();NestedLayoutOwner.Scope scope=owner.new Scope();NestedLayoutOwner.Scope.Reader reader=scope.reader();if(start.read()!=left||reader.read()!=right||scope.read()!=last||!NestedLayoutEffects.trace.equals("LRFS")||scope.first==scope.last)throw new AssertionError("nested registration/identity/initialization order");if(reader.getClass().getDeclaringClass()!=NestedLayoutOwner.Scope.class||scope.getClass().getDeclaringClass()!=NestedLayoutOwner.class)throw new AssertionError("original lexical ownership");rows++;}System.out.println(rows+":nested:ordinals:identity:LRFS");}}`

func TestNativeAccessorNestedDeclarationLayoutRoundTrip(t *testing.T) {
	testNativeNestedLayoutFixture(t, nativeAccessorNestedLayoutFixture, "NestedLayoutOwner")
}

func TestNativeAccessorNestedDeclarationLayoutRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAccessorNestedLayoutFixture, "NestedLayoutOwner", "OtherLexicalLayout")
	f = strings.ReplaceAll(f, "Reader", "Cursor")
	f = strings.ReplaceAll(f, "Scope", "Container")
	testNativeNestedLayoutFixture(t, f, "OtherLexicalLayout")
}

func testNativeNestedLayoutFixture(t *testing.T, fixture, owner string) {
	t.Helper()
	testNativePrivateSetterCompiledFixture(t, owner, "NestedLayoutDriver", "12:nested:ordinals:identity:LRFS\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, nativeLexicalExactSignatures)
}

func TestNativeAccessorNestedLayoutIdenticalMemberSpellingRoundTrip(t *testing.T) {
	const scope = `class Scope{Object first=NestedLayoutEffects.mark("F");class Reader{Object read(){return NestedLayoutOwner.this.right;}}Object read(){return NestedLayoutOwner.this.last;}Object last=NestedLayoutEffects.mark("S");Reader reader(){return new Reader();}}`
	f := strings.Replace(nativeAccessorNestedLayoutFixture, scope, scope+strings.Replace(scope, "Scope", "OtherScope", 1), 1)
	f = strings.Replace(f, "rows++;", `NestedLayoutOwner.OtherScope other=owner.new OtherScope();NestedLayoutOwner.OtherScope.Reader second=other.reader();if(second.read()!=right||other.read()!=last||second.getClass().getDeclaringClass()!=NestedLayoutOwner.OtherScope.class||!NestedLayoutEffects.trace.equals("LRFSFS"))throw new AssertionError("same spelling in distinct owners");rows++;`, 1)
	testNativeNestedLayoutFixture(t, f, "NestedLayoutOwner")
}

func TestNativeAccessorNestedLayoutThreeOwnedLevelsRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAccessorNestedLayoutFixture, `class Reader{Object read(){return NestedLayoutOwner.this.right;}}`, `class Reader{class Leaf{Object read(){return NestedLayoutOwner.this.right;}}Object read(){return NestedLayoutOwner.this.last;}Leaf leaf(){return new Leaf();}}`, 1)
	f = strings.Replace(f, "reader.read()!=right", "reader.leaf().read()!=right||reader.read()!=last", 1)
	f = strings.Replace(f, "rows++;", `if(reader.leaf().getClass().getDeclaringClass()!=NestedLayoutOwner.Scope.Reader.class)throw new AssertionError("three original lexical levels");rows++;`, 1)
	testNativeNestedLayoutFixture(t, f, "NestedLayoutOwner")
}
