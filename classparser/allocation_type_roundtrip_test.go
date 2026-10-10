package javaclassparser

import "testing"

func TestAdversarialCachedAllocationKeepsConcreteTypeRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CachedAllocation", `interface AllocationResult<T> { String value(); }
class MissingAllocation<T> implements AllocationResult<T> {
  final String name;
  MissingAllocation(String name) { this.name=name;CachedAllocation.calls++; }
  public String value(){return "missing:"+name;}
}
class PresentAllocation<T> implements AllocationResult<T> {
  final T value;
  PresentAllocation(T value) { this.value=value;CachedAllocation.calls++; }
  public String value(){return "present:"+value;}
}
public class CachedAllocation<T> {
  static int calls;
  Object cached;
  final T value;
  CachedAllocation(T value){this.value=value;}
  AllocationResult<T> resolve(int mode) {
    AllocationResult<T> result=cached!=null ? null : (mode==0 ? new MissingAllocation<>("x") : mode==1 ? new MissingAllocation<>("y") : new PresentAllocation<>(value));
    if(result==null) result=(AllocationResult<T>)cached;else cached=result;
    return result;
  }
  public static void main(String[] args) {
    for(int i=0;i<3;i++) {
      CachedAllocation<String> c=new CachedAllocation<>("z");
      AllocationResult<String> a=c.resolve(i),b=c.resolve((i+1)%3);
      System.out.print(a.value()+":"+(a==b)+":"+calls+";");
    }
  }
}`)
}
