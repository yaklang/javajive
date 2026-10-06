package javaclassparser

import (
	"strings"
	"testing"
)

// Original JVM argument evaluation is the oracle. A condition inside an array
// element must keep the already evaluated receiver and preceding array cells;
// it is not an ordinary local that can move behind constructor initialization.
func TestAdversarialConstructorConditionalWordPacketRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "ConditionalWordPacket", `
class ConditionalWordParent {
 final String result;
 ConditionalWordParent(String result){this.result=result;}
}

public class ConditionalWordPacket extends ConditionalWordParent {
 ConditionalWordPacket(int n,String value){super(String.format("%d:%s",n,value==null?"empty":value));}
 public static void main(String[]args){
  for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})
   for(String value:new String[]{null,"","x"})
    System.out.println(new ConditionalWordPacket(n,value).result);
 }
}`, Precision, Compatibility, "legacy")
}

const constructorConditionalConsumerFixture = `
class PacketEvents {
 static String trace=""; static int fail;
 static final RuntimeException failure=new IllegalStateException("original identity");
 static String mark(String name,int stage){trace+=name;if(fail==stage)throw failure;return name;}
 static boolean choose(boolean flag){mark("C",2);return flag;}
}
class PacketAdapter {
 static String adapt(String head,Object[] data){PacketEvents.mark("T",4);return head+"["+data[0]+","+data[1]+"]";}
}
class PacketBase {
 final String result;
 PacketBase(String result){PacketEvents.mark("B",5);this.result=result;}
}
class PacketSubject extends PacketBase {
 PacketSubject(boolean flag){super(PacketAdapter.adapt(PacketEvents.mark("H",0),new Object[]{PacketEvents.mark("A",1),PacketEvents.choose(flag)?PacketEvents.mark("L",3):PacketEvents.mark("R",3)}));}
}
class PacketDriver {
 public static void main(String[]args){
  for(boolean flag:new boolean[]{false,true})for(int fail=-1;fail<=5;fail++){
   PacketEvents.trace="";PacketEvents.fail=fail;
   try{PacketSubject subject=new PacketSubject(flag);System.out.println(PacketEvents.trace+":"+subject.result);}
   catch(RuntimeException failure){System.out.println(PacketEvents.trace+":error:"+(failure==PacketEvents.failure));}
  }
 }
}`

// Only PacketSubject is replaced. The original adapter, superclass and driver
// remain independent executable evidence. Expected traces also come from this
// small Go oracle, including the identity and position of every abrupt exit.
func TestAdversarialConstructorConditionalConsumerFamiliesRoundTrip(t *testing.T) {
	for _, variant := range []string{"original", "renamed", "this delegation", "wide parameters", "generic producer", "first conditional element"} {
		t.Run(variant, func(t *testing.T) {
			f := constructorConditionalConsumerFixture
			first := false
			subject := "PacketSubject"
			switch variant {
			case "renamed":
				f = strings.ReplaceAll(f, "PacketSubject", "AlternatePacket")
				f = strings.ReplaceAll(f, "PacketAdapter", "AlternateTransformer")
				f = strings.ReplaceAll(f, "adapt", "produce")
				subject = "AlternatePacket"
			case "this delegation":
				f = strings.Replace(f, "extends PacketBase {", "extends PacketBase {private PacketSubject(String result){super(result);}", 1)
				f = strings.Replace(f, "{super(PacketAdapter.adapt", "{this(PacketAdapter.adapt", 1)
			case "wide parameters":
				f = strings.Replace(f, "PacketSubject(boolean flag)", "PacketSubject(long padding,double floating,boolean flag)", 1)
				f = strings.Replace(f, "new PacketSubject(flag)", "new PacketSubject(Long.MIN_VALUE,-0.0,flag)", 1)
			case "generic producer":
				f = strings.Replace(f, "static String adapt(String head,Object[] data)", "static <T>String adapt(String head,T[] data)", 1)
			case "first conditional element":
				f = strings.Replace(f, "new Object[]{PacketEvents.mark(\"A\",1),PacketEvents.choose(flag)?PacketEvents.mark(\"L\",3):PacketEvents.mark(\"R\",3)}", "new Object[]{PacketEvents.choose(flag)?PacketEvents.mark(\"L\",3):PacketEvents.mark(\"R\",3),PacketEvents.mark(\"A\",1)}", 1)
				first = true
			}
			var oracle strings.Builder
			for _, arm := range []string{"R", "L"} {
				for fail := -1; fail <= 5; fail++ {
					stages := []int{0, 1, 2, 3, 4, 5}
					letters := []string{"H", "A", "C", arm, "T", "B"}
					result := "H[A," + arm + "]"
					if first {
						stages, letters = []int{0, 2, 3, 1, 4, 5}, []string{"H", "C", arm, "A", "T", "B"}
						result = "H[" + arm + ",A]"
					}
					for i, stage := range stages {
						oracle.WriteString(letters[i])
						if stage == fail {
							result = "error:true"
							break
						}
					}
					oracle.WriteString(":" + result + "\n")
				}
			}
			testNativeIndependentFamilyFixture(t, f, []string{subject}, "PacketDriver", oracle.String(), nativeLexicalExactSignatures)
		})
	}
}
