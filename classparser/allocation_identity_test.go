package javaclassparser

import "testing"

func TestAdversarialAllocationIdentitySurvivesMutatingCallRoundTrip(t *testing.T) {
	// Assignment expressions force javac to duplicate the initialized object
	// after invokespecial. Separate assignment and mutation statements only
	// exercise astore/aload, which did not expose the lost allocation identity.
	roundTripGenericFlow(t, "AllocationIdentity", `import java.util.*;
class MutableBits {
  static int allocations;
  int value;
  MutableBits() {allocations++;}
  void add(int bits) {value|=bits;}
}
public class AllocationIdentity {
  static int bits(boolean enabled) {
    if(!enabled) throw new IllegalArgumentException();
    MutableBits value;
    (value=new MutableBits()).add(15);
    return value.value;
  }
  static List<String> list(boolean enabled) {
    if(!enabled) return null;
    ArrayList<String> value;
    (value=new ArrayList<String>(1)).add("a");
    return value;
  }
  static List<List<String>> nested(int count) {
    List<List<String>> out=new ArrayList<>();
    for(int i=0;i<count;i++) {
      List<String> value;
      (value=new ArrayList<String>(1)).add("x"+i);
      out.add(value);
    }
    return out;
  }
  public static void main(String[] args) {
    System.out.print(bits(true)+":"+MutableBits.allocations+":"+list(true)+":"+nested(3));
  }
}`, Precision, Compatibility, "legacy")
}
