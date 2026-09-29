package javaclassparser

import "testing"

func TestAdversarialDeclaredFunctionalInterfaceRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "DeclaredAction", `import java.io.*;
import java.util.*;
interface CheckedAction<T> { void accept(T value) throws IOException; }
interface InheritedAction<U> extends CheckedAction<U> {}
interface CheckedMapper<A,B> { B apply(A value) throws IOException; }
class ActionItem {
  final int value;
  static int sum;
  ActionItem(int value) { this.value=value; }
  void record() throws IOException {
    if(value<0) throw new IOException("negative");
    sum+=value;
  }
}
public class DeclaredAction {
  static void visit(List<ActionItem> items,CheckedAction<ActionItem> action) throws IOException {
    for(ActionItem item:items) action.accept(item);
  }
  static void inherited(List<ActionItem> items,InheritedAction<ActionItem> action) throws IOException {
    for(ActionItem item:items) action.accept(item);
  }
  static String map(ActionItem item,CheckedMapper<ActionItem,String> mapper) throws IOException {
    return mapper.apply(item);
  }
  static void probe(List<ActionItem> items) throws IOException {
    visit(items,item -> ActionItem.sum+=item.value);
    CheckedAction<ActionItem> action=ActionItem::record;
    visit(items,action);visit(items,action);
    inherited(items,item -> ActionItem.sum+=item.value*2);
    System.out.print(map(items.get(0),item -> "v"+item.value)+":"+ActionItem.sum+";");
  }
  public static void main(String[] args) throws IOException {
    probe(Arrays.asList(new ActionItem(2),new ActionItem(3)));
    try { probe(Arrays.asList(new ActionItem(4),new ActionItem(-1))); }
    catch(IOException e) { System.out.print(e.getMessage()+":"+ActionItem.sum); }
  }
}`, Precision, Compatibility, "legacy")
}
