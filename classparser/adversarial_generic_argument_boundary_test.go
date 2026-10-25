package javaclassparser

import (
	"strings"
	"testing"
)

const recursiveArgumentBoundaryFixture = `
class SuffixOwner<K extends Comparable<K>> {
 SuffixOwner(){}static class Factory{static <K extends Comparable<K>> SuffixOwner<K> make(){return new SuffixOwner<K>();}}
 int order(K left,Comparable right){return left.compareTo((K)right);}
}
class SuffixDriver { public static void main(String[] args){
 SuffixOwner<Integer> owner=SuffixOwner.Factory.make();int rows=0;
 for(Integer left:new Integer[]{null,Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(Comparable right:new Comparable[]{null,Integer.MIN_VALUE,0,Integer.MAX_VALUE,"foreign"}){
  Integer result=null;Throwable thrown=null;try{result=owner.order(left,right);}catch(Throwable t){thrown=t;}
  Class<?> expected=left==null?NullPointerException.class:right!=null&&!(right instanceof Integer)?ClassCastException.class:right==null?NullPointerException.class:null;
  if(expected!=null?thrown==null||thrown.getClass()!=expected:thrown!=null||Integer.signum(result.intValue())!=Integer.signum(java.math.BigInteger.valueOf(left.longValue()).compareTo(java.math.BigInteger.valueOf(((Integer)right).longValue()))))throw new AssertionError("recursive argument binding",thrown);rows++;
 }System.out.println(rows+":recursive:argument:erasure");}}
`

func TestAdversarialRecursiveArgumentBindingKeepsOriginalOperandErasure(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, rename := range []bool{false, true} {
			t.Run(debug+"/"+map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
				fixture := recursiveArgumentBoundaryFixture
				if rename {
					fixture = strings.NewReplacer("<K extends", "<Element extends", "Comparable<K>", "Comparable<Element>", "SuffixOwner<K>", "SuffixOwner<Element>", "K left", "Element left", "(K)right", "(Element)right", "order(", "evaluate(").Replace(fixture)
				}
				constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "20:recursive:argument:erasure\n")
			})
		}
	}
}

const fluentArgumentBoundaryFixture = `
interface SuffixResult { Object key();Object value(); }
class SuffixPair<K,V> implements SuffixResult {
 final Object k,v;SuffixPair(Object k,Object v){this.k=k;this.v=v;}public Object key(){return k;}public Object value(){return v;}
 SuffixBuilder<K,V> builder(){return new SuffixBuilder<K,V>();}
}
class SuffixBuilder<K,V>{Object k,v;SuffixBuilder<K,V> key(K k){SuffixEffects.trace+="K;";this.k=k;return this;}SuffixBuilder<K,V> value(V v){SuffixEffects.trace+="V;";this.v=v;return this;}SuffixPair<K,V> build(){SuffixEffects.trace+="B;";return new SuffixPair<K,V>(k,v);}}
class SuffixEffects {static String trace="";static Object k,v;static int consume(int n,SuffixResult result){trace+="C;";if(n!=17||result.key()!=k||result.value()!=v)throw new AssertionError("identity");return 9;}}
class SuffixOwner {
 private SuffixOwner(){}static class Factory{static SuffixOwner make(){return new SuffixOwner();}}
 static final SuffixPair<String,String> defaults=new SuffixPair<String,String>(null,null);
 int evaluate(java.util.Map<String,String> input){java.util.Map.Entry<String,String> e=input.entrySet().iterator().next();return SuffixEffects.consume(17,defaults.builder().key(e.getKey()).value(e.getValue()).build());}
}
class SuffixDriver {public static void main(String[]args){SuffixOwner owner=SuffixOwner.Factory.make();int rows=0;
 for(Object key:new Object[]{null,"text",new Object(),Integer.MIN_VALUE})for(Object value:new Object[]{null,"text",new Object(),Integer.MAX_VALUE}){
  java.util.Map map=new java.util.HashMap();map.put(key,value);SuffixEffects.k=key;SuffixEffects.v=value;SuffixEffects.trace="";
  if(owner.evaluate(map)!=9||!SuffixEffects.trace.equals("K;V;B;C;"))throw new AssertionError("fluent erasure or evaluation order");rows++;
 }System.out.println(rows+":fluent:argument:raw:identity");}}
`

func TestAdversarialFluentArgumentChainKeepsErasedChecksAndEvaluation(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, rename := range []bool{false, true} {
			t.Run(debug+"/"+map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
				fixture := fluentArgumentBoundaryFixture
				if rename {
					fixture = strings.NewReplacer("SuffixPair", "EntryPacket", "SuffixBuilder", "PacketComposer", "key(", "first(", "value(", "second(", "build(", "finish(").Replace(fixture)
				}
				constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "16:fluent:argument:raw:identity\n")
			})
		}
	}
}

// An erased view may avoid inventing checks, but may not remove a check which
// the original bytecode actually performed. The unchanged driver observes
// which setter effects occurred before each possible failure.
func TestAdversarialFluentArgumentChainRetainsOriginalNarrowingChecks(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.ReplaceAll(fluentArgumentBoundaryFixture,
		".key(e.getKey()).value(e.getValue())", ".key((String)(Object)e.getKey()).value((String)(Object)e.getValue())")
	fixture = strings.ReplaceAll(fixture,
		`if(owner.evaluate(map)!=9||!SuffixEffects.trace.equals("K;V;B;C;"))throw new AssertionError("fluent erasure or evaluation order");rows++;`,
		`Throwable failure=null;int result=0;try{result=owner.evaluate(map);}catch(Throwable t){failure=t;}
  boolean badKey=key!=null&&!(key instanceof String),badValue=value!=null&&!(value instanceof String);
  String expected=badKey?"":badValue?"K;":"K;V;B;C;";
  if(!SuffixEffects.trace.equals(expected)||(badKey||badValue?failure==null||failure.getClass()!=ClassCastException.class:failure!=null||result!=9))throw new AssertionError("original check position",failure);rows++;`)
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, rename := range []bool{false, true} {
			t.Run(debug+"/"+map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
				source := fixture
				if rename {
					source = strings.NewReplacer("SuffixPair", "EntryPacket", "SuffixBuilder", "PacketComposer", "key(", "first(", "value(", "second(", "build(", "finish(").Replace(source)
				}
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{"SuffixOwner"}, "16:fluent:argument:raw:identity\n")
			})
		}
	}
}

func TestAdversarialFluentArgumentChainWithCovariantBridge(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	fixture := strings.NewReplacer(
		"class SuffixBuilder<K,V>{", "interface SuffixFactory{SuffixResult build();} class SuffixBuilder<K,V> implements SuffixFactory{",
		"SuffixPair<K,V> build(){", "public SuffixPair<K,V> build(){",
	).Replace(fluentArgumentBoundaryFixture)
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, rename := range []bool{false, true} {
			t.Run(debug+"/"+map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
				source := fixture
				if rename {
					source = strings.NewReplacer("SuffixPair", "EntryPacket", "SuffixBuilder", "PacketComposer", "SuffixFactory", "ResultProducer", "key(", "first(", "value(", "second(", "build(", "finish(").Replace(source)
				}
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{"SuffixOwner"}, "16:fluent:argument:raw:identity\n")
			})
		}
	}
}

func TestAdversarialFluentArgumentChainFromExternalGenericField(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	const field = "static final SuffixPair<String,String> defaults=new SuffixPair<String,String>(null,null);"
	fixture := "class SuffixDefaults{" + field + "}\n" + strings.NewReplacer(
		field, "",
		"defaults.builder()", "SuffixDefaults.defaults.builder()",
		"class SuffixBuilder<K,V>{", "interface SuffixFactory{SuffixResult build();} class SuffixBuilder<K,V> implements SuffixFactory{",
		"SuffixPair<K,V> build(){", "public SuffixPair<K,V> build(){",
	).Replace(fluentArgumentBoundaryFixture)
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, rename := range []bool{false, true} {
			t.Run(debug+"/"+map[bool]string{false: "original", true: "renamed"}[rename], func(t *testing.T) {
				source := fixture
				if rename {
					source = strings.NewReplacer("SuffixDefaults", "PacketStorage", "defaults", "seed", "SuffixPair", "EntryPacket", "SuffixBuilder", "PacketComposer", "SuffixFactory", "ResultProducer", "key(", "first(", "value(", "second(", "build(", "finish(").Replace(source)
				}
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{"SuffixOwner"}, "16:fluent:argument:raw:identity\n")
			})
		}
	}
}
