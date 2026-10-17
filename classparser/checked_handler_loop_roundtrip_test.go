package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativeCheckedHandlerLoopFixture = `
class LoopCatchOps {
 static int mode;static String trace="";
 static final java.io.IOException incoming=new java.io.IOException("incoming");
 static final RuntimeException runtime=new IllegalArgumentException("runtime");
 static final Error fatal=new AssertionError("fatal");
 static void attempt()throws java.io.IOException{trace+="A";throw incoming;}
 static void tick(int i){trace+="T";if(i==1){if(mode==1)throw runtime;if(mode==2)throw fatal;}}
}
class LoopCatchOwner {
 Worker worker(){return new Worker();}
 class Worker {
  int run(int limit){int count=0;try{LoopCatchOps.attempt();return -1;}catch(java.io.IOException caught){BODY return count;}}
 }
}
class LoopCatchDriver {
 public static void main(String[]args){int rows=0;
  for(int limit:new int[]{-3,0,1,2,5})for(int mode=0;mode<3;mode++){
   LoopCatchOps.mode=mode;LoopCatchOps.trace="";Object result;
   try{result=Integer.valueOf(new LoopCatchOwner().worker().run(limit));}catch(Throwable error){result=error;}
   boolean fails=limit>1&&mode!=0;Object expected=fails?(mode==1?LoopCatchOps.runtime:LoopCatchOps.fatal):Integer.valueOf(Math.max(limit,0));
   int ticks=fails?2:Math.max(limit,0);String trace="A";for(int i=0;i<ticks;i++)trace+="T";
   if((expected instanceof Throwable?result!=expected:!expected.equals(result))||!trace.equals(LoopCatchOps.trace))throw new AssertionError("catch loop identity/count/order");rows++;
  }System.out.println(rows+":caught-loop:identity:effects");
 }
}`

func TestAdversarialCheckedHandlerStructuredLoops(t *testing.T) {
	for _, shape := range []string{"while", "do", "for", "switch", "nested"} {
		t.Run(shape, func(t *testing.T) {
			body := "while(count<limit){LoopCatchOps.tick(count);count++;}"
			switch shape {
			case "do":
				body = "if(limit>0){do{LoopCatchOps.tick(count);count++;}while(count<limit);}"
			case "for":
				body = "for(int i=0;i<limit;i++){LoopCatchOps.tick(i);count++;}"
			case "switch":
				body = "switch(limit){case 0:break;case 1:LoopCatchOps.tick(count);count++;break;default:while(count<limit){LoopCatchOps.tick(count);count++;}}"
			case "nested":
				body = "while(count<limit){switch(count&1){case 0:LoopCatchOps.tick(count);break;default:LoopCatchOps.tick(count);}count++;}"
			}
			source := strings.Replace(nativeCheckedHandlerLoopFixture, "BODY", body, 1)
			for _, native := range []bool{false, true} {
				name := "modern"
				if native {
					name = "native8"
				}
				t.Run(name, func(t *testing.T) {
					if native {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return localGetterCompileJavac8(t, map[string]string{"LoopCatchOwner.java": source}, debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"LoopCatchOwner"}, "LoopCatchDriver", "15:caught-loop:identity:effects\n", nil, nativeLexicalExactSignatures)
					} else {
						testNativePrivateSetterCompiledFixture(t, "LoopCatchOwner", "LoopCatchDriver", "15:caught-loop:identity:effects\n", func(t *testing.T, debug string) map[string][]byte {
							return nativeCompileSourceReleaseClasses(t, map[string]string{"LoopCatchOwner.java": source}, debug, "8")
						}, nativeLexicalExactSignatures)
					}
				})
			}
		})
	}
}
