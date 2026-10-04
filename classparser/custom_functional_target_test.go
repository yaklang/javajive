package javaclassparser

import "testing"

func TestAdversarialDeclaredFunctionalInterfaceRoundTrip(t *testing.T) {
	t.Parallel()
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

func TestAdversarialErasedFunctionalArgumentsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedActions", `import java.util.*;
interface GroupAction<T> { void apply(T value); }
interface AssertMaker<T,A extends SelfAssert<A,T>> { A make(T value); }
class SelfAssert<S extends SelfAssert<S,T>,T> { final T value; SelfAssert(T v) { value=v; } }
class TinyAssert<T> extends SelfAssert<TinyAssert<T>,T> { TinyAssert(T v) { super(v); } }
public class ErasedActions {
  static final AssertMaker<String,TinyAssert<String>> FACTORY=TinyAssert::new;
  static void groups(List<Collection<String>> values,GroupAction<Collection<String>> action) {
    for(Collection<String> group:values) action.apply(group);
  }
  static void empty(List<Collection<String>> values) { groups(values,Collection::clear); }
  public static void main(String[] args) {
    List<Collection<String>> values=new ArrayList<>();
    values.add(new ArrayList<>(Arrays.asList("one","two")));
    values.add(new HashSet<>(Arrays.asList("three")));
    empty(values);
    System.out.print(values.get(0).size()+":"+values.get(1).size()+":"+FACTORY.make("ok").value);
  }
}`, Precision, Compatibility, "legacy")
}
