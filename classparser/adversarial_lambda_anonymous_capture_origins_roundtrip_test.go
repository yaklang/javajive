package javaclassparser

import (
	"strings"
	"testing"
)

// Lambda implementation parameters are another declaration scope. Their
// original words must be mapped through the invokedynamic capture tuple,
// rather than borrowing the anonymous field's source spelling as a new local.
func TestAdversarialLambdaAnonymousCapturesKeepOriginalDeclarationOriginsRoundTrip(t *testing.T) {
	for _, owner := range []string{"LambdaOriginOwner", "RenamedLambdaOriginOwner"} {
		for _, mode := range []string{"static", "instance"} {
			for _, layout := range []string{"narrow", "wide-and-equal-reference"} {
				t.Run(owner+"/"+mode+"/"+layout, func(t *testing.T) {
					fixture := `class LambdaOriginOwner {
 int base;
 LambdaOriginOwner(int n){base=n;}
 private LambdaOriginOwner(LambdaOriginOwner input){LambdaOriginObserved.value=value();LambdaOriginObserved.calls++;base=input.base;}
 int value(){return base;}
 static java.util.function.Supplier<LambdaOriginOwner> make(final LambdaOriginOwner input,final int delta){return ()->new LambdaOriginOwner(input){int value(){return input.base+delta;}};}
 static java.util.Iterator<Integer> tail(final int n){return new java.util.Iterator<Integer>(){boolean consumed;public boolean hasNext(){return !consumed;}public Integer next(){if(consumed)throw new java.util.NoSuchElementException();consumed=true;return n;}};}
}
class LambdaOriginObserved{static int value,calls;}
class LambdaOriginDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
 LambdaOriginOwner owner=new LambdaOriginOwner(n),input=new LambdaOriginOwner(n^0x5a5a5a5a);
 LambdaOriginObserved.calls=0;
 java.util.function.Supplier<LambdaOriginOwner> factory=owner.make(input,delta);
 if(LambdaOriginObserved.calls!=0)throw new AssertionError("allocation moved before lambda invocation");
 input.base+=101;owner.base-=19;
 int expected=java.math.BigInteger.valueOf(input.base).add(java.math.BigInteger.valueOf(delta)).intValue();
 LambdaOriginOwner first=factory.get();
 if(first.value()!=expected||LambdaOriginObserved.value!=expected||LambdaOriginObserved.calls!=1)throw new AssertionError("original captured words/heap/copy-super callback");
 LambdaOriginOwner second=factory.get();
 if(first==second||second.value()!=expected||LambdaOriginObserved.value!=expected||LambdaOriginObserved.calls!=2)throw new AssertionError("lambda allocation identity/order");
 java.util.Iterator<Integer> tail=LambdaOriginOwner.tail(n);if(!tail.hasNext()||tail.next()!=n||tail.hasNext())throw new AssertionError("independent terminal iterator");try{tail.next();throw new AssertionError("missing exhaustion");}catch(java.util.NoSuchElementException correct){}
 rows++;}
 LambdaOriginObserved.calls=0;
 LambdaOriginOwner nonnull=new LambdaOriginOwner(7);
 java.util.function.Supplier<LambdaOriginOwner> nullFactory=nonnull.make(null,3);
 if(LambdaOriginObserved.calls!=0)throw new AssertionError("captured null evaluated eagerly");
 for(int repeat=0;repeat<2;repeat++){try{nullFactory.get();throw new AssertionError("missing null capture failure");}catch(NullPointerException correct){}if(LambdaOriginObserved.calls!=0)throw new AssertionError("callback progressed after original null read");}
 System.out.println(rows+":lambda:origins:heap:callbacks:identity");}}
`
					if mode == "instance" {
						fixture = strings.Replace(fixture, "static java.util.function.Supplier<LambdaOriginOwner> make", "java.util.function.Supplier<LambdaOriginOwner> make", 1)
						fixture = strings.Replace(fixture, "return input.base+delta;", "return LambdaOriginOwner.this.base+input.base+delta;", 1)
						fixture = strings.Replace(fixture, "java.math.BigInteger.valueOf(input.base).add", "java.math.BigInteger.valueOf(owner.base).add(java.math.BigInteger.valueOf(input.base)).add", 1)
					}

					if layout == "wide-and-equal-reference" {
						fixture = strings.Replace(fixture, "final LambdaOriginOwner input,final int delta", "final LambdaOriginOwner input,final long wide,final LambdaOriginOwner other,final double fraction,final int delta", 1)
						fixture = strings.Replace(fixture, "input.base+delta;", "input.base+31*other.base+delta+(int)wide+(int)fraction;", 1)
						fixture = strings.Replace(fixture, "nonnull.make(null,3)", "nonnull.make(null,0L,nonnull,13.25,3)", 1)
						fixture = strings.Replace(fixture, "java.util.function.Supplier<LambdaOriginOwner> factory=owner.make(input,delta);", "LambdaOriginOwner other=new LambdaOriginOwner(n^0x33333333);long wide=((long)n)*0x100000001L;double fraction=13.25;java.util.function.Supplier<LambdaOriginOwner> factory=owner.make(input,wide,other,fraction,delta);", 1)
						fixture = strings.Replace(fixture, "input.base+=101;owner.base-=19;", "input.base+=101;other.base-=37;owner.base-=19;", 1)
						fixture = strings.Replace(fixture, ".add(java.math.BigInteger.valueOf(delta)).intValue()", ".add(java.math.BigInteger.valueOf(delta)).add(java.math.BigInteger.valueOf(other.base).multiply(java.math.BigInteger.valueOf(31))).add(java.math.BigInteger.valueOf(n)).add(java.math.BigInteger.valueOf(13)).intValue()", 1)
					}
					fixture = strings.ReplaceAll(fixture, "LambdaOriginOwner", owner)
					testNativePrivateSetterCompiledFixtureWithShape(t, owner, "LambdaOriginDriver", "25:lambda:origins:heap:callbacks:identity\n", func(t *testing.T, debug string) map[string][]byte {
						files := nativeCompileDebugClasses(t, fixture, debug)
						for name, raw := range files {
							if !strings.HasPrefix(name, owner) || !strings.HasSuffix(name, ".class") {
								continue
							}
							object, err := Parse(raw)
							if err != nil {
								t.Fatal(err)
							}
							if object.GetClassName() == owner+"$2" {
								object.AccessFlags |= 0x0010
							}
							for _, a := range object.Attributes {
								if table, ok := a.(*InnerClassesAttribute); ok {
									for _, row := range table.Classes {
										binary, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
										if known && binary == owner+"$2" {
											row.InnerClassAccessFlags |= 0x0010
										}
									}
								}
							}
							files[name] = object.Bytes()
						}
						return files
					}, nativeAnonymousPrefixRepresentationShape)
				})
			}
		}
	}
}
