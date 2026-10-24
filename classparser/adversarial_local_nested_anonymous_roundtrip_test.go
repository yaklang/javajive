package javaclassparser

import "testing"

func TestAdversarialLocalOwnedNestedAnonymousBindingMVP(t *testing.T) {
	const source = `
class ChainRoot {
 static java.util.function.Supplier<java.util.function.Supplier<Object[]>> make(final java.util.function.Supplier<Object> alias,final long seed){
  class Local {java.util.function.Supplier<java.util.function.Supplier<Object[]>> open(){return new java.util.function.Supplier<java.util.function.Supplier<Object[]>>(){public java.util.function.Supplier<Object[]> get(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed)};}};}};}}
  return new Local().open();
 }
}
class ChainDriver {public static void main(String[]args){int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(final boolean failure:new boolean[]{false,true}){
 final Object token=new Object();final RuntimeException expected=new IllegalStateException("same");final int[] calls={0};
 java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(failure)throw expected;return token;}};
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> activation=ChainRoot.make(alias,seed);if(calls[0]!=0)throw new AssertionError("eager outer capture");
 java.util.function.Supplier<Object[]> first=activation.get(),second=activation.get();if(first==second||calls[0]!=0)throw new AssertionError("activation or eager leaf");
 for(java.util.function.Supplier<Object[]> leaf:new java.util.function.Supplier[]{first,second,first}){try{Object[] value=leaf.get();if(failure||value.length!=2||value[0]!=token||!(value[1]instanceof Long)||((Long)value[1]).longValue()!=seed)throw new AssertionError("binding,identity,wide word");}catch(RuntimeException actual){if(!failure||actual!=expected)throw new AssertionError("failure identity",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");
 Class<?> level1=activation.getClass(),level2=first.getClass();Class<?> local=level1.getEnclosingClass();if(!level1.isAnonymousClass()||!level2.isAnonymousClass()||local==null||!local.isLocalClass()||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!level1.getEnclosingMethod().getName().equals("open")||level2.getEnclosingClass()!=level1||!level2.getEnclosingMethod().getName().equals("get"))throw new AssertionError("actual lexical owner chain");rows++;
 }System.out.println(rows+":local:anonymous:anonymous:activation:identity:wide:deferred");}}
`
	testResourceCompletionFamily(t, source, "ChainRoot", "ChainDriver", "10:local:anonymous:anonymous:activation:identity:wide:deferred\n")
}

func TestAdversarialLocalNestedAnonymousPreSuperPublicationMVP(t *testing.T) {
	const source = `
abstract class EarlyBase implements java.util.function.Supplier<Object[]> {
 static Object published,observed;static long observedWord;static int publications;
 static final RuntimeException failure=new IllegalArgumentException("same");
 EarlyBase(int marker){published=this;publications++;Object[] row=get();observed=row[0];observedWord=((Long)row[1]).longValue();if(marker!=0)throw failure;}
}
class EarlyRoot {
 static java.util.function.Supplier<java.util.function.Supplier<Object[]>> make(final java.util.function.Supplier<Object> alias,final long seed,final int marker){
  class Local {java.util.function.Supplier<java.util.function.Supplier<Object[]>> open(){return new java.util.function.Supplier<java.util.function.Supplier<Object[]>>(){public java.util.function.Supplier<Object[]> get(){return new EarlyBase(marker){public Object[] get(){return new Object[]{alias.get(),Long.valueOf(seed)};}};}};}}
  return new Local().open();
 }
}
class EarlyDriver {public static void main(String[]args){int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int marker=0;marker<2;marker++){
 final Object token=new Object();final int[] calls={0};java.util.function.Supplier<Object> alias=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;return token;}};
 EarlyBase.published=null;EarlyBase.observed=null;EarlyBase.observedWord=0;EarlyBase.publications=0;
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> activation=EarlyRoot.make(alias,seed,marker);if(calls[0]!=0||EarlyBase.publications!=0)throw new AssertionError("eager activation");
 java.util.function.Supplier<Object[]> leaf=null;try{leaf=activation.get();if(marker!=0)throw new AssertionError("lost constructor failure");}catch(RuntimeException actual){if(marker==0||actual!=EarlyBase.failure)throw new AssertionError("constructor failure identity",actual);}
 if(EarlyBase.publications!=1||EarlyBase.published==null||EarlyBase.observed!=token||EarlyBase.observedWord!=seed||calls[0]!=1)throw new AssertionError("pre-super capture storage and callback order");
 if(marker==0){if(leaf!=EarlyBase.published)throw new AssertionError("published receiver identity");Object[] value=leaf.get();if(value[0]!=token||((Long)value[1]).longValue()!=seed||calls[0]!=2)throw new AssertionError("retained binding");}
 Class<?> c=EarlyBase.published.getClass();if(!c.isAnonymousClass()||c.getEnclosingClass()!=activation.getClass()||c.getEnclosingMethod()==null||!c.getEnclosingMethod().getName().equals("get"))throw new AssertionError("published lexical identity");rows++;
 }System.out.println(rows+":nested:local:pre-super:capture:publication:callback:abrupt");}}
`
	testResourceCompletionFamily(t, source, "EarlyRoot", "EarlyDriver", "10:nested:local:pre-super:capture:publication:callback:abrupt\n")
}

func TestAdversarialRenamedLocalNestedAnonymousCaptureIdentity(t *testing.T) {
	const source = `
class 链Owner {
 static java.util.function.Supplier<java.util.function.Supplier<Object[]>> make(final java.util.function.Supplier<Object> var0,final long 种子){
  class 局部 {java.util.function.Supplier<java.util.function.Supplier<Object[]>> open(){return new java.util.function.Supplier<java.util.function.Supplier<Object[]>>(){public java.util.function.Supplier<Object[]> get(){return new java.util.function.Supplier<Object[]>(){public Object[] get(){return new Object[]{var0.get(),Long.valueOf(种子)};}};}};}}
  return new 局部().open();
 }
}
class ChainDriver {public static void main(String[]args){int rows=0;
 for(final long 种子:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(final boolean failure:new boolean[]{false,true}){
 final Object token=new Object();final RuntimeException expected=new IllegalStateException("same");final int[] calls={0};
 java.util.function.Supplier<Object> var0=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(failure)throw expected;return token;}};
 java.util.function.Supplier<java.util.function.Supplier<Object[]>> activation=链Owner.make(var0,种子);if(calls[0]!=0)throw new AssertionError("eager outer capture");
 java.util.function.Supplier<Object[]> first=activation.get(),second=activation.get();if(first==second||calls[0]!=0)throw new AssertionError("activation or eager leaf");
 for(java.util.function.Supplier<Object[]> leaf:new java.util.function.Supplier[]{first,second,first}){try{Object[] value=leaf.get();if(failure||value.length!=2||value[0]!=token||!(value[1]instanceof Long)||((Long)value[1]).longValue()!=种子)throw new AssertionError("binding,identity,wide word");}catch(RuntimeException actual){if(!failure||actual!=expected)throw new AssertionError("failure identity",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation count");
 Class<?> level1=activation.getClass(),level2=first.getClass();Class<?> local=level1.getEnclosingClass();if(!level1.isAnonymousClass()||!level2.isAnonymousClass()||local==null||!local.isLocalClass()||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("make")||!level1.getEnclosingMethod().getName().equals("open")||level2.getEnclosingClass()!=level1||!level2.getEnclosingMethod().getName().equals("get"))throw new AssertionError("actual lexical owner chain");rows++;
 }System.out.println(rows+":local:anonymous:anonymous:activation:identity:wide:deferred");}}
`
	testResourceCompletionFamily(t, source, "链Owner", "ChainDriver", "10:local:anonymous:anonymous:activation:identity:wide:deferred\n")
}

func TestAdversarialThreeAnonymousCaptureEdgesKeepOriginalClassIdentity(t *testing.T) {
	const source = `
class GraphRoot {
 static java.util.function.Supplier make(final java.util.function.Supplier alias,final long seed){
  class Local {java.util.function.Supplier open(){return new java.util.function.Supplier(){public Object get(){return new java.util.function.Supplier(){public Object get(){return new java.util.function.Supplier(){public Object get(){return new Object[]{alias.get(),Long.valueOf(seed)};}};}};}};}}
  return new Local().open();
 }
}
class GraphDriver {public static void main(String[]args){int rows=0;
 for(final long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(final boolean fail:new boolean[]{false,true}){
 final Object token=new Object();final RuntimeException expected=new IllegalStateException("same");final int[] calls={0};
 java.util.function.Supplier alias=new java.util.function.Supplier(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier outer=GraphRoot.make(alias,seed),middle=(java.util.function.Supplier)outer.get(),leaf=(java.util.function.Supplier)middle.get(),again=(java.util.function.Supplier)middle.get();
 if(calls[0]!=0||leaf==again||leaf.getClass()!=again.getClass())throw new AssertionError("activation,class identity,eager evaluation");
 for(java.util.function.Supplier current:new java.util.function.Supplier[]{leaf,again,leaf}){try{Object[] value=(Object[])current.get();if(fail||value.length!=2||value[0]!=token||((Long)value[1]).longValue()!=seed)throw new AssertionError("captured binding");}catch(RuntimeException actual){if(!fail||actual!=expected)throw new AssertionError("exception identity",actual);}}
 if(calls[0]!=3)throw new AssertionError("evaluation cardinality");
 Class<?> type=leaf.getClass();for(int depth=0;depth<3;depth++){if(!type.isAnonymousClass()||type.getEnclosingMethod()==null||!type.getEnclosingMethod().getName().equals(depth<2?"get":"open"))throw new AssertionError("anonymous lexical edge");type=type.getEnclosingClass();}
 if(type==null||!type.isLocalClass()||type.getEnclosingClass()!=GraphRoot.class||type.getEnclosingMethod()==null||!type.getEnclosingMethod().getName().equals("make"))throw new AssertionError("actual local declaration");rows++;
 }System.out.println(rows+":three-edges:shared-class:wide:deferred:identity");}}
`
	testResourceCompletionFamily(t, source, "GraphRoot", "GraphDriver", "10:three-edges:shared-class:wide:deferred:identity\n")
}
