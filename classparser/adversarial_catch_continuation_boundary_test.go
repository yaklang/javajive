package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const catchContinuationBoundaryBody = `long value;try{value=input.read();}catch(java.io.IOException error){abort.run();throw error;}if(value<0){output.write(-value);return -1;}output.write(value);return value;`

const catchContinuationBoundaryFixture = `
class BoundaryEffects {static String trace;static int mode;static long seen;static final java.io.IOException readError=new java.io.IOException("read"),writeError=new java.io.IOException("write"),abortError=new java.io.IOException("abort");}
class BoundaryInput {final long value;BoundaryInput(long n){value=n;}long read()throws java.io.IOException{BoundaryEffects.trace+="R";if(BoundaryEffects.mode==1||BoundaryEffects.mode==3)throw BoundaryEffects.readError;return value;}}
class BoundaryAbort {void run()throws java.io.IOException{BoundaryEffects.trace+="A";if(BoundaryEffects.mode==3)throw BoundaryEffects.abortError;}}
class BoundaryOutput {void write(long n)throws java.io.IOException{BoundaryEffects.trace+="W";BoundaryEffects.seen=n;if(BoundaryEffects.mode==2)throw BoundaryEffects.writeError;}}
interface BoundaryTask {long run()throws java.io.IOException;}
class BoundaryOwner {OWNER_BODY}
class BoundaryDriver {public static void main(String[]args)throws Exception{int rows=0;
 for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int mode=0;mode<4;mode++){
 BoundaryEffects.trace="";BoundaryEffects.mode=mode;BoundaryEffects.seen=17;java.io.IOException caught=null;
 try{long got=CALL_BODY;if(mode!=0||got!=(n<0?-1:n))throw new AssertionError("value/exception");}catch(java.io.IOException error){caught=error;}
 java.io.IOException expected=mode==0?null:mode==1?BoundaryEffects.readError:mode==2?BoundaryEffects.writeError:BoundaryEffects.abortError;
 String trace=mode==1||mode==3?"RA":"RW";long seen=mode==1||mode==3?17:n<0?java.math.BigInteger.valueOf(n).negate().longValue():n;
 if(caught!=expected||!BoundaryEffects.trace.equals(trace)||BoundaryEffects.seen!=seen)throw new AssertionError("handler domain/effect order/identity: "+mode+":"+BoundaryEffects.trace,caught);rows++;
 }System.out.println(rows+":protected-producer:unprotected-continuation:identity");}}
`

func TestAdversarialCatchDoesNotAbsorbThrowingNormalContinuation(t *testing.T) {
	for _, root := range []string{"BoundaryOwner", "RenamedProducerScope"} {
		for _, anonymous := range []bool{false, true} {
			kind := "ordinary"
			declaration := "static long process(BoundaryInput input,BoundaryAbort abort,BoundaryOutput output)throws java.io.IOException{" + catchContinuationBoundaryBody + "}"
			call := root + ".process(new BoundaryInput(n),new BoundaryAbort(),new BoundaryOutput())"
			if anonymous {
				kind = "anonymous"
				declaration = "BoundaryTask make(final BoundaryInput input,final BoundaryAbort abort,final BoundaryOutput output){return new BoundaryTask(){public long run()throws java.io.IOException{" + catchContinuationBoundaryBody + "}};}"
				call = "new " + root + "().make(new BoundaryInput(n),new BoundaryAbort(),new BoundaryOutput()).run()"
			}
			t.Run(root+"/"+kind, func(t *testing.T) {
				fixture := strings.NewReplacer("OWNER_BODY", declaration, "CALL_BODY", call, "BoundaryOwner", root).Replace(catchContinuationBoundaryFixture)
				testNativePrivateSetterCompiledFixtureWithPolicies(t, root, "BoundaryDriver", "20:protected-producer:unprotected-continuation:identity\n", func(t *testing.T, debug string) map[string][]byte {
					native := os.Getenv("JAVA8_JAVAC")
					if native == "" {
						t.Fatal("original native compiler required")
					}
					return compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, root, debug, false)
				}, nil, []string{"normal"})
			})
		}
	}
}
