package javaclassparser

import "testing"

// A source-expressible post-SUPER field initializer remains an initialization
// event. Its old ownership-only rejection is not a behavioral oracle: rebuild
// the actual platform-interface implementation and retain the independent JVM
// driver, original capture fields, field modifiers and enclosing metadata.
func TestAdversarialAnonymousPlatformInitializerPreservesOriginalFieldAndException(t *testing.T) {
	const fixture = `
class NativeArchiveOwner {
 static Runnable make(final Object x){return new Runnable(){final Object y=x;public void run(){if(y==null)throw new IllegalArgumentException();}};}
}
class AnonymousPlatformInitDriver {
 public static void main(String[]args)throws Exception {
  Object token=new Object();int rows=0;
  for(Object value:new Object[]{null,token,"original",Integer.valueOf(31)}) {
   Runnable r=NativeArchiveOwner.make(value);Class<?> type=r.getClass();
   if(!type.isAnonymousClass()||type.getEnclosingClass()!=NativeArchiveOwner.class||type.getDeclaringClass()!=null||!type.getEnclosingMethod().getName().equals("make"))throw new AssertionError("original anonymous declaration identity");
   java.lang.reflect.Field y=type.getDeclaredField("y");y.setAccessible(true);
   if(y.getType()!=Object.class||y.get(r)!=value||y.getModifiers()!=java.lang.reflect.Modifier.FINAL)throw new AssertionError("original initialized field identity/modifiers");
   try{r.run();if(value==null)throw new AssertionError("missing original failure");}
   catch(IllegalArgumentException failure){if(value!=null||failure.getClass()!=IllegalArgumentException.class||failure.getMessage()!=null)throw new AssertionError("original exception",failure);}
   rows++;
  }
  System.out.println(rows+":platform:anonymous:initializer:identity:exception");
 }
}
`
	testNativeIndependentFamilyFixture(t, fixture, []string{"NativeArchiveOwner"}, "AnonymousPlatformInitDriver", "4:platform:anonymous:initializer:identity:exception\n", nativeLexicalExactSignatures)
}
