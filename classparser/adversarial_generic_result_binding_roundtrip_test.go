package javaclassparser

import "testing"

// The physical invocation chooses the bounded generic declaration. Its source
// argument views and solved local result must remain one binding decision:
// descriptor pins must not erase the result into an unassignable first bound.
func TestAdversarialGenericResultBindingPreservesSolvedLocalConsumer(t *testing.T) {
	fixture := `
interface GenericResultOwnerToken<T extends GenericResultOwnerToken<T>> {int id();}
class GenericResultOwnerValue implements GenericResultOwnerToken<GenericResultOwnerValue> {final int n;GenericResultOwnerValue(int n){this.n=n;}public int id(){return n;}}
class GenericResultOwnerOther implements GenericResultOwnerToken<GenericResultOwnerOther> {public int id(){return 73;}}
class GenericResultOwnerBox<T extends GenericResultOwnerToken<T>> {final T value;GenericResultOwnerBox(T v){value=v;}
 static <T extends GenericResultOwnerToken<T>> T dot(GenericResultOwnerBox<T> a,GenericResultOwnerBox<T> b){GenericResultOwnerOps.trace+="D";return a.value;}
 static <T extends GenericResultOwnerToken<T>> T dot(GenericResultOwnerBox<T> a,GenericResultOwnerValue b){GenericResultOwnerOps.trace+="V";return a.value;}
 static GenericResultOwnerToken dot(GenericResultOwnerBox a,Object b){GenericResultOwnerOps.trace+="X";return b==null?null:((GenericResultOwnerBox)b).value;}
 static <T extends GenericResultOwnerToken<T>> T between(GenericResultOwnerBox<T> a,GenericResultOwnerBox<T> b){T result=dot(a,b);if(result==null)throw new IllegalStateException("empty");return result;}
 static <T extends GenericResultOwnerToken<T>> T between(GenericResultOwnerBox<T> a,GenericResultOwnerValue b){T result=dot(a,b);if(result==null)throw new IllegalStateException("empty");return result;}
 static <T extends GenericResultOwnerToken<T>> GenericResultOwnerToken widened(GenericResultOwnerBox<T> a,GenericResultOwnerBox<T> b,GenericResultOwnerToken other,boolean replace){GenericResultOwnerToken result=dot(a,b);if(replace)result=other;return result;}
}
class GenericResultOwnerOps {static String trace="";
 static <N extends GenericResultOwnerToken<N>> N choose(GenericResultOwnerBox<N> a,GenericResultOwnerBox<N> b){trace+="G";return a.value;}
 static GenericResultOwnerToken choose(GenericResultOwnerBox a,Object b){trace+="X";return b==null?null:((GenericResultOwnerBox)b).value;}
}
public abstract class GenericResultOwner<E extends GenericResultOwnerToken<E>> {
 final E selected;
 GenericResultOwner(GenericResultOwnerBox<E> a,GenericResultOwnerBox<E> b){E result=GenericResultOwnerBox.dot(a,b);if(result==null)throw new IllegalStateException("empty");selected=result;}
 E instance(GenericResultOwnerBox<E>a,GenericResultOwnerBox<E>b){E result=GenericResultOwnerOps.choose(a,b);if(result==null)throw new IllegalStateException("empty");return result;}
 static <T extends GenericResultOwnerToken<T>> T method(GenericResultOwnerBox<T>a,GenericResultOwnerBox<T>b){T result=GenericResultOwnerOps.choose(a,b);if(result==null)throw new IllegalStateException("empty");return result;}
 protected abstract <T>T attribute(String key,Class<T> type);
 protected Object attribute(Object key,Object type){GenericResultOwnerOps.trace+="X";return key;}
 <T>T required(String key,Class<T> type){T result=this.attribute(key,type);if(result==null)throw new IllegalStateException("empty");return result;}
}
class GenericResultDriver {
 public static void main(String[]args){GenericResultOwnerValue a=new GenericResultOwnerValue(-42),b=new GenericResultOwnerValue(17);GenericResultOwnerBox<GenericResultOwnerValue> left=new GenericResultOwnerBox<>(a),right=new GenericResultOwnerBox<>(b);
  GenericResultOwnerOps.trace="";
  GenericResultOwner<GenericResultOwnerValue> owner=new GenericResultOwner<GenericResultOwnerValue>(left,right){protected <T>T attribute(String key,Class<T>type){GenericResultOwnerOps.trace+="A";return type.cast(key);}};
  if(owner.selected!=a||GenericResultOwnerBox.between(left,right)!=a||GenericResultOwnerBox.between(left,b)!=a||!GenericResultOwnerOps.trace.equals("DDV"))throw new AssertionError("generic owner/shadowed formal/constructor result");
  GenericResultOwnerOther other=new GenericResultOwnerOther();if(GenericResultOwnerBox.widened(left,right,other,false)!=a||GenericResultOwnerBox.widened(left,right,other,true)!=other)throw new AssertionError("whole-web widened consumer");
  GenericResultOwnerOps.trace="";if(owner.instance(left,right)!=a||GenericResultOwner.method(left,right)!=a||!GenericResultOwnerOps.trace.equals("GG"))throw new AssertionError("identity/physical overload");
  GenericResultOwnerOps.trace="";if(owner.required("payload",String.class)!="payload"||!GenericResultOwnerOps.trace.equals("A"))throw new AssertionError("local inference/overload");
  GenericResultOwnerOps.trace="";try{owner.required("payload",Integer.class);throw new AssertionError("missing original Class.cast");}catch(ClassCastException e){if(!GenericResultOwnerOps.trace.equals("A"))throw new AssertionError("cast order");}
  GenericResultOwnerOps.trace="";try{owner.required("payload",null);throw new AssertionError("missing original null");}catch(NullPointerException e){if(!GenericResultOwnerOps.trace.equals("A"))throw new AssertionError("null order");}
  GenericResultOwnerOps.trace="";try{owner.required(null,String.class);throw new AssertionError("missing empty result");}catch(IllegalStateException e){if(!GenericResultOwnerOps.trace.equals("A"))throw new AssertionError("empty order");}
  System.out.println("11:generic-result:class-and-method-formals:physical-overloads:identity:original-checks");
 }
}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"GenericResultOwner.java": fixture}, "GenericResultOwner", "GenericResultDriver", "11:generic-result:class-and-method-formals:physical-overloads:identity:original-checks\n")
}
