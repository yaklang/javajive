package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Helpers stay on the original classpath so their Signature attributes provide
// independent declaration evidence. Only the consumer is replaced by generated
// source; its output must match the original across both modes and debug forms.
func roundTripGenericFlow(t *testing.T, main, source string, modes ...DecompileMode) {
	t.Helper()
	if len(modes) == 0 {
		modes = []DecompileMode{Precision, Compatibility}
	}
	javac, java := t04Tools(t)
	simple := main[strings.LastIndex(main, ".")+1:]
	for _, debug := range []string{"-g", "-g:none"} {
		dir := t.TempDir()
		path := filepath.Join(dir, simple+".java")
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("original: %v\n%s", err, out)
		}
		want := t04RunJava(t, java, dir, main)
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(main, ".", "/"))+".class"))
		if err != nil {
			t.Fatal(err)
		}
		resolve := func(name string) ([]byte, bool) {
			b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
			return b, e == nil
		}
		// Modes often produce byte-for-byte identical source. Reuse only that
		// compilation within this exact fixture/classpath/debug variant; still
		// decompile and execute each mode independently against the original.
		compiled := map[string]string{}
		for _, mode := range modes {
			var result DecompileResult
			var err error
			if mode == "legacy" {
				result.Source, err = DecompileWithResolver(raw, resolve)
			} else {
				result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
			}
			if err != nil {
				t.Fatal(err)
			}
			rebuilt, cached := compiled[result.Source]
			if !cached {
				rebuilt = t.TempDir()
				src := filepath.Join(rebuilt, simple+".java")
				if err := os.WriteFile(src, []byte(result.Source), 0644); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, src)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("rebuild %s/%s: %v\n%s\n%s", mode, debug, err, out, result.Source)
				}
				compiled[result.Source] = rebuilt
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, main); got != want {
				t.Fatalf("%s/%s: got %q want %q\n%s", mode, debug, got, want, result.Source)
			}
		}
	}
}

func TestT19GenericFieldChainRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "FieldChain", `import java.util.function.*;
class ChainBox<Y> {
  final Function<? super Y,? extends Y> fn;
  ChainBox(Function<? super Y,? extends Y> fn) { this.fn=fn; }
}
class ChainHolder<X> {
  final ChainBox<X> box;
  ChainHolder(Function<? super X,? extends X> fn) { box=new ChainBox<>(fn); }
}
public class FieldChain<T> {
  final ChainHolder<T> holder;
  FieldChain(Function<? super T,? extends T> fn) { holder=new ChainHolder<>(fn); }
  T read(Object value) { return holder.box.fn.apply((T)value); }
  public static void main(String[] args) {
    FieldChain<String> s=new FieldChain<>(x -> x==null ? "null" : x+"!");
    FieldChain<Integer> n=new FieldChain<>(x -> x+1);
    System.out.print(s.read("a")+":"+s.read(null)+":"+n.read(9));
  }
}`)
}

func TestT19GenericWrapperAndFactoryRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "GenericFactoryFlow", `import java.util.*;
import java.util.function.*;
import java.util.concurrent.*;
class FunctionWrapper {
  <A,B,R> BiFunction<? super A,? super B,? extends R> wrap(BiFunction<? super A,? super B,? extends R> fn) { return fn; }
}
public class GenericFactoryFlow<K,V> {
  final FunctionWrapper owner=new FunctionWrapper();
  V apply(BiFunction<? super K,? super V,? extends V> fn,K key,Object value) {
    return owner.wrap(fn).apply(key,(V)value);
  }
  CompletableFuture<Map<K,V>> defer(Map<K,V> values,Executor executor) {
    Supplier<Map<K,V>> task=() -> values;
    return CompletableFuture.supplyAsync(task,executor);
  }
  public static void main(String[] args) {
    GenericFactoryFlow<String,Integer> f=new GenericFactoryFlow<>();
    Map<String,Integer> m=new HashMap<>();
    m.put("x",7);
    System.out.print(f.apply((k,v)->v+k.length(),"abc",4)+":"+f.defer(m,Runnable::run).join().get("x"));
  }
}`)
}

// The original bytecode erases explicit raw casts. Recover from invariant
// declaration conflicts, independently of neighboring argument casts or names.
func TestT19InvariantArgumentBridgeRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "InvariantArguments", `import java.util.*;
class ArgumentMetadata<A,B> {
  A key;
  ArgumentMetadata(A key) { this.key=key; }
}
public class InvariantArguments {
  static StringBuilder trace=new StringBuilder();
  static <U> void emit(boolean key,U value,ArgumentMetadata<Boolean,U> meta) {
    trace.append(key).append(":").append(value).append(":").append(meta.key).append(";");
  }
  static <U> void batch(ArgumentMetadata<Boolean,U> meta,Map<Boolean,U> values) {
    trace.append(values.size()).append(":").append(meta.key);
  }
  static <K,V> void dispatch(ArgumentMetadata<K,V> meta,Map<K,V> values) {
    V value=values.get(Boolean.FALSE);
    emit(false,value,(ArgumentMetadata)meta);
    batch((ArgumentMetadata)meta,(Map)values);
  }
  public static void main(String[] args) {
    Map<Boolean,Integer> values=new HashMap<>();
    values.put(false,3);
    dispatch(new ArgumentMetadata<Boolean,Integer>(true),values);
    System.out.print(trace);
  }
}`)
}

// Generic factories can return a subtype of a generic map value. Their
// arguments must be inferred structurally; no class or local spelling is fixed.
func TestT19GenericFactorySubtypeRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "FactorySubtype", `import java.util.*;
interface Delta<U> { String text(); }
class DeltaImpl<V> implements Delta<V> {
  V left,right;
  DeltaImpl(V left,V right) { this.left=left;this.right=right; }
  static <V> DeltaImpl<V> create(V left,V right) { return new DeltaImpl<>(left,right); }
  public String text() { return left+":"+right; }
}
public class FactorySubtype {
  static <K,V> void compare(Map<K,V> left,Map<K,V> right,Map<K,Delta<V>> out) {
    for (Map.Entry<K,V> entry:left.entrySet()) {
      Object key=entry.getKey();
      Object value=entry.getValue();
      V other=right.remove(key);
      out.put((K)key,DeltaImpl.create((V)value,other));
    }
  }
  public static void main(String[] args) {
    Map<String,Integer> left=new HashMap<>(),right=new HashMap<>();
    left.put("x",2);right.put("x",5);
    Map<String,Delta<Integer>> out=new HashMap<>();
    compare(left,right,out);
    System.out.print(out.get("x").text()+":"+right.size());
  }
}`)
}

func TestT19AnnotatedLoopCaptureRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "AnnotatedCapture", `import java.lang.annotation.*;
import java.util.*;
import java.util.function.*;
@Target(ElementType.TYPE_USE) @Retention(RetentionPolicy.RUNTIME) @interface FlowMark {}
public class AnnotatedCapture<T> {
  @FlowMark T select(T value,int count) {
    T pending=null;
    List<Supplier<T>> history=new ArrayList<>();
    for (int i=0;i<count;i++) {
      if (pending==null) pending=value;
      T snapshot=pending;
      Supplier<T> task=() -> snapshot;
      history.add(task);
      pending=null;
    }
    return history.get(count-1).get();
  }
  public static void main(String[] args) {
    System.out.print(new AnnotatedCapture<String>().select("ok",3));
  }
}`)
}

func TestT19MixedIteratorDefinitionsRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "MixedIteratorDefinitions", `import java.util.*;
public class MixedIteratorDefinitions<T> {
  StringBuilder trace=new StringBuilder();
  void add(T value) { trace.append(value).append(";"); }
  void collect(boolean pairs,Iterable<? extends T> values,Collection<Map.Entry<T,Integer>> entries) {
    if(pairs) {
      Iterator<Map.Entry<T,Integer>> it=entries.iterator();
      while(it.hasNext()) add(it.next().getKey());
    } else {
      Iterator<? extends T> it=values.iterator();
      while(it.hasNext()) add(it.next());
    }
  }
  public static void main(String[] args) {
    MixedIteratorDefinitions<String> c=new MixedIteratorDefinitions<>();
    List<Map.Entry<String,Integer>> entries=Arrays.asList(new AbstractMap.SimpleEntry<>("p",1));
    c.collect(true,Arrays.asList("a","b"),entries);
    c.collect(false,Arrays.asList("a","b"),entries);
    System.out.print(c.trace);
  }
}`)
}

func TestT19RawAndGenericIteratorSlotRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "RawIteratorSlot", `import java.util.*;
class SlotEntries<T> extends ArrayList<T> {
  Set<Map.Entry<T,Integer>> entries() { return Collections.singleton(new AbstractMap.SimpleEntry<>(get(0),3)); }
}
public class RawIteratorSlot<T> {
  StringBuilder trace=new StringBuilder();
  RawIteratorSlot<T> add(T value) { trace.append(value).append(";");return this; }
  RawIteratorSlot<T> add(T... values) { for(T value:values) add(value);return this; }
  RawIteratorSlot<T> collect(Iterable<? extends T> values) {
    if(values instanceof SlotEntries) {
      for(Map.Entry<T,Integer> entry:(Set<Map.Entry<T,Integer>>)((SlotEntries)values).entries()) add(entry.getKey());
    } else {
      for(T value:values) add(value);
    }
    return this;
  }
  public static void main(String[] args) {
    SlotEntries<String> entries=new SlotEntries<>();entries.add("p");
    RawIteratorSlot<String> c=new RawIteratorSlot<>();
    c.collect(entries);c.collect(Arrays.asList("a","b"));
    System.out.print(c.trace);
  }
}`)
}

func TestT19ConditionalWildcardReturnRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ConditionalWildcardReturn", `import java.util.function.*;
class WrappedFunction<X> implements Function<X,X> {
  final Function<? super X,? super X> delegate;
  WrappedFunction(Function<? super X,? super X> delegate) {this.delegate=delegate;}
  public X apply(X value) {return (X)delegate.apply(value);}
}
public class ConditionalWildcardReturn<T> {
  Function<? super T,? super T> action;
  Function<T,T> get(boolean wrap) {
    return (Function<T,T>)(wrap && action!=null ? new WrappedFunction(action):action);
  }
  public static void main(String[] args) {
    ConditionalWildcardReturn<String> c=new ConditionalWildcardReturn<>();
    System.out.print(c.get(false)+":"+c.get(true)+";");
    c.action=x->x+"!";
    System.out.print(c.get(false).apply("a")+":"+c.get(true).apply("b"));
  }
}`)
}
