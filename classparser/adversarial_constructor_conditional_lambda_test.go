package javaclassparser

import (
	"strings"
	"testing"
)

// A generic conditional joins a bound method reference and a stateless lambda.
// Rendering a target-type view is not another source occurrence. The original
// caller independently checks constructor short-circuit effects, deferred
// callbacks, object identity, null-failure timing and modular long arithmetic.
const constructorConditionalLambdaFixture = `import java.util.function.Consumer;
public class ArtifactLambdaOwner<K,V> {
 final boolean firstFlag,secondFlag;final Consumer<ArtifactPolicyNode<K,V>> policy;
 public String trace="";public int checks,calls;public long state;public Object seen;
 protected ArtifactLambdaOwner(boolean a,boolean b,long seed){
  firstFlag=a;secondFlag=b;state=seed;
  policy=(first()||second())?this::onAccess:ignored->{};
  trace+="C";
 }
 boolean first(){checks++;trace+="A";return firstFlag;}
 boolean second(){checks++;trace+="B";return secondFlag;}
 void onAccess(ArtifactPolicyNode<K,V> node){calls++;trace+="D";seen=node;state+=node.value;}
 public void accept(ArtifactPolicyNode<K,V> node){policy.accept(node);}
}
class ArtifactPolicyNode<K,V> {
 public final K key;public final V valueObject;public final long value;
 ArtifactPolicyNode(K k,V v,long n){key=k;valueObject=v;value=n;}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;
  for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true})for(long seed:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(int kind=0;kind<6;kind++){
   Object identity=new Object();long number=new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE,0L}[kind];
   ArtifactPolicyNode<String,Object> node=kind==5?null:new ArtifactPolicyNode<String,Object>("key",identity,number);
   ArtifactLambdaOwner<String,Object> owner=new ArtifactLambdaOwner<String,Object>(a,b,seed);
   String eager=a?"AC":"ABC";boolean active=a||b;
   if(owner.checks!=(a?1:2)||owner.calls!=0||owner.seen!=null||owner.state!=seed||!owner.trace.equals(eager))throw new AssertionError("constructor branch/factory timing");
   for(int i=0;i<3;i++){
    Throwable error=null;try{owner.accept(node);}catch(Throwable e){error=e;}
    if((error!=null)!=(active&&node==null)||error!=null&&!(error instanceof NullPointerException))throw new AssertionError("deferred null-failure phase");
   }
   long expected=active&&node!=null?java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(number).multiply(java.math.BigInteger.valueOf(3L))).longValue():seed;
   if(owner.calls!=(active?3:0)||owner.seen!=(active?node:null)||owner.state!=expected||!owner.trace.equals(eager+(active?"DDD":"")))throw new AssertionError("bound receiver/generic field/callback multiplicity/overflow");
   if(node!=null&&(node.valueObject!=identity||!node.key.equals("key")))throw new AssertionError("generic node identity");rows++;
  }
  System.out.println(rows+":constructor:conditional:policy:timing:overflow:null:binding");
 }
}`

func TestAdversarialConstructorConditionalGenericPolicyRecordsOnlyActualLambda(t *testing.T) {
	testArtifactLambdaMetadata(t, constructorConditionalLambdaFixture, []bool{false, true}, "120:constructor:conditional:policy:timing:overflow:null:binding\n")
}

func TestAdversarialConstructorConditionalStatelessPolicyPreservesDeferredEffects(t *testing.T) {
	fixture := strings.NewReplacer(
		`public String trace="";`, `public static int emptyCalls;public static long emptyTotal;public String trace="";`,
		`ignored->{};`, `ignored->{emptyCalls++;emptyTotal+=ignored.value;};`,
		`Object identity=new Object();`, `ArtifactLambdaOwner.emptyCalls=0;ArtifactLambdaOwner.emptyTotal=0L;Object identity=new Object();`,
		`(error!=null)!=(active&&node==null)`, `(error!=null)!=(node==null)`,
		`if(node!=null&&(node.valueObject`, `long emptyExpected=!active&&node!=null?java.math.BigInteger.valueOf(number).multiply(java.math.BigInteger.valueOf(3L)).longValue():0L;
   if(ArtifactLambdaOwner.emptyCalls!=(active?0:3)||ArtifactLambdaOwner.emptyTotal!=emptyExpected)throw new AssertionError("stateless body effects remain deferred and once per call");
   if(node!=null&&(node.valueObject`,
	).Replace(constructorConditionalLambdaFixture)
	testArtifactLambdaMetadata(t, fixture, []bool{false, true}, "120:constructor:conditional:policy:timing:overflow:null:binding\n")
}
