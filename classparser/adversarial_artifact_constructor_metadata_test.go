package javaclassparser

import (
	"strings"
	"testing"
)

const artifactAnonymousConstructorMetadataFixture = `import java.util.function.LongUnaryOperator;
public class ArtifactLambdaOwner {
 public long base;public int calls;
 public ArtifactLambdaOwner(long base){this.base=base;}
 LongUnaryOperator bind(final long delta){return new LongUnaryOperator(){public long applyAsLong(long value){calls++;return base*31L+delta+value;}};}
 LongUnaryOperator own(){return new LongUnaryOperator(){public long applyAsLong(long value){calls++;return base+value;}};}
}
class ArtifactLambdaDriver {
 static long expected(long base,long delta,long value,boolean wide){java.math.BigInteger b=java.math.BigInteger.valueOf(base);if(wide)b=b.multiply(java.math.BigInteger.valueOf(31L));return b.add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(value)).longValue();}
 public static void main(String[]args){int rows=0;
  for(long base:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})
   for(long delta:new long[]{Long.MIN_VALUE,0L,Long.MAX_VALUE})
    for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
     ArtifactLambdaOwner owner=new ArtifactLambdaOwner(base),other=new ArtifactLambdaOwner(base^17L);
     LongUnaryOperator wide=owner.bind(delta),outer=owner.own(),peer=other.bind(delta);
     if(owner.calls!=0||other.calls!=0)throw new AssertionError("eager constructor/capture body");
     if(wide.applyAsLong(value)!=expected(base,delta,value,true)||owner.calls!=1||other.calls!=0)throw new AssertionError("wide capture ports/overflow");
     if(outer.applyAsLong(value)!=expected(base,0L,value,false)||owner.calls!=2)throw new AssertionError("enclosing receiver port");
     if(peer.applyAsLong(value)!=expected(base^17L,delta,value,true)||other.calls!=1||owner.calls!=2)throw new AssertionError("capture receiver alias");
     if(wide.applyAsLong(value)!=expected(base,delta,value,true)||owner.calls!=3)throw new AssertionError("repeated capture state");rows++;
    }
  System.out.println(rows+":artifact:constructor:wide:receiver:alias:overflow");
 }
}`

func TestAdversarialArtifactAnonymousConstructorMetadataKeepsImplicitPorts(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactAnonymousConstructorMetadataFixture, []bool{false, true}, "75:artifact:constructor:wide:receiver:alias:overflow\n")
}

func TestAdversarialArtifactMemberAnonymousConstructorMetadataKeepsOwningPublication(t *testing.T) {
	source := strings.Replace(artifactAnonymousConstructorMetadataFixture, " LongUnaryOperator bind(final long delta)", " public static class Cell {final ArtifactLambdaOwner owner;Cell(ArtifactLambdaOwner owner){this.owner=owner;}\n LongUnaryOperator bind(final long delta)", 1)
	source = strings.Replace(source, "return base*31L+delta+value", "return owner.base*31L+delta+value", 1)
	source = strings.Replace(source, "return base+value", "return owner.base+value", 1)
	source = strings.ReplaceAll(source, "{calls++;return", "{owner.calls++;return")
	source = strings.Replace(source, "}\nclass ArtifactLambdaDriver", " }}\nclass ArtifactLambdaDriver", 1)
	source = strings.ReplaceAll(source, "owner.bind(delta)", "new ArtifactLambdaOwner.Cell(owner).bind(delta)")
	source = strings.ReplaceAll(source, "owner.own()", "new ArtifactLambdaOwner.Cell(owner).own()")
	source = strings.ReplaceAll(source, "other.bind(delta)", "new ArtifactLambdaOwner.Cell(other).bind(delta)")
	testArtifactLambdaMetadata(t, source, []bool{false, true}, "75:artifact:constructor:wide:receiver:alias:overflow\n")
}
