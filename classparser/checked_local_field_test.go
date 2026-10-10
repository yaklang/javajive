package javaclassparser

import "testing"

func TestAdversarialCheckedLocalBeforeShortCircuitRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "CheckedLocalField", `public class CheckedLocalField {
  final int value;
  CheckedLocalField(int value) { this.value=value; }
  boolean matches(Object other,boolean enabled) {
    CheckedLocalField checked=(CheckedLocalField)other;
    return enabled && value==checked.value;
  }
  boolean guarded(Object other,boolean enabled) {
    return enabled && value==((CheckedLocalField)other).value;
  }
  static int choose(Object other,boolean enabled) {
    return enabled ? ((CheckedLocalField)other).value : -1;
  }
  static boolean guardedByType(Object other) {
    return other instanceof CheckedLocalField && ((CheckedLocalField)other).value==7;
  }
  static void probe(Object other,boolean enabled) {
    try { System.out.print(new CheckedLocalField(7).matches(other,enabled)+";"); }
    catch(ClassCastException e) { System.out.print("cast;"); }
    catch(NullPointerException e) { System.out.print("null;"); }
    try { System.out.print(choose(other,enabled)+";"); }
    catch(ClassCastException e) { System.out.print("cast;"); }
    catch(NullPointerException e) { System.out.print("null;"); }
    System.out.print(guardedByType(other)+";");
    try { System.out.print(new CheckedLocalField(7).guarded(other,enabled)+";"); }
    catch(ClassCastException e) { System.out.print("cast;"); }
    catch(NullPointerException e) { System.out.print("null;"); }
  }
  public static void main(String[] args) {
    probe(new CheckedLocalField(7),true);
    probe(new CheckedLocalField(8),true);
    probe(new CheckedLocalField(7),false);
    probe(new Object(),true);
    probe(new Object(),false);
    probe(null,true);
    probe(null,false);
  }
}`, Precision, Compatibility, "legacy")
}
