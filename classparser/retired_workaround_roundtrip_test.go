package javaclassparser

import "testing"

// Turning off a retired source workaround must not reintroduce its old bug.
// Exercise the production pipeline and original JVM independently; assertions
// include handler dispatch, shared assignment identity and generic return use.
func TestAdversarialRetiredWorkaroundContractsRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_HARDJAR_SHAPE_OFF", "JDEC_NULL_ADOPTED_SUBTYPE_REASSIGN_OFF", "JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "JDEC_JACKSON_REMAINING_OFF", "JDEC_ORIG14_REMAINING_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "RetiredContracts", `import java.util.*;import java.io.*;import java.util.zip.*;
class ContractHelpers {
 static final Object primary=new Object(),fallback=new Object();
 static Object select(boolean fail){if(fail)throw new IllegalStateException();return primary;}
 static InputStream stream(byte[] bytes){return new ByteArrayInputStream(bytes);}
 static byte[] compressed(byte[] bytes)throws IOException{ByteArrayOutputStream out=new ByteArrayOutputStream();GZIPOutputStream gzip=new GZIPOutputStream(out);gzip.write(bytes);gzip.close();return out.toByteArray();}
}
public class RetiredContracts<T> {
 final Object delegate;
 RetiredContracts(boolean fail){Object selected;try{selected=ContractHelpers.select(fail);}catch(Throwable e){selected=ContractHelpers.fallback;}delegate=selected;}
 int sum(boolean gzip,byte[] first,byte[] second)throws IOException{
  InputStream input=null;input=first!=null?ContractHelpers.stream(first):ContractHelpers.stream(second);
  if(gzip)input=new GZIPInputStream(input);
  int sum=0,c;while((c=input.read())!=-1)sum+=c;return sum;
 }
 <E extends T> List<E> limit(Collection<E> input,int count){Object[] items=input.toArray();if(items.length>count)items=Arrays.copyOf(items,count);return (List<E>)(List)Collections.unmodifiableList(Arrays.asList(items));}
 Iterable<T> values(Object[] input){return (Iterable<T>)(Iterable)Collections.unmodifiableList(Arrays.asList(input));}
 public static void main(String[] args)throws Exception{
  for(boolean fail:new boolean[]{false,true}){
   RetiredContracts<String> x=new RetiredContracts<>(fail);System.out.println(x.delegate==ContractHelpers.primary);System.out.println(x.delegate==ContractHelpers.fallback);
   for(boolean gzip:new boolean[]{false,true})for(boolean first:new boolean[]{false,true})for(byte[] raw:new byte[][]{new byte[0],new byte[]{0,1,127,-1}}){byte[] data=gzip?ContractHelpers.compressed(raw):raw;System.out.println(x.sum(gzip,first?data:null,data));}
   for(int count=0;count<4;count++){List<String> result=x.limit(Arrays.asList("a","b","c"),count);System.out.println(result);try{result.add("bad");}catch(UnsupportedOperationException e){System.out.println(e.getClass().getName());}}
   for(String value:x.values(new String[]{"x","y"}))System.out.println(value);
   try{x.sum(true,new byte[]{1,2},null);}catch(IOException e){System.out.println(e.getClass().getName());}
  }
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialExceptionContractWithoutGuessedCatchTypesRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_HARDJAR_SHAPE_OFF", "JDEC_JACKSON_REMAINING_OFF", "JDEC_ORIG14_REMAINING_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "ExceptionContracts", `import java.util.concurrent.*;import java.beans.*;
class ExceptionContractHelpers {
 static Object construct(String name)throws Exception{return Class.forName(name).getConstructor(new Class[0]).newInstance(new Object[0]);}
 static BeanInfo info(int kind)throws IntrospectionException{if(kind==1)throw new IntrospectionException("checked");if(kind==2)throw new IllegalStateException("unchecked");return null;}
 static String get(int kind)throws Exception{if(kind==1)throw new ExecutionException(new IllegalArgumentException());if(kind==2)throw new InterruptedException();if(kind==3)throw new IllegalStateException();if(kind==4)throw new AssertionError();return "ok";}
}
public class ExceptionContracts {
 static Object make(String name)throws Exception{try{return ExceptionContractHelpers.construct(name);}catch(ClassNotFoundException e){throw new RuntimeException(e);}}
 static BeanInfo inspect(int kind)throws IntrospectionException{return ExceptionContractHelpers.info(kind);}
 static String info(int kind){try{inspect(kind);return "ok";}catch(IntrospectionException e){return "checked";}}
 static String future(int kind){try{return ExceptionContractHelpers.get(kind);}catch(ExecutionException e){return "execution:"+e.getCause().getClass().getName();}catch(Exception e){return "other:"+e.getClass().getName();}}
 public static void main(String[] args){
  for(String name:new String[]{"java.lang.Object","java.lang.Integer","no.such.Type"})try{System.out.println(make(name).getClass().getName());}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e.getCause()==null?"none":e.getCause().getClass().getName()));}
  for(int kind=0;kind<3;kind++)try{System.out.println(info(kind));}catch(Throwable e){System.out.println(e.getClass().getName());}
  for(int kind=0;kind<5;kind++)try{System.out.println(future(kind));}catch(Throwable e){System.out.println(e.getClass().getName());}
 }
}`, Precision, Compatibility, "legacy")
}
