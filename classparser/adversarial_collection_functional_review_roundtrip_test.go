package javaclassparser

import "testing"

func TestAdversarialReviewedCollectionFunctionalBindingsRoundTrip(t *testing.T) {
	t.Parallel()
	// Raw receiver views must keep the instantiated SAM's payload checks, while
	// preserving object identity, lazy evaluation, callback order and captures.
	roundTripGenericFlow(t, "CollectionFunctionalReview", `
import java.util.*;import java.util.function.*;import java.util.stream.*;
class CollectionReviewPayload {final String name;final int rank;CollectionReviewPayload(String n,int r){name=n;rank=r;}long cost(){CollectionReviewHelpers.trace.append("C").append(name);return rank;}public String toString(){return name;}}
class CollectionReviewHelpers {
 static StringBuilder trace=new StringBuilder();static final IllegalStateException failure=new IllegalStateException("callback");
 static List<CollectionReviewPayload> items(int mode){trace.append("I");if(mode==0)return new ArrayList<>();if(mode==1)return new ArrayList<>(Arrays.asList(new CollectionReviewPayload("a",2),new CollectionReviewPayload("b",1),new CollectionReviewPayload("c",2)));if(mode==2)return (List)new ArrayList<>(Arrays.asList(new CollectionReviewPayload("a",2),null));return (List)new ArrayList<>(Arrays.asList(new CollectionReviewPayload("a",2),"wrong"));}
 static <T> void apply(Collection<T> items,Consumer<T> action){for(T item:items)action.accept(item);}
}
public class CollectionFunctionalReview {
 final String prefix="p";
 boolean excluded(CollectionReviewPayload p){CollectionReviewHelpers.trace.append("P");return p.rank>1;}
 Object remove(int mode){ArrayList raw=new ArrayList(CollectionReviewHelpers.items(mode));raw.removeIf((Predicate<CollectionReviewPayload>)this::excluded);return raw;}
 Object collect(int mode){List<CollectionReviewPayload> items=CollectionReviewHelpers.items(mode);Stream<Object> mapped=items.stream().map((CollectionReviewPayload p)->{CollectionReviewHelpers.trace.append("M");return p==null?null:prefix+p.name;});return mapped.filter(Objects::nonNull).collect(Collectors.toList());}
 Object apply(int mode){List<CollectionReviewPayload> items=CollectionReviewHelpers.items(mode);List<String> result=new ArrayList<>();CollectionReviewHelpers.apply((Collection)items,(Consumer<CollectionReviewPayload>)(p->{CollectionReviewHelpers.trace.append("A");if(p.rank==1)throw CollectionReviewHelpers.failure;result.add(prefix+p.name);}));return result;}
 Object sorted(int mode){List<CollectionReviewPayload> items=CollectionReviewHelpers.items(mode);Collections.sort(items,(CollectionReviewPayload a,CollectionReviewPayload b)->{CollectionReviewHelpers.trace.append("S");return Long.compare(a.cost(),b.cost());});return items;}
 Object comparing(int mode){List<CollectionReviewPayload> items=CollectionReviewHelpers.items(mode);items.sort(Comparator.comparing((CollectionReviewPayload p)->{CollectionReviewHelpers.trace.append("K");return p.rank;}).thenComparing((CollectionReviewPayload p)->p.name));return items;}
 long longs(int mode){Collection raw=CollectionReviewHelpers.items(mode);return ((Stream<CollectionReviewPayload>)raw.stream()).mapToLong(CollectionReviewPayload::cost).sum();}
 static String run(int action,int mode){CollectionReviewHelpers.trace.setLength(0);CollectionFunctionalReview c=new CollectionFunctionalReview();String result;try{switch(action){case 0:result=String.valueOf(c.remove(mode));break;case 1:result=String.valueOf(c.collect(mode));break;case 2:result=String.valueOf(c.apply(mode));break;case 3:result=String.valueOf(c.sorted(mode));break;case 4:result=String.valueOf(c.comparing(mode));break;default:result=String.valueOf(c.longs(mode));}}catch(Throwable e){result=e.getClass().getName()+":"+(e==CollectionReviewHelpers.failure);}return result+":"+CollectionReviewHelpers.trace;}
 public static void main(String[] args){for(int action=0;action<6;action++)for(int mode=0;mode<4;mode++)System.out.println(run(action,mode));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialReviewedPrimitiveFlatMapComputeBindingsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "PrimitiveFunctionalReview", `
import java.util.*;import java.util.function.*;import java.util.stream.*;
class PrimitiveReviewLeaf {final String[] names;PrimitiveReviewLeaf(String... n){names=n;}}
class PrimitiveReviewHelpers {static StringBuilder trace=new StringBuilder();static List values(int mode,boolean doubles){if(mode==0)return Collections.emptyList();if(mode==1)return doubles?Arrays.asList(Double.valueOf(-1.5),Double.valueOf(0),Double.valueOf(2.5)):Arrays.asList(Integer.valueOf(-3),Integer.valueOf(0),Integer.valueOf(7));if(mode==2)return Arrays.asList((Object)null);return Arrays.asList("wrong");}static List leaves(int mode){if(mode==0)return Collections.emptyList();if(mode==1)return Arrays.asList(new PrimitiveReviewLeaf("a","b"),new PrimitiveReviewLeaf(),new PrimitiveReviewLeaf("c"));if(mode==2)return Arrays.asList((Object)null);return Arrays.asList("wrong");}}
public class PrimitiveFunctionalReview {
 int[] integers(int mode){List raw=PrimitiveReviewHelpers.values(mode,false);return ((Stream<Integer>)raw.stream()).mapToInt((Integer value)->{PrimitiveReviewHelpers.trace.append("I");return value.intValue();}).toArray();}
 double[] doubles(int mode){List raw=PrimitiveReviewHelpers.values(mode,true);return ((Stream<Double>)raw.stream()).mapToDouble((Double value)->{PrimitiveReviewHelpers.trace.append("D");return value.doubleValue();}).toArray();}
 Object flat(int mode){List<PrimitiveReviewLeaf> leaves=(List)PrimitiveReviewHelpers.leaves(mode);return leaves.stream().flatMap((PrimitiveReviewLeaf leaf)->{PrimitiveReviewHelpers.trace.append("F");return Arrays.stream(leaf.names).filter((String name)->{PrimitiveReviewHelpers.trace.append("P");return !name.equals("b");});}).map((String name)->{PrimitiveReviewHelpers.trace.append("M");return name+"!";}).collect(Collectors.toList());}
 Object compute(int mode){Map raw=new LinkedHashMap();if(mode==1)raw.put("a",Integer.valueOf(2));if(mode==2)raw.put("a",null);if(mode==3)raw.put("a","wrong");BiFunction<String,Integer,Integer> count=(String key,Integer previous)->{PrimitiveReviewHelpers.trace.append("B");return previous==null?Integer.valueOf(1):Integer.valueOf(previous.intValue()+1);};Object first=raw.compute("a",count);Object second=raw.compute("a",count);return first+":"+second+":"+raw;}
 static String run(int action,int mode){PrimitiveReviewHelpers.trace.setLength(0);PrimitiveFunctionalReview c=new PrimitiveFunctionalReview();String result;try{switch(action){case 0:result=Arrays.toString(c.integers(mode));break;case 1:result=Arrays.toString(c.doubles(mode));break;case 2:result=String.valueOf(c.flat(mode));break;default:result=String.valueOf(c.compute(mode));}}catch(Throwable e){result=e.getClass().getName();}return result+":"+PrimitiveReviewHelpers.trace;}
 public static void main(String[] args){for(int action=0;action<4;action++)for(int mode=0;mode<4;mode++)System.out.println(run(action,mode));}
}`, Precision, Compatibility, "legacy")
}
