package javaclassparser

import "testing"

func TestAdversarialNestedReferenceDispatchRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedReferenceDispatch", `import java.util.*;
class DispatchResult {
  final int code;
  DispatchResult(int code) { this.code=code; }
  DispatchResult(Date value,int mode) { this.code=(int)value.getTime()+mode; }
  DispatchResult(Collection<?> value,int mode) { this.code=300+value.size()+mode; }
}
public class NestedReferenceDispatch {
  final boolean adapters,legacy;
  NestedReferenceDispatch(boolean adapters,boolean legacy) { this.adapters=adapters;this.legacy=legacy; }
  int mode() { return 7; }
  static DispatchResult list(List<?> value,int mode) { return new DispatchResult(100+value.size()+mode); }
  static DispatchResult collection(Collection<?> value,int mode) { return new DispatchResult(200+value.size()+mode); }
  DispatchResult wrap(Object value) {
    if(value==null)return new DispatchResult(-1);
    if(value instanceof String)return new DispatchResult(((String)value).length());
    if(value instanceof Date)return new DispatchResult((Date)value,mode());
    if(value instanceof Collection) {
      if(adapters) {
        if(value instanceof List)return list((List<?>)value,mode());
        return legacy ? new DispatchResult((Collection<?>)value,mode()) : collection((Collection<?>)value,mode());
      }
      return new DispatchResult((Collection<?>)value,mode());
    }
    return new DispatchResult(-2);
  }
  public static void main(String[] args) {
    for(boolean adapters:new boolean[]{false,true})for(boolean legacy:new boolean[]{false,true}) {
      NestedReferenceDispatch w=new NestedReferenceDispatch(adapters,legacy);
      for(Object value:new Object[]{null,"text",new Date(20),Arrays.asList("a","b"),new HashSet<String>(Arrays.asList("a","b","c")),new Object()})
        System.out.print(w.wrap(value).code+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}
