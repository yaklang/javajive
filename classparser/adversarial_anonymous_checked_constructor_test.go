package javaclassparser

import (
	"strings"
	"testing"
)

// Native javac8 omits Exceptions on this synthetic anonymous constructor.
// Modern javac infers IOException from the parent. The exception attribute
// does not govern execution: original hidden captures still precede SUPER,
// while the instance initializer must never run after an abrupt parent.
const checkedAnonymousConstructorFixture = `import java.io.IOException;
public class ArtifactLambdaOwner {
 public static String trace="";public static boolean fail;public static int initializations;
 public static final IOException failure=new IOException("original parent");
 public static ArtifactCheckedParent published;
 public static long mark(long value){trace+="I";initializations++;return value;}
 public static ArtifactCheckedParent make(final long capture,long seed)throws IOException{
  trace+="A";return new ArtifactCheckedParent(seed){
   final long initialized=mark(capture*7L+3L);
   protected long capture(){return capture;}
   public long value(long input){return initialized+input;}
  };
 }
}
abstract class ArtifactCheckedParent {
 public final long seed;public final long observed;
 protected ArtifactCheckedParent(long value)throws IOException{
  ArtifactLambdaOwner.trace+="P";ArtifactLambdaOwner.published=this;seed=value;observed=capture();
  if(ArtifactLambdaOwner.fail)throw ArtifactLambdaOwner.failure;
 }
 protected abstract long capture();public abstract long value(long input);
}
class ArtifactLambdaDriver {
 static long initialized(long value){return java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(7L)).add(java.math.BigInteger.valueOf(3L)).longValue();}
 static long add(long a,long b){return java.math.BigInteger.valueOf(a).add(java.math.BigInteger.valueOf(b)).longValue();}
 public static void main(String[]args)throws Exception{int rows=0;
  for(long seed:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(long capture:new long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){
   ArtifactLambdaOwner.trace="";ArtifactLambdaOwner.initializations=0;ArtifactLambdaOwner.published=null;ArtifactLambdaOwner.fail=fail;
   ArtifactCheckedParent value=null;Throwable error=null;
   try{value=ArtifactLambdaOwner.make(capture,seed);}catch(Throwable e){error=e;}
   ArtifactCheckedParent published=ArtifactLambdaOwner.published;
   if(published==null||published.seed!=seed||published.observed!=capture||published.capture()!=capture)throw new AssertionError("captures must precede publishing SUPER");
   if(error!=(fail?ArtifactLambdaOwner.failure:null)||ArtifactLambdaOwner.initializations!=(fail?0:1)||!ArtifactLambdaOwner.trace.equals(fail?"AP":"API"))throw new AssertionError("checked exception identity and initialization phase");
   if(fail){if(published.value(11L)!=11L||value!=null)throw new AssertionError("abrupt parent must leave default initializer state");}
   else if(value!=published||value.value(seed)!=add(initialized(capture),seed))throw new AssertionError("wide initializer/return bindings");
   rows++;
  }
  System.out.println(rows+":checked:anonymous:super:failure:order:overflow");
 }
}`

func TestAdversarialAnonymousCheckedParentKeepsOriginalInitializationAndException(t *testing.T) {
	testArtifactLambdaMetadata(t, checkedAnonymousConstructorFixture, []bool{false, true}, "30:checked:anonymous:super:failure:order:overflow\n")
}

func TestAdversarialAnonymousCheckedParentRetainsOrderedUncheckedDeclaration(t *testing.T) {
	fixture := strings.Replace(checkedAnonymousConstructorFixture,
		"protected ArtifactCheckedParent(long value)throws IOException{",
		"protected ArtifactCheckedParent(long value)throws IOException,IllegalStateException{", 1)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:checked:anonymous:super:failure:order:overflow\n")
}

func TestAdversarialAnonymousCheckedParentRetainsMultipleCheckedDeclarations(t *testing.T) {
	fixture := strings.NewReplacer(
		"public static final IOException failure=new IOException(", "public static final java.io.EOFException failure=new java.io.EOFException(",
		"protected ArtifactCheckedParent(long value)throws IOException{", "protected ArtifactCheckedParent(long value)throws java.io.EOFException,java.sql.SQLException{",
		"public static ArtifactCheckedParent make(final long capture,long seed)throws IOException{", "public static ArtifactCheckedParent make(final long capture,long seed)throws IOException,java.sql.SQLException{",
	).Replace(checkedAnonymousConstructorFixture)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:checked:anonymous:super:failure:order:overflow\n")
}

func TestAdversarialAnonymousCheckedParentEliminatesCoveredExceptionDeclaration(t *testing.T) {
	fixture := strings.Replace(checkedAnonymousConstructorFixture,
		"protected ArtifactCheckedParent(long value)throws IOException{",
		"protected ArtifactCheckedParent(long value)throws IOException,java.io.EOFException{", 1)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:checked:anonymous:super:failure:order:overflow\n")
}

func TestAdversarialAnonymousUncheckedParentDoesNotInventExceptionAttribute(t *testing.T) {
	fixture := strings.NewReplacer(
		"public static final IOException failure=new IOException(", "public static final IllegalStateException failure=new IllegalStateException(",
		"protected ArtifactCheckedParent(long value)throws IOException{", "protected ArtifactCheckedParent(long value)throws IllegalStateException{",
	).Replace(checkedAnonymousConstructorFixture)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "30:checked:anonymous:super:failure:order:overflow\n")
}
