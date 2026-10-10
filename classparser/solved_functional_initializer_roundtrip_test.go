package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSolvedFunctionalInitializerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "SolvedFunctionalInitializer", `import java.util.*;
interface MissingBiConsumer<A,B> {void accept(A a,B b);}
interface MissingTriAction<A,B,C> {void accept(A a,long sequence,B b,C c);}
public class SolvedFunctionalInitializer {
  static MissingBiConsumer<String,Object> action(int kind) {
    InitializerSink<String> sink=new InitializerSink<>(1);
    MissingBiConsumer<String,Object> first=(key,value)->sink.accept(key,value);
    InitializerOracle.drive(first,"before",0);
    MissingBiConsumer<String,Object> selected=first;
    InitializerOracle.drive(first,"after",0);
    if(kind!=0)selected=(key,value)->InitializerOracle.accept(key,value,3);
    return selected;
  }
  static int execute(int kind,Object key,Object value) {
    InitializerOracle.reset();
    MissingBiConsumer<String,Object> selected=action(kind);
    return InitializerOracle.drive(selected,key,value);
  }
  static int tri(Object key,Object value,Object context,boolean unused) {
    InitializerOracle.reset();
    MissingTriAction<String,Object,StringBuilder> action;
    if(unused)action=(text,sequence,item,ignored)->InitializerOracle.accept(text,item,5+(int)(sequence&7));
    else action=(text,sequence,item,builder)->{builder.append(text);InitializerOracle.accept(text,item,7+(int)(sequence&7));};
    return InitializerOracle.driveTri(action,key,value,context);
  }
  public static void main(String[] args){InitializerOracle.run();}
}
class InitializerSink<T> {
  final int factor;InitializerSink(int factor){this.factor=factor;}
  void accept(String key,Object value){InitializerOracle.accept(key,value,factor);}
}
class InitializerOracle {
  static StringBuilder trace;static int result;
  static void reset(){trace=new StringBuilder();result=0;}
  static void accept(String key,Object value,int factor){trace.append("body;");result+=key.length()*factor+String.valueOf(value).length();}
  @SuppressWarnings({"rawtypes","unchecked"}) static int drive(MissingBiConsumer selected,Object key,Object value){selected.accept(key,value);selected.accept(key,value);return result;}
  @SuppressWarnings({"rawtypes","unchecked"}) static int driveTri(MissingTriAction selected,Object key,Object value,Object context){selected.accept(key,-17L,value,context);selected.accept(key,-17L,value,context);return result;}
  static void run(){
    Object[] keys={null,"","text",Integer.valueOf(8)};
    Object[] values={null,"","payload",Integer.valueOf(0),Arrays.asList("a","b")};
    for(int mode=0;mode<4;mode++)for(int k=0;k<keys.length;k++)for(int v=0;v<values.length;v++) {
      String outcome;
      try{outcome="ok:"+SolvedFunctionalInitializer.execute(mode,keys[k],values[v]);}
      catch(Throwable e){outcome=e.getClass().getSimpleName();}
      System.out.println(mode+":"+k+":"+v+":"+outcome+":"+trace);
    }
    for(int unused=0;unused<2;unused++)for(int k=0;k<keys.length;k++)for(int v=0;v<values.length;v++)for(int ctx=0;ctx<4;ctx++) {
      Object context=ctx==0 ? null : ctx==1 ? new StringBuilder() : ctx==2 ? Integer.valueOf(3) : new String[]{"x"};
      String outcome;
      try{outcome="ok:"+SolvedFunctionalInitializer.tri(keys[k],values[v],context,unused!=0);}
      catch(Throwable e){outcome=e.getClass().getSimpleName()+(e instanceof ClassCastException ? ":"+e.getMessage() : "");}
      System.out.println("tri:"+unused+":"+k+":"+v+":"+ctx+":"+outcome+":"+trace);
    }
  }
}`, func(name string) bool { return !strings.HasPrefix(name, "Missing") }, Precision, Compatibility, "legacy")
}
