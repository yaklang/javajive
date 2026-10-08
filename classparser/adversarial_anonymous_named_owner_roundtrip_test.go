package javaclassparser

import (
	"strings"
	"testing"
)

// This is the exact source formerly refused by the archive ownership table.
// Keep the JVM driver separate: it observes the otherwise discarded result of
// Nested.read and checks the physical enclosing word without decompiling it.
const anonymousNamedOwnerFixture = `class NativeArchiveOwner {static Runnable make(){return new Runnable(){class Nested{int read(){return 1;}}public void run(){new Nested().read();}};}}`

const anonymousNamedOwnerDriver = `class AnonymousNamedOwnerDriver {
 public static void main(String[] args) throws Exception {
  int rows=0;
  for(int i=0;i<4;i++){
   Runnable task=NativeArchiveOwner.make();task.run();Class<?> anonymous=task.getClass();
   if(!anonymous.isAnonymousClass()||anonymous.getDeclaringClass()!=null||anonymous.getEnclosingClass()!=NativeArchiveOwner.class||!anonymous.getEnclosingMethod().getName().equals("make"))throw new AssertionError("anonymous source scope");
   Class<?>[] members=anonymous.getDeclaredClasses();
   if(members.length!=1)throw new AssertionError("missing member declaration");
   Class<?> nested=members[0];
   if(!nested.isMemberClass()||nested.isAnonymousClass()||nested.getDeclaringClass()!=anonymous||nested.getEnclosingClass()!=anonymous||nested.getEnclosingMethod()!=null)throw new AssertionError("named lexical owner");
   java.lang.reflect.Constructor<?> ctor=nested.getDeclaredConstructor(anonymous);ctor.setAccessible(true);
   java.lang.reflect.Method read=nested.getDeclaredMethod("read");read.setAccessible(true);
   for(Object enclosing:new Object[]{task,null}){
    Object value=ctor.newInstance(enclosing);
    if(((Integer)read.invoke(value)).intValue()!=1)throw new AssertionError("discarded algorithm result");
    int enclosingFields=0;
    for(java.lang.reflect.Field field:nested.getDeclaredFields())if(field.isSynthetic()&&!java.lang.reflect.Modifier.isStatic(field.getModifiers())&&field.getType()==anonymous){field.setAccessible(true);if(field.get(value)!=enclosing)throw new AssertionError("enclosing object identity");enclosingFields++;}
    if(enclosingFields!=1)throw new AssertionError("physical enclosing word");rows++;
   }
  }
  System.out.println(rows+":anonymous:named:owner:result:identity");
 }
}`

func TestAdversarialAnonymousNamedOwnerPreservesPreviouslyRefusedProgram(t *testing.T) {
	testNativeIndependentFamilyFixture(t, anonymousNamedOwnerFixture+anonymousNamedOwnerDriver, []string{"NativeArchiveOwner"}, "AnonymousNamedOwnerDriver", "8:anonymous:named:owner:result:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialAnonymousNamedOwnerPreservesRenamedDeclarationsAndDifferentResult(t *testing.T) {
	f := strings.NewReplacer("NativeArchiveOwner", "MethodScopeRoot", "Nested", "OwnedItem", "return 1;", "return -17;", "intValue()!=1", "intValue()!=-17").Replace(anonymousNamedOwnerFixture + anonymousNamedOwnerDriver)
	testNativeIndependentFamilyFixture(t, f, []string{"MethodScopeRoot"}, "AnonymousNamedOwnerDriver", "8:anonymous:named:owner:result:identity\n", nativeLexicalExactSignatures)
}
