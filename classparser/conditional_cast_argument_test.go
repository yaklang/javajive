package javaclassparser

import "testing"

func TestAdversarialConditionalCastArgumentRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalCastArgument", `import java.util.*;
import java.util.regex.Pattern;
class CountedItems extends AbstractList<CharSequence> {
  static int reads;
  CharSequence value;
  CountedItems(CharSequence value) {this.value=value;}
  public int size() {return 1;}
  public CharSequence get(int index) { reads++; return value; }
}
public class ConditionalCastArgument {
  static final Pattern SPLIT=Pattern.compile(",");
  static StringBuilder trace=new StringBuilder();
  static int before() {trace.append("before;");return 1;}
  static String join(int first,CharSequence last) {trace.append("join;");return first+":"+last;}
  static String invoke(boolean take,Object value) {return take ? join(before(),(CharSequence)value) : "skip";}
  static String[] parse(List<CharSequence> items) {
    String[] result=items==null ? null : SPLIT.split(items.get(0));
    if(result!=null && result.length!=2) result=null;
    return result;
  }
  public static void main(String[] args) {
    System.out.print(Arrays.toString(parse(null))+":"+Arrays.toString(parse(new CountedItems("a,b")))+":"+Arrays.toString(parse(new CountedItems("a")))+":"+CountedItems.reads);
    System.out.print(":"+invoke(false,new Object())+":"+invoke(true,"ok"));
    try {invoke(true,new Object());} catch(ClassCastException e) {System.out.print(":cast");}
    System.out.print(":"+invoke(true,null)+":"+trace);
  }
}`)
}
