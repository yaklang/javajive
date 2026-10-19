package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The inner cleanup runs while its monitor is held; the enclosing finally and
// normal continuation run after that monitor is released. Checking values alone
// misses this distinction. The original caller also checks exception identity,
// cleanup count and overriding failures for every subset of six fault points.
const monitorCleanupBoundaryFixture = `
class BoundaryEffects{static String trace="";static final RuntimeException[] errors=new RuntimeException[6];static{for(int i=0;i<errors.length;i++)errors[i]=new IllegalStateException("fault-"+i);}}
class BoundaryOwner{
 final Object outer=new Object(),inner=new Object();
 int step(int stage,int value,int faults){int mask=(Thread.holdsLock(outer)?1:0)|(Thread.holdsLock(inner)?2:0)|(Thread.holdsLock(this)?4:0);int expected=stage==1?7:stage<3||stage==5?3:1;if(mask!=expected)throw new AssertionError("monitor domain stage="+stage+" mask="+mask+" expected="+expected);BoundaryEffects.trace+=stage;if((faults&(1<<stage))!=0)throw BoundaryEffects.errors[stage];return stage==0?value*31+3:stage==1?value^0x5a5a5a5a:stage==4?Integer.rotateLeft(value,5):value;}
 int run(int input,int faults,boolean logging){synchronized(outer){int answer;try{synchronized(inner){boolean success=false;try{answer=step(0,input,faults);synchronized(this){answer=step(1,answer,faults);}success=true;}finally{if(!success&&logging)step(5,0,faults);assert Thread.holdsLock(inner);step(2,success?1:0,faults);}}}finally{step(3,0,faults);}return step(4,answer,faults);}}
}
class BoundaryDriver{
 public static void main(String[]args){int rows=0;for(int input:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int faults=0;faults<64;faults++)for(boolean logging:new boolean[]{false,true}){
  String trace="0";int error=(faults&1)!=0?0:-1;if(error<0){trace+="1";if((faults&2)!=0)error=1;}
  if(error>=0&&logging){trace+="5";if((faults&32)!=0)error=5;}
  if(!(error==5&&logging)){trace+="2";if((faults&4)!=0)error=2;}
  trace+="3";if((faults&8)!=0)error=3;if(error<0){trace+="4";if((faults&16)!=0)error=4;}
  long first=((long)input*31L+3L)&0xffffffffL,word=first^0x5a5a5a5aL;int expected=(int)(((word<<5)|(word>>>27))&0xffffffffL);
  BoundaryOwner owner=new BoundaryOwner();BoundaryEffects.trace="";int actual=0;Throwable thrown=null;try{actual=owner.run(input,faults,logging);}catch(Throwable e){thrown=e;}
  if(thrown!=(error<0?null:BoundaryEffects.errors[error])||!BoundaryEffects.trace.equals(trace)||error<0&&actual!=expected||Thread.holdsLock(owner.outer)||Thread.holdsLock(owner.inner)||Thread.holdsLock(owner))throw new AssertionError("cleanup/continuation input="+input+" faults="+faults+" logging="+logging+" trace="+BoundaryEffects.trace+" expected="+trace,thrown);
  rows++;
 }if(rows!=640)throw new AssertionError("world census");System.out.println(rows+":monitor-domains:nested-finally:failure-overrides:values");}
}`

func TestAdversarialMonitorCleanupKeepsReleasedFinallyAndContinuation(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"BoundaryOwner", "OtherReleaseOwner"} {
			for _, word := range []string{"int", "long"} {
				t.Run(compiler.name+"/"+owner+"/"+word, func(t *testing.T) {
					source := strings.ReplaceAll(monitorCleanupBoundaryFixture, "BoundaryOwner", owner)
					if word == "long" {
						source = strings.NewReplacer(
							"int step(int stage,int value,int faults)", "long step(int stage,long value,int faults)",
							"int run(int input,int faults,boolean logging)", "long run(long input,int faults,boolean logging)",
							"int answer;", "long answer;",
							"Integer.rotateLeft(value,5)", "Long.rotateLeft(value,5)",
							"int input:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}", "long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}",
							"int actual=0;", "long actual=0;",
							"long first=((long)input*31L+3L)&0xffffffffL,word=first^0x5a5a5a5aL;int expected=(int)(((word<<5)|(word>>>27))&0xffffffffL);",
							`java.math.BigInteger mask=java.math.BigInteger.ONE.shiftLeft(64).subtract(java.math.BigInteger.ONE),wordValue=java.math.BigInteger.valueOf(input).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(3)).and(mask).xor(java.math.BigInteger.valueOf(0x5a5a5a5aL));long expected=wordValue.shiftLeft(5).or(wordValue.shiftRight(59)).and(mask).longValue();`,
						).Replace(source)
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, compiler.profile, compiler.javac, []string{owner}, "BoundaryDriver", "640:monitor-domains:nested-finally:failure-overrides:values\n", nil, nil, []string{"normal"}, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
