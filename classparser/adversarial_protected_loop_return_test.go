package javaclassparser

import (
	"strings"
	"testing"
)

const protectedLoopReturnFixture = `
import java.util.*;import java.io.*;import java.nio.file.*;
class LoopEffects {
 static int names,pending,duplicates,mode;static String trace="";
 static final String[] tokens={new String("a"),new String("b"),new String("c"),new String("d"),new String("e"),new String("f"),new String("g")};
 static final FileAlreadyExistsException duplicate=new FileAlreadyExistsException("same duplicate");
 static final IOException io=new IOException("same io");static final NullPointerException nil=new NullPointerException("same null");
 static String next(){trace+="N";if(names>=tokens.length)throw new AssertionError("unbounded retry");return tokens[names++];}
 static boolean pending(String name){trace+="P";if(pending>0){pending--;return true;}return false;}
}
class LoopOutput {
 final String name;
 LoopOutput(String name)throws IOException{LoopEffects.trace+="C";if(LoopEffects.duplicates>0){LoopEffects.duplicates--;throw LoopEffects.duplicate;}if(LoopEffects.mode==1)throw LoopEffects.io;if(LoopEffects.mode==2)throw LoopEffects.nil;this.name=name;}
}
public class UncapturedLambdaNames {
 public static LoopOutput make()throws IOException{
  String name=null;
  for(;;){try{name=LoopEffects.next();if(LoopEffects.pending(name))continue;return new LoopOutput(name);}catch(FileAlreadyExistsException error){continue;}}
 }
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(int pending=0;pending<3;pending++)for(int duplicates=0;duplicates<3;duplicates++)for(int mode=0;mode<3;mode++){
   LoopEffects.names=0;LoopEffects.pending=pending;LoopEffects.duplicates=duplicates;LoopEffects.mode=mode;LoopEffects.trace="";LoopOutput output=null;Throwable caught=null;
   try{output=UncapturedLambdaNames.make();}catch(Throwable error){caught=error;}
   StringBuilder expected=new StringBuilder();for(int i=0;i<pending;i++)expected.append("NP");for(int i=0;i<=duplicates;i++)expected.append("NPC");
   Throwable failure=mode==1?LoopEffects.io:mode==2?LoopEffects.nil:null;
   if(caught!=failure||!LoopEffects.trace.equals(expected.toString())||LoopEffects.names!=pending+duplicates+1||LoopEffects.pending!=0||LoopEffects.duplicates!=0||(mode==0?(output==null||output.name!=LoopEffects.tokens[pending+duplicates]):output!=null))throw new AssertionError("protected constructor/retry/order/failure identity expected="+expected+" actual="+LoopEffects.trace,caught);rows++;
  }
  List<String> declarations=new ArrayList<>();for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());declarations.add(row.toString());
  }
  Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));System.out.println(rows+":uncaptured:lambda:parameters:"+Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialLoopReturnConstructorKeepsOriginalProtectedRetryRegion(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, protectedLoopReturnFixture, 0, "27")
}

// The retry handler belongs to every original protected producer in a folded
// return, regardless of whether the root is an allocation or an invocation.
func TestAdversarialLoopReturnStaticFactoryKeepsOriginalProtectedRetryRegion(t *testing.T) {
	source := strings.Replace(protectedLoopReturnFixture, "return new LoopOutput(name);", "return LoopEffects.open(name);", 1)
	source = strings.Replace(source, "static int names,pending,duplicates,mode;", `static LoopOutput open(String name)throws IOException{return new LoopOutput(name);}
 static int names,pending,duplicates,mode;`, 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 0, "27")
}

func TestAdversarialLoopReturnConstructorArgumentsKeepOriginalProtectedRetryRegion(t *testing.T) {
	source := strings.Replace(protectedLoopReturnFixture, "static int names,pending,duplicates,mode;", `static String argument(String name)throws IOException{trace+="A";return name;}
 static int names,pending,duplicates,mode;`, 1)
	source = strings.Replace(source, "return new LoopOutput(name);", "return new LoopOutput(LoopEffects.argument(name));", 1)
	source = strings.ReplaceAll(source, "append(\"NPC\")", "append(\"NPAC\")")
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 0, "27")
}

// An ordinary break really ends the exception interval. A later constructor
// failure must escape once; extending the retry handler would change identity,
// attempts and effects even though a successful return has the same value.
func TestAdversarialLoopUnprotectedReturnCannotBorrowRetryHandler(t *testing.T) {
	source := strings.Replace(protectedLoopReturnFixture, "static int names,pending,duplicates,mode;", `static void prepared(String name)throws IOException{trace+="V";}
 static int names,pending,duplicates,mode;`, 1)
	source = strings.Replace(source, "for(;;){try{name=LoopEffects.next();if(LoopEffects.pending(name))continue;return new LoopOutput(name);}catch(FileAlreadyExistsException error){continue;}}", "for(;;){try{name=LoopEffects.next();if(LoopEffects.pending(name))continue;LoopEffects.prepared(name);}catch(FileAlreadyExistsException error){continue;}break;}return new LoopOutput(name);", 1)
	source = strings.Replace(source, "for(int i=0;i<=duplicates;i++)expected.append(\"NPC\");", "expected.append(\"NPVC\");", 1)
	source = strings.Replace(source, "Throwable failure=mode==1?LoopEffects.io:mode==2?LoopEffects.nil:null;", "Throwable failure=duplicates>0?LoopEffects.duplicate:mode==1?LoopEffects.io:mode==2?LoopEffects.nil:null;", 1)
	source = strings.Replace(source, "LoopEffects.names!=pending+duplicates+1", "LoopEffects.names!=pending+1", 1)
	source = strings.Replace(source, "LoopEffects.duplicates!=0", "LoopEffects.duplicates!=Math.max(0,duplicates-1)", 1)
	source = strings.Replace(source, "(mode==0?(output==null||output.name!=LoopEffects.tokens[pending+duplicates]):output!=null)", "(failure==null?(output==null||output.name!=LoopEffects.tokens[pending]):output!=null)", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 0, "27")
}

// A retry may exist only on an exceptional edge. A normal continue is not a
// prerequisite for keeping the successful constructor inside its handler.
func TestAdversarialConstructorReturnKeepsExceptionOnlyRetryLoop(t *testing.T) {
	source := strings.Replace(protectedLoopReturnFixture, `if(LoopEffects.pending(name))continue;`, `LoopEffects.pending(name);`, 1)
	source = strings.Replace(source, `pending<3`, `pending<1`, 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 0, "9")
}
