package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Computational definitions in duplicated finally copies are not sufficient
// evidence for splitting their shared structured-source exception binder.
// The unchanged driver observes every footer/close, exception identity,
// suppression, and overflowed return across normal and abrupt completions.
// Resource closing is in the unchanged caller, outside the recompiled reader.
const pendingCleanupBindingFixture = `
class PendingInput implements AutoCloseable {
 final long[] values;final int mode;final boolean failFooter,failClose;
 final Throwable body=new java.io.IOException("body"),read=new AssertionError("read"),corrupt=new java.io.IOException("corrupt"),footer=new java.io.IOException("footer");
 final java.io.IOException close=new java.io.IOException("close");String trace="";int cursor,footers,closes;
 PendingInput(long[]v,int m,boolean f,boolean c){values=v;mode=m;failFooter=f;failClose=c;}
 void header()throws Throwable{trace+="H";if(mode==1)throw body;}
 long next()throws Throwable{trace+="R";if(mode==2)throw read;return values[cursor++];}
 void finish(Throwable prior)throws Throwable{footers++;trace+="F";if(failFooter)throw footer;if(prior!=null)throw prior;}
 public void close()throws java.io.IOException{closes++;trace+="C";if(failClose)throw close;}
}
class PendingReader {
 static long read(PendingInput supplied,boolean valid)throws Throwable{
  PendingInput input=supplied;Throwable prior=null;
   try{input.header();long sum=0;for(int i=0;i<input.values.length;i++)sum+=input.next();if(!valid)throw input.corrupt;else{long result=sum;return result;}}
   catch(Throwable failure){prior=failure;}
   finally{input.finish(prior);}
   throw new AssertionError("unreachable");
 }
}
class PendingDriver {public static void main(String[]args){int rows=0;for(long[]values:new long[][]{{},{0},{Long.MAX_VALUE,1},{Long.MIN_VALUE,-1,3}})for(int mode=0;mode<3;mode++)for(boolean footer:new boolean[]{false,true})for(boolean close:new boolean[]{false,true})for(boolean valid:new boolean[]{false,true}){
 PendingInput in=new PendingInput(values,mode,footer,close);Object actual;try{actual=Long.valueOf(PendingReader.read(in,valid));}catch(Throwable failure){actual=failure;}try{in.close();}catch(Throwable failure){if(actual instanceof Throwable)((Throwable)actual).addSuppressed(failure);else actual=failure;}
 Throwable primary=footer?in.footer:mode==1?in.body:mode==2&&values.length>0?in.read:!valid?in.corrupt:null;Throwable expected=primary!=null?primary:close?in.close:null;
 if(expected==null){java.math.BigInteger sum=java.math.BigInteger.ZERO;for(long value:values)sum=sum.add(java.math.BigInteger.valueOf(value));if(!(actual instanceof Long)||((Long)actual).longValue()!=sum.longValue())throw new AssertionError("return word");}else{if(actual!=expected)throw new AssertionError("failure identity/precedence");Throwable[] suppressed=expected.getSuppressed();if(primary!=null&&close){if(suppressed.length!=1||suppressed[0]!=in.close)throw new AssertionError("close suppression identity");}else if(suppressed.length!=0)throw new AssertionError("extra suppression");}
 int reads=mode==1?0:mode==2?Math.min(1,values.length):values.length;String trace="H";for(int i=0;i<reads;i++)trace+="R";trace+="FC";if(!in.trace.equals(trace)||in.footers!=1||in.closes!=1)throw new AssertionError("cleanup evaluation domain:"+in.trace);
 rows++;}System.out.println(rows+":pending:footer:resource:identity:suppression:return");}}
`

func TestAdversarialPendingCleanupBindingKeepsFinallyDomains(t *testing.T) {
	for _, namespace := range []string{"Pending", "CleanupState"} {
		t.Run(namespace, func(t *testing.T) {
			fixture := strings.ReplaceAll(pendingCleanupBindingFixture, "Pending", namespace)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, fixture, []string{namespace + "Reader"}, namespace+"Driver", "96:pending:footer:resource:identity:suppression:return\n", nativeLexicalExactSignatures)
			})
			t.Run("original8", func(t *testing.T) {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, fixture, namespace+"Reader", debug)
				}, NativeJavac8, compiler, []string{namespace + "Reader"}, namespace+"Driver", "96:pending:footer:resource:identity:suppression:return\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}

// The post-finally continuation is reachable here. Its callback must retain
// its own evaluation/failure domain, including when cleanup replaces a return
// or a caught failure. Keep the observing input and driver as original binaries.
func TestAdversarialFinallyContinuationKeepsCallbackEffects(t *testing.T) {
	const fixture = `
class CompletionInput {
 final boolean failBody,failFooter,failTail;
 final java.io.IOException body=new java.io.IOException("body"),footer=new java.io.IOException("footer");
 final Throwable continuation=new AssertionError("continuation");final RuntimeException tail=new IllegalStateException("tail");
 int footers,tails;String trace="";Throwable observed;
 CompletionInput(boolean b,boolean f,boolean t){failBody=b;failFooter=f;failTail=t;}
 void header()throws Throwable{trace+="H";if(failBody)throw body;}
 void finish(Throwable prior)throws Throwable{trace+="F";footers++;observed=prior;if(failFooter)throw footer;}
 Throwable after(){trace+="T";tails++;if(failTail)throw tail;return continuation;}
}
class CompletionReader {
 static long read(CompletionInput input)throws Throwable{
  Throwable prior=null;
  try{input.header();return Long.MAX_VALUE;}
  catch(Throwable failure){prior=failure;}
  finally{input.finish(prior);}
  throw input.after();
 }
}
class CompletionDriver {public static void main(String[]args){int rows=0;for(boolean body:new boolean[]{false,true})for(boolean footer:new boolean[]{false,true})for(boolean tail:new boolean[]{false,true}){
 CompletionInput input=new CompletionInput(body,footer,tail);Object actual;try{actual=Long.valueOf(CompletionReader.read(input));}catch(Throwable failure){actual=failure;}
 Throwable expected=footer?input.footer:body?(tail?input.tail:input.continuation):null;
 if(expected==null){if(!(actual instanceof Long)||((Long)actual).longValue()!=Long.MAX_VALUE)throw new AssertionError("pending return");}else if(actual!=expected)throw new AssertionError("failure identity/precedence");
 boolean after=body&&!footer;if(input.footers!=1||input.tails!=(after?1:0)||!input.trace.equals(after?"HFT":"HF")||input.observed!=(body?input.body:null))throw new AssertionError("continuation effects "+input.trace);
 rows++;}if(rows!=8)throw new AssertionError("coverage");System.out.println("8:finally:callback:identity:precedence:return");}}
`
	for _, namespace := range []string{"Completion", "AfterCleanup"} {
		t.Run(namespace, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "Completion", namespace)
			t.Run("modern", func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, source, []string{namespace + "Reader"}, namespace+"Driver", "8:finally:callback:identity:precedence:return\n", nativeLexicalExactSignatures)
			})
			t.Run("original8", func(t *testing.T) {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, namespace+"Reader", debug)
				}, NativeJavac8, compiler, []string{namespace + "Reader"}, namespace+"Driver", "8:finally:callback:identity:precedence:return\n", nil, nativeLexicalExactSignatures)
			})
		})
	}
}
