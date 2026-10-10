package javaclassparser

import "testing"

// Both short-circuit routes enter the same effectful value leaf. The shared
// call must be evaluated once after the original || / && predicate, including
// its short-circuit order; a value leaf is not a second condition operand.
func TestAdversarialSharedCallValueShortCircuitRoundTrip(t *testing.T) {
	rebuild := t17RebuildRunner(t)
	const main = "SharedCallValue"
	const source = `public class SharedCallValue {
  static StringBuilder trace;
  static Object SINGLE=new Object();
  Object value;
  static Object singleton() { trace.append("S"); return SINGLE; }
  static Object bounded(Object value) { trace.append("B"); if(value==null) throw new IllegalArgumentException(); return value; }
  static boolean mark(char ch,boolean value) { trace.append(ch);return value; }
  Object choose() {
    Object result=(value==null || value==singleton()) ? singleton():bounded(value);
    return result;
  }
  static Object and(boolean a,boolean b) {
    return mark('a',a) && mark('b',b) ? singleton():bounded(SINGLE);
  }
  static Object or(boolean a,boolean b) {
    return mark('a',a) || mark('b',b) ? singleton():bounded(SINGLE);
  }
  public static void main(String[] args) {
    SharedCallValue p=new SharedCallValue();
    for(int i=0;i<3;i++) {
      trace=new StringBuilder();p.value=i==0?null:i==1?SINGLE:new Object();
      Object result=p.choose();System.out.print((result==SINGLE)+":"+trace+";");
    }
    for(int bits=0;bits<4;bits++) {
      trace=new StringBuilder();and((bits&1)!=0,(bits&2)!=0);System.out.print(trace+";");
      trace=new StringBuilder();or((bits&1)!=0,(bits&2)!=0);System.out.print(trace+";");
    }
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		want, raw := functionalFixture(t, main, source, debug)
		for _, mode := range []DecompileMode{Precision, Compatibility} {
			result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
			if err != nil {
				t.Fatal(err)
			}
			if err := rebuild("8", main, result.Source, want); err != nil {
				t.Fatalf("shared value %s/%s: %v\n%s", mode, debug, err, result.Source)
			}
		}
	}
}
