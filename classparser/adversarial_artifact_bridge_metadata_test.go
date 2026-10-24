package javaclassparser

import "testing"

const artifactBridgeMetadataFixture = `import java.util.function.Function;
public class ArtifactLambdaOwner {
 public static final RuntimeException FAILURE=new IllegalStateException("identity");
 public final String token;public int calls;
 public ArtifactLambdaOwner(String token){this.token=token;}
 public final Function<String,String> select=new Function<String,String>() {
  public String apply(String value){calls++;if(value==null)throw FAILURE;return value.isEmpty()?token:value;}
 };
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;
  for(String token:new String[]{null,new String("token"),new String("\uD83D\uDE00")})
   for(String value:new String[]{null,"",new String("same"),new String("\uD834\uDD1E")}){
    ArtifactLambdaOwner owner=new ArtifactLambdaOwner(token),peer=new ArtifactLambdaOwner("peer");
    Function erased=owner.select;
    if(owner.calls!=0||peer.calls!=0)throw new AssertionError("eager initializer/bridge body");
    for(int repeat=0;repeat<2;repeat++){
     try{Object actual=erased.apply(value);if(value==null||actual!=(value.isEmpty()?token:value))throw new AssertionError("bridge return/alias");}
     catch(RuntimeException failure){if(value!=null||failure!=ArtifactLambdaOwner.FAILURE)throw new AssertionError("body failure identity",failure);}
     if(owner.calls!=repeat+1||peer.calls!=0)throw new AssertionError("bridge receiver/repeated state");
    }
    try{erased.apply(Integer.valueOf(17));throw new AssertionError("missing bridge cast");}
    catch(ClassCastException expected){}
    if(owner.calls!=2||peer.calls!=0)throw new AssertionError("cast must precede body effects");
    if(peer.select.apply("")!=peer.token||owner.calls!=2||peer.calls!=1)throw new AssertionError("peer receiver alias");rows++;
   }
  System.out.println(rows+":artifact:bridge:castphase:receiver:alias:nullidentity");
 }
}`

func TestAdversarialArtifactBridgeParameterMetadataKeepsCastBeforeBody(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactBridgeMetadataFixture, []bool{false, true}, "12:artifact:bridge:castphase:receiver:alias:nullidentity\n")
}

const artifactArrayBridgeMetadataFixture = `public class ArtifactLambdaOwner {
 public static final RuntimeException FAILURE=new IllegalStateException("identity");public int calls;
 public interface Join<A,B,R>{R combine(A left,B right);}
 Join<String,long[],long[]> bind(final long[] captured){return new Join<String,long[],long[]>() {
  public long[] combine(String left,long[] right){calls++;if(left==null)throw FAILURE;return left.isEmpty()?captured:right;}
 };}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;
  for(long[] captured:new long[][]{null,new long[]{Long.MIN_VALUE,Long.MAX_VALUE}})
   for(String left:new String[]{null,"",new String("same"),new String("\uD83D\uDE00")})
    for(long[] right:new long[][]{null,captured,new long[]{0L,-1L}}){
     ArtifactLambdaOwner owner=new ArtifactLambdaOwner();ArtifactLambdaOwner.Join erased=owner.bind(captured);
     if(owner.calls!=0)throw new AssertionError("eager array capture/body");
     for(int repeat=0;repeat<2;repeat++){
      try{Object actual=erased.combine(left,right);if(left==null||actual!=(left.isEmpty()?captured:right))throw new AssertionError("bridge array return/alias");}
      catch(RuntimeException failure){if(left!=null||failure!=ArtifactLambdaOwner.FAILURE)throw new AssertionError("array body failure identity",failure);}
      if(owner.calls!=repeat+1)throw new AssertionError("array bridge repeated state");
     }
     try{erased.combine(Integer.valueOf(17),right);throw new AssertionError("missing left cast");}catch(ClassCastException expected){}
     try{erased.combine(null,Integer.valueOf(19));throw new AssertionError("right cast must precede null body failure");}catch(ClassCastException expected){}
     if(owner.calls!=2)throw new AssertionError("erased argument casts must precede all body effects");rows++;
    }
  System.out.println(rows+":artifact:bridge:array:orderedcasts:alias:nullidentity");
 }
}`

func TestAdversarialArtifactArrayBridgeParameterMetadataKeepsOrderedCastsAndAliases(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactArrayBridgeMetadataFixture, []bool{false, true}, "24:artifact:bridge:array:orderedcasts:alias:nullidentity\n")
}
