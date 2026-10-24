package javaclassparser

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// A classfile's ordinary method-table order need not be its source order.
// Reordering that table is JVM-valid and changes no instruction, field or
// signature, but a recompiling javac still needs the original bridge schedule.
const methodRegistrationScheduleFixture = `
interface MethodScheduleView {int read();}
class MethodScheduleEffects {static String trace="";static Object mark(String value){trace+=value;return new Object();}}
class MethodScheduleOwner {
 static final Object FIRST=MethodScheduleEffects.mark("A");
 private int value;
 static final Object SECOND=MethodScheduleEffects.mark("B");
 MethodScheduleOwner(int n){value=n;}
 static class Token {final int value;private Token(int n){value=n;}}
 static Token allocate(int n){return new Token(n);}
 MethodScheduleView make(){return new MethodScheduleView(){public int read(){return value;}};}
}
class MethodScheduleDriver {
 public static void main(String[]args){
  int rows=0;
  for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   MethodScheduleOwner owner=new MethodScheduleOwner(n),other=new MethodScheduleOwner(~n);
   MethodScheduleOwner.Token token=MethodScheduleOwner.allocate(n);
   MethodScheduleView view=owner.make(),another=other.make();
   if(view.read()!=n||another.read()!=~n||token.value!=n||!MethodScheduleEffects.trace.equals("AB"))throw new AssertionError("constructor bridge/receiver/initializer order");
   if(!view.getClass().isAnonymousClass()||view.getClass().getEnclosingClass()!=MethodScheduleOwner.class||!view.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("anonymous registration identity");
   rows++;
  }
  System.out.println(rows+":method-table:private-constructor:accessors:initializer-order");
 }
}`

func TestAdversarialOrdinaryMethodTableOrderKeepsOriginalRegistration(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, simple := range []string{"MethodScheduleOwner", "OtherMethodScheduleOwner"} {
			t.Run(compiler.name+"/"+simple, func(t *testing.T) {
				source := strings.ReplaceAll(methodRegistrationScheduleFixture, "MethodScheduleOwner", simple)
				compile := func(debug string) map[string][]byte {
					var files map[string][]byte
					if compiler.profile == NativeJavac8 {
						files = localGetterCompileJavac8(t, map[string]string{simple + ".java": source}, debug)
					} else {
						files = nativeCompileDebugClasses(t, source, debug)
					}
					object, err := Parse(files[simple+".class"])
					if err != nil {
						t.Fatal(err)
					}
					var moved *MemberInfo
					var reordered []*MemberInfo
					original := map[*MemberInfo][]byte{}
					for _, method := range object.Methods {
						original[method] = bytes.Clone(methodCodeBytes(method))
						name, _ := sourceBridgeUTF8(object, method.NameIndex)
						if name == "allocate" {
							moved = method
						} else {
							reordered = append(reordered, method)
						}
					}
					if moved == nil {
						t.Fatal("authored ordinary declaration missing")
					}
					object.Methods = append(reordered, moved)
					for method, code := range original {
						if !bytes.Equal(code, methodCodeBytes(method)) {
							t.Fatal("method-table perturbation changed original instructions")
						}
					}
					files[simple+".class"] = object.Bytes()
					return files
				}
				testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{simple}, "MethodScheduleDriver", "5:method-table:private-constructor:accessors:initializer-order\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
			})
		}
	}
}
