package javaclassparser

import (
	"strings"
	"testing"
)

const sequentialResourceFixture = `
import java.util.*;import java.io.*;
class PhaseEffects {
 static int mask;static String trace="";static Throwable[] errors;
 static void reset(int bits,int kind){mask=bits;trace="";errors=new Throwable[6];for(int i=0;i<6;i++)errors[i]=kind==0?new IOException("p"+i):kind==1?new IllegalArgumentException("p"+i):new AssertionError("p"+i);}
 static void fail(int port)throws IOException{if((mask&(1<<port))==0)return;Throwable e=errors[port];if(e instanceof IOException)throw(IOException)e;if(e instanceof RuntimeException)throw(RuntimeException)e;throw(Error)e;}
 static Phase open(int phase,String input)throws IOException{trace+="O"+phase;fail(3*(phase-1));return new Phase(phase,input);}
}
class Phase implements AutoCloseable {
 final int phase;final String input;
 Phase(int phase,String input){this.phase=phase;this.input=input;}
 String read()throws IOException{PhaseEffects.trace+="R"+phase;PhaseEffects.fail(3*(phase-1)+1);return input;}
 public void close()throws IOException{PhaseEffects.trace+="C"+phase;PhaseEffects.fail(3*(phase-1)+2);}
}
public class UncapturedLambdaNames {
 public static String normalize(String input){
  final String first;
  try(Phase reader=PhaseEffects.open(1,input)){first=reader.read();}catch(IOException error){throw new IllegalStateException("phase1",error);}
  try(Phase stream=PhaseEffects.open(2,first)){return stream.read();}catch(IOException error){throw new IllegalStateException("phase2",error);}
 }
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int mask=0;mask<64;mask++)for(int kind=0;kind<3;kind++)for(int nullable=0;nullable<2;nullable++){
   String input=nullable==0?null:new String("value");PhaseEffects.reset(mask,kind);
   StringBuilder expected=new StringBuilder();int primary=-1,suppressed=-1,failedPhase=0;
   for(int phase=1;phase<=2;phase++){
    int base=3*(phase-1);expected.append("O").append(phase);
    if((mask&(1<<base))!=0){primary=base;failedPhase=phase;break;}
    expected.append("R").append(phase);if((mask&(1<<(base+1)))!=0)primary=base+1;
    expected.append("C").append(phase);if((mask&(1<<(base+2)))!=0){if(primary<0)primary=base+2;else suppressed=base+2;}
    if(primary>=0){failedPhase=phase;break;}
   }
   String result=null;Throwable caught=null;try{result=UncapturedLambdaNames.normalize(input);}catch(Throwable error){caught=error;}
   if(!PhaseEffects.trace.equals(expected.toString()))throw new AssertionError("sequential phase/close order expected="+expected+" actual="+PhaseEffects.trace,caught);
   if(primary<0){if(caught!=null||result!=input)throw new AssertionError("result identity",caught);}
   else{
    Throwable actual=caught;
    if(kind==0){if(caught==null||caught.getClass()!=IllegalStateException.class||!caught.getMessage().equals("phase"+failedPhase))throw new AssertionError("wrong checked wrapper",caught);actual=caught.getCause();}
    if(actual!=PhaseEffects.errors[primary])throw new AssertionError("wrong primary identity",caught);
    Throwable[] extra=actual.getSuppressed();if(extra.length!=(suppressed<0?0:1)||(suppressed>=0&&extra[0]!=PhaseEffects.errors[suppressed]))throw new AssertionError("suppression identity/order",caught);
   }
   rows++;
  }
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialSequentialResourcePhasesKeepCleanupAndFailureDomains(t *testing.T) {
	testResourceCompletionMetadataRoundTrip(t, sequentialResourceFixture, 0, "384")
}

// Four states at each of six independent failure ports exhaust all mixed
// checked/runtime/Error primary-vs-close combinations. Higher phases are still
// enrolled even when a preceding failure makes them unreachable in that world.
func sequentialResourceMixedFailureFixture() string {
	source := strings.Replace(sequentialResourceFixture,
		`static void reset(int bits,int kind){mask=bits;trace="";errors=new Throwable[6];for(int i=0;i<6;i++)errors[i]=kind==0?new IOException("p"+i):kind==1?new IllegalArgumentException("p"+i):new AssertionError("p"+i);}`,
		`static void reset(int states,int unused){mask=0;trace="";errors=new Throwable[6];for(int i=0;i<6;i++){int state=(states >>> (2*i))&3;if(state!=0)mask|=1<<i;errors[i]=state==1?new IOException("p"+i):state==2?new IllegalArgumentException("p"+i):new AssertionError("p"+i);}}`, 1)
	source = strings.Replace(source, `for(int mask=0;mask<64;mask++)for(int kind=0;kind<3;kind++)for(int nullable=0;nullable<2;nullable++){`, `for(int states=0;states<4096;states++)for(int nullable=0;nullable<2;nullable++){`, 1)
	source = strings.Replace(source, `PhaseEffects.reset(mask,kind);`, `int mask=0;for(int port=0;port<6;port++)if(((states >>> (2*port))&3)!=0)mask|=1<<port;PhaseEffects.reset(states,0);`, 1)
	source = strings.Replace(source, `if(kind==0){if(caught==null`, `if(PhaseEffects.errors[primary] instanceof IOException){if(caught==null`, 1)
	return source
}

func TestAdversarialSequentialResourceFailuresKeepIndependentPrimaryAndSuppressionKinds(t *testing.T) {
	testResourceCompletionMetadataRoundTrip(t, sequentialResourceMixedFailureFixture(), 0, "8192")
}

// ldc of a class resolves a Class object without initializing that class. It
// remains a real producer at its original exception-domain PC, even when used
// only as an argument in a later, independent resource phase.
func TestAdversarialSequentialResourceClassLiteralKeepsResolutionAndCleanupDomains(t *testing.T) {
	source := strings.Replace(sequentialResourceMixedFailureFixture(), `class Phase implements AutoCloseable {`, `class LiteralOnly {static {PhaseEffects.trace+="L";}}
class Phase implements AutoCloseable {`, 1)
	source = strings.Replace(source, `String read()throws IOException`, `String read(Class<?> token)throws IOException{if(token!=LiteralOnly.class)throw new AssertionError("literal identity");return read();}
 String read()throws IOException`, 1)
	source = strings.Replace(source, `return stream.read();`, `return stream.read(LiteralOnly.class);`, 1)
	testResourceCompletionMetadataRoundTrip(t, source, 0, "8192")
}
