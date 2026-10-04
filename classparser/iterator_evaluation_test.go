package javaclassparser

import "testing"

func TestAdversarialIteratorElementEvaluatedOnceRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "IteratorEvaluation", `import java.util.*;
public class IteratorEvaluation {
  static String primary(Collection<List<String>> values) {
    List<String> result=new ArrayList<>();
    Iterator<List<String>> iterator=values.iterator();
    while(iterator.hasNext()) {
      String first;
      if((first=iterator.next().get(0))!=null && !first.isEmpty()) result.add(first);
    }
    return result.toString();
  }
  static int sum(Collection<int[]> values) {
    int sum=0;
    Iterator<int[]> iterator=values.iterator();
    while(iterator.hasNext()) {
      int[] entry;
      int length=(entry=iterator.next()).length;
      for(int i=0;i<length;i++) sum+=entry[i];
    }
    return sum;
  }
  public static void main(String[] args) {
    System.out.print(primary(Arrays.asList(Arrays.asList("a"),Arrays.asList(""),Arrays.asList((String)null),Arrays.asList("b")))+":"+sum(Arrays.asList(new int[]{1,2},new int[]{3,4,5})));
  }
}`, Precision, Compatibility, "legacy")
}
