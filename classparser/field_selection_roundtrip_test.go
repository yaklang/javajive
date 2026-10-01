package javaclassparser

import "testing"

// Class values belong to their exact declarations inside the traversal. A
// later source pass must not redeclare them based on reused slot names.
func TestAdversarialFieldDeclarationScopeRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FieldSelection", `
import java.lang.reflect.Field;
public class FieldSelection {
  static Field choose(SelectionLink head) {
    if(head==null)return null;
    Field current=head.value;
    for(SelectionLink cursor=head.next;cursor!=null;cursor=cursor.next) {
      Field candidate=SelectionOracle.read(cursor);
      Class<?> left=current.getDeclaringClass();
      Class<?> right=candidate.getDeclaringClass();
      if(left!=right) {
        if(left.isAssignableFrom(right)) current=candidate;
        else if(!right.isAssignableFrom(left)) throw new IllegalArgumentException("ambiguous");
      }
    }
    return current;
  }
  public static void main(String[] args)throws Exception {SelectionOracle.run();}
}
class SelectionLink {
  final Field value;final SelectionLink next;
  SelectionLink(Field value,SelectionLink next){this.value=value;this.next=next;}
}
class SelectionBase {public int value;}
class SelectionChild extends SelectionBase {public int value;}
class SelectionSibling extends SelectionBase {public int value;}
class SelectionOther {public int value;}
class SelectionOracle {
  static StringBuilder trace;static int calls,failAt;
  static Field read(SelectionLink link) {
    trace.append(link.value==null ? "null" : link.value.getDeclaringClass().getSimpleName()).append(';');
    if(++calls==failAt)throw new IllegalStateException("read:"+calls);
    return link.value;
  }
  static void run()throws Exception {
    Field[] fields={SelectionBase.class.getDeclaredField("value"),SelectionChild.class.getDeclaredField("value"),SelectionSibling.class.getDeclaredField("value"),SelectionOther.class.getDeclaredField("value"),null};
    for(int a=-1;a<fields.length;a++)for(int b=0;b<fields.length;b++)for(int c=0;c<fields.length;c++)for(int failure:new int[]{0,1,2,3}) {
      SelectionLink head=a<0 ? null : new SelectionLink(fields[a],new SelectionLink(fields[b],new SelectionLink(fields[c],null)));
      trace=new StringBuilder();calls=0;failAt=failure;String result;
      try {Field selected=FieldSelection.choose(head);result=selected==null ? "null" : selected.getDeclaringClass().getSimpleName()+":"+selected.getName();}
      catch(Throwable e){result=e.getClass().getSimpleName();}
      System.out.println(a+":"+b+":"+c+":"+failure+":"+result+":"+calls+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
