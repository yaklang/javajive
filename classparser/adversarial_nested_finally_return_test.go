package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialNestedFinallyKeepsNormalReturnFrontier(t *testing.T) {
	const source = `class ReturnChecked extends Exception {}
class ReturnWrapped extends Exception {ReturnWrapped(Throwable cause){super(cause);}}
class ReturnEffects {
 static String trace;static int mode;static final RuntimeException first=new IllegalArgumentException("first"),cleanup=new IllegalStateException("cleanup");
 static final java.io.IOException io=new java.io.IOException("io");static final ReturnChecked other=new ReturnChecked();static final Error fatal=new AssertionError("fatal");
 static void body(){trace+="B";if(mode==6)throw fatal;if(mode!=0)throw first;}
 static void handle(Throwable original)throws java.io.IOException,ReturnChecked{trace+="H";if(original!=first)throw new AssertionError("original identity");if(mode==2)throw io;if(mode==3)throw other;if(mode==4)throw first;}
 static void cleanup(){trace+="F";if(mode==5)throw cleanup;}
}
public class ReturnOwner {
 public boolean empty,cached;
 public int run()throws ReturnWrapped{
  if(empty)return 6;
  try{ReturnEffects.body();}catch(Exception original){try{ReturnEffects.handle(original);}catch(java.io.IOException|ReturnChecked failure){throw new ReturnWrapped(failure);}}
  finally{if(!cached)ReturnEffects.cleanup();}
  return 6;
 }
}
class ReturnDriver {public static void main(String[]args){int rows=0;for(boolean empty:new boolean[]{false,true})for(boolean cached:new boolean[]{false,true})for(int mode=0;mode<7;mode++){
 ReturnOwner owner=new ReturnOwner();owner.empty=empty;owner.cached=cached;ReturnEffects.mode=mode;ReturnEffects.trace="";Throwable failure=null;int result=-1;
 try{result=owner.run();}catch(Throwable caught){failure=caught;}
 String trace=empty?"":"B"+((mode==0||mode==6)?"":"H")+(cached?"":"F");if(!trace.equals(ReturnEffects.trace))throw new AssertionError("cleanup order");
 if(empty||mode==0||mode==1||mode==5&&cached){if(failure!=null||result!=6)throw new AssertionError("normal result",failure);}
 else if(mode==2||mode==3){if(!(failure instanceof ReturnWrapped)||failure.getCause()!=(mode==2?ReturnEffects.io:ReturnEffects.other))throw new AssertionError("checked wrap");}
 else if(failure!=(mode==4?ReturnEffects.first:mode==5?ReturnEffects.cleanup:ReturnEffects.fatal))throw new AssertionError("failure priority",failure);rows++;
 }System.out.println(rows+":finally:normal:return:identity");}}
`
	for _, shape := range []struct{ name, kind, literal, initial, mismatch string }{
		{"integer", "int", "6", "-1", "result!=6"},
		{"wide integer", "long", "Long.MIN_VALUE", "1", "result!=Long.MIN_VALUE"},
		{"negative zero", "double", "-0.0", "1", "Double.doubleToRawLongBits(result)!=Long.MIN_VALUE"},
		{"null", "String", "null", `"initial"`, "result!=null"},
		{"UTF16 string", "String", `"a\ud800"`, "null", `!"a\ud800".equals(result)`},
	} {
		t.Run(shape.name, func(t *testing.T) {
			fixture := strings.ReplaceAll(source, "return 6;", "return "+shape.literal+";")
			fixture = strings.Replace(fixture, "public int run()", "public "+shape.kind+" run()", 1)
			fixture = strings.Replace(fixture, "int result=-1;", shape.kind+" result="+shape.initial+";", 1)
			fixture = strings.Replace(fixture, "result!=6", shape.mismatch, 1)
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "ReturnOwner", "ReturnDriver", "28:finally:normal:return:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{"ReturnOwner.java": fixture}, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
