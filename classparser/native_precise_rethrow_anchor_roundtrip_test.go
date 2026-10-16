package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativePreciseAnchorFixture = `
class AnchorEvents {
 static final java.io.IOException bodyChecked=new java.io.IOException("body"),closeChecked=new java.io.IOException("close");
 static final RuntimeException bodyRuntime=new IllegalStateException("body"),closeRuntime=new IllegalStateException("close");
 static final Error bodyError=new AssertionError("body"),closeError=new AssertionError("close");
 static Object created;static Throwable recorded;static String trace="";
 static int body(int seed,int mode)throws java.io.IOException{trace+="B";if(mode==1)throw bodyChecked;if(mode==2)throw bodyRuntime;if(mode==3)throw bodyError;return seed*31+17;}
 static void close(int mode)throws java.io.IOException{trace+="C";if(mode==1)throw closeChecked;if(mode==2)throw closeRuntime;if(mode==3)throw closeError;}
 static Throwable fault(int body,int close){return close==1?closeChecked:close==2?closeRuntime:close==3?closeError:body==1?bodyChecked:body==2?bodyRuntime:body==3?bodyError:null;}
}
public class AnchorOwner {
 public class Entry {public final int value;
  public Entry(int seed,int body,int close)throws java.io.IOException{
   try{AnchorEvents.created=this;value=AnchorEvents.body(seed,body);}finally{AnchorEvents.close(close);}
  }
 }
}
class AnchorDriver {public static void main(String[]args)throws Exception{int rows=0;
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int body=0;body<4;body++)for(int close=0;close<4;close++){
  AnchorEvents.created=null;AnchorEvents.recorded=null;AnchorEvents.trace="";AnchorOwner outer=new AnchorOwner();Object result;
  try{result=outer.new Entry(seed,body,close);}catch(Throwable failure){result=failure;}
  Throwable wanted=AnchorEvents.fault(body,close);Object made=AnchorEvents.created;
  if(made==null||made.getClass()!=AnchorOwner.Entry.class)throw new AssertionError("partial publication");
  int value=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(17)).intValue();
  if(((AnchorOwner.Entry)made).value!=(body==0?value:0))throw new AssertionError("assignment/overflow");
  if(wanted==null?result!=made:result!=wanted)throw new AssertionError("cleanup overrides/identity");
  if(!AnchorEvents.trace.equals("BC"))throw new AssertionError("evaluation/cleanup order");

  java.lang.reflect.Field enclosing=AnchorOwner.Entry.class.getDeclaredField("this$0");enclosing.setAccessible(true);if(enclosing.get(made)!=outer)throw new AssertionError("enclosing capture");
  rows++;
 }System.out.println(rows+":precise:anchor:identity:cleanup:publication");}}
`

func TestAdversarialPreciseRethrowSourceAnchor(t *testing.T) {
	for _, shape := range []string{"instance", "static"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []bool{false, true} {
				name := "original"
				source := nativePreciseAnchorFixture
				owner, driver := "AnchorOwner", "AnchorDriver"
				if shape == "static" {
					source = strings.Replace(source, "public class Entry", "public static class Entry", 1)
					source = strings.Replace(source, "outer.new Entry(seed,body,close)", "new AnchorOwner.Entry(seed,body,close)", 1)
					source = strings.Replace(source, `java.lang.reflect.Field enclosing=AnchorOwner.Entry.class.getDeclaredField("this$0");enclosing.setAccessible(true);if(enclosing.get(made)!=outer)throw new AssertionError("enclosing capture");`, `if(AnchorOwner.Entry.class.getDeclaringClass()!=AnchorOwner.class)throw new AssertionError("static declaration owner");`, 1)
				}
				if rename {
					name = "renamed"
					source = strings.NewReplacer("AnchorOwner", "OtherOwner", "AnchorDriver", "OtherDriver", "AnchorEvents", "OtherEvents").Replace(source)
					owner, driver = "OtherOwner", "OtherDriver"
				}
				t.Run(name, func(t *testing.T) {
					for _, native := range []bool{false, true} {
						profile := "modern"
						if native {
							profile = "native8"
						}
						t.Run(profile, func(t *testing.T) {
							want := "80:precise:anchor:identity:cleanup:publication\n"
							if native {
								testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
									return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
								}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, driver, want, nil, nativeLexicalExactSignatures)
							} else {
								testNativePrivateSetterCompiledFixture(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
									return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
								}, nativeLexicalExactSignatures)
							}
						})
					}
				})
			}
		})
	}
}
