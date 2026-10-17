package javaclassparser

import "testing"

func TestAdversarialLocalMemberAllocationKeepsHiddenReceiverAndEvaluation(t *testing.T) {
	const source = `
class AllocationRoot {class Layer {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(Layer.this.live)};}}
 java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local implements java.util.function.Supplier<Object>{public Object get(){return new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(fail)throw new AssertionError("lost argument exception");}catch(RuntimeException actual){if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail){if(made.getClass()!=AllocationRoot.Layer.Piece.class)throw new AssertionError("member identity");layer.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=3||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-member-allocation:nullable-hidden-receiver:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-member-allocation:nullable-hidden-receiver:argument-priority:live\n")
}

func TestAdversarialLocalGrandparentAllocationKeepsReceiverFailurePriority(t *testing.T) {
	const source = `
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local implements java.util.function.Supplier<Object>{public Object get(){return new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-grandparent-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-grandparent-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalAllocationTypeVariableKeepsOriginalMemberBinding(t *testing.T) {
	const source = `
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<Piece> implements java.util.function.Supplier<Object>{public Object get(){return new AllocationRoot.Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-grandparent-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-grandparent-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalAllocationInheritedTypeKeepsOriginalMemberBinding(t *testing.T) {
	const source = `
class ShadowBase {static class Piece{Piece(Object token){throw new AssertionError("selected inherited decoy");}}}
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local extends ShadowBase implements java.util.function.Supplier<Object>{public Object get(){return new AllocationRoot.Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-grandparent-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-grandparent-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalExplicitEnclosingCheckKeepsArgumentPriority(t *testing.T) {
	const source = `
class AllocationRoot {class Layer {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(Layer.this.live)};}}
 java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local implements java.util.function.Supplier<Object>{public Object get(){return Layer.this.new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Layer.Piece.class)throw new AssertionError("member identity");layer.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":explicit-local-member-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:explicit-local-member-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalMemberAllocationKeepsDistinctNullableOwnerEdges(t *testing.T) {
	const source = `
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local implements java.util.function.Supplier<Object>{public Object get(){return new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nilRoot:new boolean[]{false,true})for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();java.lang.reflect.Constructor<?> layerCtor=AllocationRoot.Layer.class.getDeclaredConstructor(AllocationRoot.class);layerCtor.setAccessible(true);AllocationRoot.Layer layer=(AllocationRoot.Layer)layerCtor.newInstance(new Object[]{nilRoot?null:root});final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nilRoot||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nilRoot)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-member-allocation:distinct-null-edges:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "16:local-member-allocation:distinct-null-edges:argument-priority:live\n")
}

func TestAdversarialGenericLocalMemberAllocationKeepsOriginalTypeView(t *testing.T) {
	const source = `
class AllocationRoot<T> {volatile int live;
 class Piece<U> implements java.util.function.Supplier<Object[]> {final U token;Piece(U token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local implements java.util.function.Supplier<Object>{public Object get(){return new Piece<>(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":generic-local-member-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:generic-local-member-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalExplicitEnclosingWithAllocationRootShadow(t *testing.T) {
	const source = `
class AllocationRoot {class Layer {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(Layer.this.live)};}}
 java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<AllocationRoot> implements java.util.function.Supplier<Object>{public Object get(){return Layer.this.new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Layer.Piece.class)throw new AssertionError("member identity");layer.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":explicit-local-member-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:explicit-local-member-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalExplicitEnclosingWithPieceShadow(t *testing.T) {
	const source = `
class AllocationRoot {class Layer {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(Layer.this.live)};}}
 java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<Piece> implements java.util.function.Supplier<Object>{public Object get(){return Layer.this.new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Layer.Piece.class)throw new AssertionError("member identity");layer.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":explicit-local-member-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:explicit-local-member-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalExplicitEnclosingWithLayerShadow(t *testing.T) {
	const source = `
class AllocationRoot {class Layer {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(Layer.this.live)};}}
 java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<Layer> implements java.util.function.Supplier<Object>{public Object get(){return AllocationRoot.Layer.this.new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Layer.Piece.class)throw new AssertionError("member identity");layer.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":explicit-local-member-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:explicit-local-member-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalImplicitAllocationKeepsShadowedRootClassRole(t *testing.T) {
	const source = `
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<AllocationRoot> implements java.util.function.Supplier<Object>{public Object get(){return new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-grandparent-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-grandparent-allocation:receiver-priority:argument-priority:live\n")
}

func TestAdversarialLocalAllocationSeparatesUsedFormalAndRootClassRole(t *testing.T) {
	const source = `
class AllocationRoot {volatile int live;
 class Piece implements java.util.function.Supplier<Object[]> {final Object token;Piece(Object token){this.token=token;}public Object[] get(){return new Object[]{token,Integer.valueOf(AllocationRoot.this.live)};}}
 class Layer {java.util.function.Supplier<Object> make(final java.util.function.Supplier<Object> arg){class Local<AllocationRoot> implements java.util.function.Supplier<Object>{AllocationRoot echo(AllocationRoot value){return value;}public Object get(){return new Piece(arg.get());}}return new Local();}
 java.util.function.Supplier<Object> unrelated(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return token;}};}
}}
class AllocationDriver {public static void main(String[] args)throws Exception{int rows=0;
 for(boolean nil:new boolean[]{false,true})for(final boolean fail:new boolean[]{false,true})for(boolean nullToken:new boolean[]{false,true}){
 AllocationRoot root=new AllocationRoot();AllocationRoot.Layer layer=root.new Layer();final Object token=nullToken?null:new Object();final int[] calls={0};final RuntimeException expected=new IllegalStateException("same");java.util.function.Supplier<Object> arg=new java.util.function.Supplier<Object>(){public Object get(){calls[0]++;if(fail)throw expected;return token;}};
 java.util.function.Supplier<Object> ordinary=layer.make(arg);Class<?> local=ordinary.getClass();java.lang.reflect.Constructor<?>[] ctors=local.getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("local ctor");java.lang.reflect.Constructor<?> ctor=ctors[0];ctor.setAccessible(true);Class<?>[] ports=ctor.getParameterTypes();Object[] vals=new Object[ports.length];for(int i=0;i<ports.length;i++){if(ports[i]==AllocationRoot.Layer.class)vals[i]=nil?null:layer;else if(ports[i]==java.util.function.Supplier.class)vals[i]=arg;else throw new AssertionError("unknown port");}
 java.util.function.Supplier<Object> supplied=(java.util.function.Supplier<Object>)ctor.newInstance(vals);if(calls[0]!=0)throw new AssertionError("eager call");java.lang.reflect.Method echo=local.getDeclaredMethod("echo",Object.class);echo.setAccessible(true);java.lang.reflect.Type resultType=echo.getGenericReturnType();if(!(resultType instanceof java.lang.reflect.TypeVariable)||((java.lang.reflect.TypeVariable<?>)resultType).getGenericDeclaration()!=local||!((java.lang.reflect.TypeVariable<?>)resultType).getName().equals("AllocationRoot")||!echo.getGenericParameterTypes()[0].equals(resultType)||echo.invoke(supplied,new Object[]{token})!=token)throw new AssertionError("formal declaration identity");
 for(int i=0;i<3;i++){Object made=null;try{made=supplied.get();if(nil||fail)throw new AssertionError("lost receiver or argument exception");}catch(RuntimeException actual){if(nil){if(!(actual instanceof NullPointerException))throw new AssertionError("receiver priority",actual);}else if(!fail||actual!=expected)throw new AssertionError("argument priority",actual);}
 if(!fail&&!nil){if(made.getClass()!=AllocationRoot.Piece.class)throw new AssertionError("member identity");root.live=70+i;try{Object[] result=((java.util.function.Supplier<Object[]>)made).get();if(nil||result[0]!=token||((Integer)result[1]).intValue()!=70+i)throw new AssertionError("live receiver state");}catch(NullPointerException actual){if(!nil)throw actual;}}}
 if(calls[0]!=(nil?0:3)||!local.isLocalClass()||local.getEnclosingClass()!=AllocationRoot.Layer.class||!local.getEnclosingMethod().getName().equals("make"))throw new AssertionError("deferred count or lexical identity");rows++;
 }System.out.println(rows+":local-grandparent-allocation:receiver-priority:argument-priority:live");}}
`
	testResourceCompletionFamily(t, source, "AllocationRoot", "AllocationDriver", "8:local-grandparent-allocation:receiver-priority:argument-priority:live\n")
}
