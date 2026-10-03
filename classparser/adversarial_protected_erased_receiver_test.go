package javaclassparser

import "testing"

func TestAdversarialProtectedGenericReceiverAcrossPackages(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowSources(t, "consumer.AccessDriver", `package consumer;
import parent.Parent;
public class AccessDriver<T> extends Parent<T> {
 public void update(T value) { setState(produce(value)); }
 public void updateAgain(T value) { setState(produce(produce(value).value)); }
 public static void main(String[] args) {
  for (int mode=0;mode<4;mode++) {
   AccessDriver<String> driver=new AccessDriver<String>();
   String input=mode==0?null:new String("value");
   driver.failureMode=mode;
   try {
    driver.update(input); driver.updateAgain(input);
    if(driver.last.value!=input || driver.trace!=12112) throw new AssertionError("effects");
    System.out.println(mode+":ok:"+driver.trace);
   } catch (RuntimeException failure) {
    if(failure!=driver.failure) throw new AssertionError("identity");
    int expected=mode==1?1:mode==2?12:1211;
    if(driver.trace!=expected) throw new AssertionError("trace:"+driver.trace);
    System.out.println(mode+":failure:"+driver.trace);
   }
  }
 }
}`, map[string]string{"parent/Parent.java": `package parent;
public class Parent<T> {
 public State<T> last;
 public int trace, failureMode;
 public final RuntimeException failure=new IllegalArgumentException("sentinel");
 protected void setState(State<T> state) {
  trace=trace*10+2;
  if(failureMode==2) throw failure;
  last=state;
 }
 protected State<T> produce(T value) {
  trace=trace*10+1;
  if(failureMode==1 || failureMode==3 && trace==1211) throw failure;
  return new State<T>(value);
 }
}`, "parent/State.java": `package parent;
public class State<V> { public final V value; public State(V value){this.value=value;} }
`}, nil, nil, false, Precision, Compatibility, "legacy")
}
