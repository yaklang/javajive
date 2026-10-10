package javaclassparser

import "testing"

func TestAdversarialCalleeWildcardScopeRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CalleeWildcardScope", `import java.util.*;
class OtherListOwner {
  static int count(List<?> input) {return 100+input.size();}
}
class ExternalScopeOwner<V> {
  static ExternalScopeOwner<String> make() {return new ExternalScopeOwner<>();}
  public int select(List<? extends CharSequence> input) {return 500+input.size();}
  public int select(Collection<? extends Number> input) {return 600+input.size();}
}
public class CalleeWildcardScope<E extends Number> {
  private <T> T first(List<? extends T> input) {return input.get(0);}
  private <K> int count(List<? extends K> input) {return input.size();}
  String head(Object value) {return (String)this.first((List<?>)value);}
  int size(Object value) {return this.count((List<?>)value);}
  int other(Object value) {return OtherListOwner.count((List<?>)value);}
  private <E extends CharSequence> E shadowFirst(List<? extends E> input) {return input.get(0);}
  String shadow(Object value) {return (String)this.shadowFirst((List<String>)value);}
  private int overload(List<? extends CharSequence> input) {return 200+input.size();}
  private int overload(Collection<? extends Number> input) {return 400+input.size();}
  private int overload(Object input) {return 300;}
  int selected(Object value) {return this.overload((List<String>)value);}
  int subtype(Object value) {
    ArrayList<String> copy=new ArrayList<>((List<String>)value);
    return this.overload(copy);
  }
  int external(Object value) {return ExternalScopeOwner.make().select((List<String>)value);}
  int empty() {return this.overload(Collections.<Integer>emptyList());}
  private static <V> List<V> blank() {return Collections.emptyList();}
  int customEmpty() {return this.overload(CalleeWildcardScope.<Integer>blank());}
  public static void main(String[] args) {
    CalleeWildcardScope<Integer> worker=new CalleeWildcardScope<>();
    Object[] inputs={Arrays.asList("a","b"),Arrays.asList(1,2),Collections.emptyList(),null,"wrong"};
    for(Object value:inputs)for(int operation=0;operation<9;operation++) {
      try {Object out;
        switch(operation) {case 0:out=worker.head(value);break;case 1:out=worker.size(value);break;
          case 2:out=worker.other(value);break;case 3:out=worker.shadow(value);break;
          case 4:out=worker.selected(value);break;case 5:out=worker.subtype(value);break;
          case 6:out=worker.external(value);break;case 7:out=worker.empty();break;default:out=worker.customEmpty();}
        System.out.print(out+";");
      }catch(RuntimeException ex){System.out.print(ex.getClass().getSimpleName()+";");}
    }
  }
}`, Precision, Compatibility, "legacy")
}
