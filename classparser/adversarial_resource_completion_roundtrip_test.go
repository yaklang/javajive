package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Original-first unchanged callers observe pending returns and every cleanup,
// exception identity, suppression order and array-store evaluation domain.
const pendingResourceCompletionFixture = `
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
  try(PendingInput input=supplied){Throwable prior=null;
   try{input.header();long[]words=new long[input.values.length];for(int i=0;i<words.length;i++)words[i]=input.next();long sum=0;for(int i=0;i<words.length;i++)sum+=words[i];if(!valid)throw input.corrupt;else{long result=sum;return result;}}
   catch(Throwable failure){prior=failure;}
   finally{input.finish(prior);}
   throw new AssertionError("unreachable");
  }
 }
}
class PendingDriver {public static void main(String[]args){int rows=0;for(long[]values:new long[][]{{},{0},{Long.MAX_VALUE,1},{Long.MIN_VALUE,-1,3}})for(int mode=0;mode<3;mode++)for(boolean footer:new boolean[]{false,true})for(boolean close:new boolean[]{false,true})for(boolean valid:new boolean[]{false,true}){
 PendingInput in=new PendingInput(values,mode,footer,close);Object actual;try{actual=Long.valueOf(PendingReader.read(in,valid));}catch(Throwable failure){actual=failure;}
 Throwable primary=footer?in.footer:mode==1?in.body:mode==2&&values.length>0?in.read:!valid?in.corrupt:null;Throwable expected=primary!=null?primary:close?in.close:null;
 if(expected==null){java.math.BigInteger sum=java.math.BigInteger.ZERO;for(long value:values)sum=sum.add(java.math.BigInteger.valueOf(value));if(!(actual instanceof Long)||((Long)actual).longValue()!=sum.longValue())throw new AssertionError("return word");}else{if(actual!=expected)throw new AssertionError("failure identity/precedence");Throwable[] suppressed=expected.getSuppressed();if(primary!=null&&close){if(suppressed.length!=1||suppressed[0]!=in.close)throw new AssertionError("close suppression identity");}else if(suppressed.length!=0)throw new AssertionError("extra suppression");}
 int reads=mode==1?0:mode==2?Math.min(1,values.length):values.length;String trace="H";for(int i=0;i<reads;i++)trace+="R";trace+="FC";if(!in.trace.equals(trace)||in.footers!=1||in.closes!=1)throw new AssertionError("cleanup evaluation domain:"+in.trace);
 rows++;}System.out.println(rows+":pending:footer:resource:identity:suppression:return");}}
`

func TestAdversarialPendingCleanupBindingKeepsFinallyAndResourceDomains(t *testing.T) {
	for _, namespace := range []string{"Pending", "CleanupState"} {
		t.Run(namespace, func(t *testing.T) {
			fixture := strings.ReplaceAll(pendingResourceCompletionFixture, "Pending", namespace)
			testResourceCompletionFamily(t, fixture, namespace+"Reader", namespace+"Driver", "96:pending:footer:resource:identity:suppression:return\n")
		})
	}
}

const resourceArrayStoreDomainFixture = `
class StoreState implements AutoCloseable {
 final Throwable indexFailure=new IllegalArgumentException("index"),valueFailure=new IllegalStateException("value"),footerFailure=new AssertionError("footer");
 final java.io.IOException closeFailure=new java.io.IOException("close");
 final boolean footer,closing;String trace="";int result=41,footers,closes;
 StoreState(boolean f,boolean c){footer=f;closing=c;}
 int index(int mode)throws Throwable {trace+="I";if(mode==3)throw indexFailure;return mode-1;}
 Object value(int mode)throws Throwable {trace+="V";if(mode==2)throw valueFailure;return mode==0?"text":new Object();}
 int result(){trace+="R";return result;}
 void finish()throws Throwable {trace+="F";footers++;result=99;if(footer)throw footerFailure;}
 public void close()throws java.io.IOException {trace+="C";closes++;if(closing)throw closeFailure;}
}
class StoreReader {
 static int write(StoreState supplied,Object[] array,int indexMode,int valueMode)throws Throwable {
  try(StoreState resource=supplied){
   try {array[resource.index(indexMode)]=resource.value(valueMode);return resource.result();}
   finally {resource.finish();}
  }
 }
}
class StoreDriver {
 public static void main(String[]args){int rows=0;
 for(int kind=0;kind<4;kind++)for(int index=0;index<4;index++)for(int value=0;value<3;value++)for(boolean footer:new boolean[]{false,true})for(boolean close:new boolean[]{false,true}){
  Object[] array=kind==0?null:kind==1?new Object[0]:kind==2?new Object[1]:new String[1];StoreState state=new StoreState(footer,close);
  Class<?> system=null;Throwable body=null;
  if(index==3)body=state.indexFailure;
  else if(value==2)body=state.valueFailure;
  else if(kind==0)system=NullPointerException.class;
  else if(kind==1||index!=1)system=ArrayIndexOutOfBoundsException.class;
  else if(kind==3&&value==1)system=ArrayStoreException.class;
  boolean succeeds=body==null&&system==null;
  Throwable expected=footer?state.footerFailure:body!=null?body:system==null&&close?state.closeFailure:null;
  Object actual;try{actual=Integer.valueOf(StoreReader.write(state,array,index,value));}catch(Throwable failure){actual=failure;}
  if(footer||body!=null||system==null&&close){if(actual!=expected)throw new AssertionError("original exception identity/priority");}
  else if(system!=null){if(actual==null||actual.getClass()!=system)throw new AssertionError("array exception class");}
  else if(!(actual instanceof Integer)||((Integer)actual).intValue()!=41)throw new AssertionError("pending return snapshot");
  if(actual instanceof Throwable){Throwable[] suppressed=((Throwable)actual).getSuppressed();boolean suppress=close&&(footer||!succeeds);if(suppress){if(suppressed.length!=1||suppressed[0]!=state.closeFailure)throw new AssertionError("suppression identity");}else if(suppressed.length!=0)throw new AssertionError("extra suppression");}
  String trace="I"+(index==3?"":"V")+(succeeds?"R":"")+"FC";
  if(!state.trace.equals(trace)||state.footers!=1||state.closes!=1)throw new AssertionError("store domain:"+kind+":"+index+":"+value+":"+state.trace+":"+trace);
  rows++;
 }
 System.out.println(rows+":array:index:rhs:store:finally:close:return:suppression");
 }
}
`

func TestAdversarialResourceArrayStoreKeepsEffectsAndPendingReturn(t *testing.T) {
	for _, namespace := range []string{"Store", "MemoryStore"} {
		t.Run(namespace, func(t *testing.T) {
			source := strings.NewReplacer("StoreState", namespace+"State", "StoreReader", namespace+"Reader", "StoreDriver", namespace+"Driver").Replace(resourceArrayStoreDomainFixture)
			testResourceCompletionFamily(t, source, namespace+"Reader", namespace+"Driver", "192:array:index:rhs:store:finally:close:return:suppression\n")
		})
	}
}

const nestedResourceCompletionFixture = `
class DualState {
 final int mask;String trace="";int result=41,footers,innerCloses,outerCloses;
 final Throwable bodyFailure=new AssertionError("body"),footerFailure=new AssertionError("footer");
 final java.io.IOException innerFailure=new java.io.IOException("inner"),outerFailure=new java.io.IOException("outer");
 DualState(int m){mask=m;}
 Object value()throws Throwable{trace+="B";if((mask&1)!=0)throw bodyFailure;return this;}
 int result(){trace+="R";return result;}
 void finish()throws Throwable{trace+="F";footers++;result=99;if((mask&2)!=0)throw footerFailure;}
}
class DualResource implements AutoCloseable {
 final DualState state;final boolean inner;DualResource(DualState s,boolean i){state=s;inner=i;}
 public void close()throws java.io.IOException{if(inner){state.trace+="I";state.innerCloses++;if((state.mask&4)!=0)throw state.innerFailure;}else{state.trace+="O";state.outerCloses++;if((state.mask&8)!=0)throw state.outerFailure;}}
}
class DualReader {
 static int read(DualState state,Object[] array)throws Throwable{
  try(DualResource outer=new DualResource(state,false);DualResource inner=new DualResource(state,true)){
   try{array[0]=state.value();return state.result();}finally{state.finish();}
  }
 }
}
class DualDriver {public static void main(String[]args){for(int mask=0;mask<32;mask++){
 DualState state=new DualState(mask);Object[] array=(mask&16)!=0?null:new Object[1];boolean body=(mask&1)!=0,footer=(mask&2)!=0,inner=(mask&4)!=0,outer=(mask&8)!=0,system=!body&&array==null;
 Throwable primary=footer?state.footerFailure:body?state.bodyFailure:system?null:inner?state.innerFailure:outer?state.outerFailure:null;
 Object actual;try{actual=Integer.valueOf(DualReader.read(state,array));}catch(Throwable failure){actual=failure;}
 if(system&&!footer){if(!(actual instanceof NullPointerException))throw new AssertionError("null store exception");}
 else if(primary!=null){if(actual!=primary)throw new AssertionError("primary identity");}
 else if(!(actual instanceof Integer)||((Integer)actual).intValue()!=41)throw new AssertionError("pending return");
 if(actual instanceof Throwable){Throwable[] suppressed=((Throwable)actual).getSuppressed();boolean pending=footer||body||system;int count=0;if(pending&&inner){if(suppressed.length<=count||suppressed[count++]!=state.innerFailure)throw new AssertionError("inner suppression");}if((pending||inner)&&outer){if(suppressed.length<=count||suppressed[count++]!=state.outerFailure)throw new AssertionError("outer suppression");}if(suppressed.length!=count)throw new AssertionError("extra suppression");}
 String trace="B"+(body||system?"":"R")+"FIO";if(!trace.equals(state.trace)||state.footers!=1||state.innerCloses!=1||state.outerCloses!=1)throw new AssertionError("nested completion:"+mask+":"+state.trace);
 }System.out.println("32:two-resources:finally:pending-return:suppression-order");}}
`

func TestAdversarialNestedResourcePendingCompletionKeepsCloseOrder(t *testing.T) {
	testResourceCompletionFamily(t, nestedResourceCompletionFixture, "DualReader", "DualDriver", "32:two-resources:finally:pending-return:suppression-order\n")
}

func testResourceCompletionFamily(t *testing.T, source, owner, driver, want string) {
	t.Helper()
	t.Run("modern", func(t *testing.T) {
		testNativeIndependentFamilyFixture(t, source, []string{owner}, driver, want, nativeLexicalExactSignatures)
	})
	t.Run("native8", func(t *testing.T) {
		testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
			return localGetterCompileJavac8(t, map[string]string{owner + ".java": source}, debug)
		}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{owner}, driver, want, nil, nativeLexicalExactSignatures)
	})
}

func TestAdversarialResourcePendingCompletionKeepsPreciseRethrowABI(t *testing.T) {
	source := strings.Replace(resourceArrayStoreDomainFixture, `final Throwable indexFailure=new IllegalArgumentException("index"),valueFailure=new IllegalStateException("value"),footerFailure=new AssertionError("footer");`, `final RuntimeException indexFailure=new IllegalArgumentException("index"),valueFailure=new IllegalStateException("value");final Error footerFailure=new AssertionError("footer");`, 1)
	source = strings.ReplaceAll(source, "throws Throwable", "")
	source = strings.Replace(source, "int valueMode)", "int valueMode)throws java.io.IOException", 1)
	source = strings.Replace(source, "public static void main(String[]args){int rows=0;", "public static void main(String[]args)throws Exception{Class<?>[] declared=StoreReader.class.getDeclaredMethod(\"write\",StoreState.class,Object[].class,int.class,int.class).getExceptionTypes();if(declared.length!=1||declared[0]!=java.io.IOException.class)throw new AssertionError(\"precise declared throw ABI\");int rows=0;", 1)
	testResourceCompletionFamily(t, source, "StoreReader", "StoreDriver", "192:array:index:rhs:store:finally:close:return:suppression\n")
}
