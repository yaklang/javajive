package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The middle accessed symbol belongs to an anonymous scope. A named-only
// inventory cannot establish absence; only the completed forest can do so.
const accessorForestRegistrationFixture = `class RegistrationOwner {
 private int first=7,last=13;
 java.util.function.IntSupplier make(final int seed){return new java.util.function.IntSupplier(){
  private final int hidden=seed+first;
  public int getAsInt(){return new java.util.function.IntSupplier(){public int getAsInt(){return hidden;}}.getAsInt();}
 };}
 class Tail {int get(){return last;}} Tail tail(){return new Tail();}
}
class RegistrationDriver {public static void main(String[]args){RegistrationOwner c=new RegistrationOwner();
 for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  if(c.make(x).getAsInt()!=x+7||c.tail().get()!=13)throw new AssertionError("shared anonymous registration");
 }System.out.println("registration:closure:5");}}
`

func TestNativeAccessorRegistrationWaitsForAnonymousForest(t *testing.T) {
	for _, shape := range []string{"integer", "wide", "mutable identity"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					owner, source := "RegistrationOwner", accessorForestRegistrationFixture
					if shape == "wide" {
						source = strings.ReplaceAll(source, "IntSupplier", "LongSupplier")
						source = strings.ReplaceAll(source, "getAsInt", "getAsLong")
						source = strings.ReplaceAll(source, "private final int hidden", "private final long hidden")
						source = strings.ReplaceAll(source, "int seed", "long seed")
						source = strings.ReplaceAll(source, "public int getAsLong", "public long getAsLong")
						source = strings.ReplaceAll(source, "int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}", "long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}")
					} else if shape == "mutable identity" {
						source = `abstract class MutableReader {abstract Object read();abstract void set(Object value);}
class RegistrationOwner {private Object first=new Object();private int last=13;
 MutableReader make(final Object seed){return new MutableReader(){private Object hidden=seed==null?first:seed;
  void set(Object value){hidden=value;}
  Object read(){return new java.util.function.Supplier<Object>(){public Object get(){return hidden;}}.get();}
 };}
 class Tail{int get(){return last;}}Tail tail(){return new Tail();}}
class RegistrationDriver {public static void main(String[]args){RegistrationOwner c=new RegistrationOwner();
 for(Object x:new Object[]{null,new Object(),new String("same"),new String("same"),new Object()}){
  MutableReader r=c.make(x);if(x!=null&&r.read()!=x)throw new AssertionError("initial identity");
  Object next=new Object();r.set(next);if(r.read()!=next)throw new AssertionError("delayed mutable identity");
  r.set(null);if(r.read()!=null||c.tail().get()!=13)throw new AssertionError("null/shared registration");
 }System.out.println("registration:closure:5");}}
`
					}
					if rename == "renamed" {
						owner = "DifferentLexicalOwner"
						source = strings.ReplaceAll(source, "RegistrationOwner", owner)
						source = strings.ReplaceAll(source, "hidden", "anotherPrivateSymbol")
					}
					t.Run("modern", func(t *testing.T) {
						testNativePrivateSetterFixture(t, source, owner, "RegistrationDriver", "registration:closure:5\n")
					})
					t.Run("native8", func(t *testing.T) {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, "RegistrationDriver", "registration:closure:5\n", nil, nativeLexicalExactSignatures)
					})
				})
			}
		})
	}
}
