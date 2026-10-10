package javaclassparser

import (
	"strings"
	"testing"
)

const orderedVoidInitializerFixture = `interface OrderedVoidRead {int get();}
class OrderedVoidCell {int length=17;}
class OrderedVoidEffects {static String trace="";static int fail;static final RuntimeException error=new RuntimeException("original");
 static void observe(OrderedVoidRead who,OrderedVoidCell cell,int n){trace+="C";if(fail==1)throw error;if(!who.getClass().isAnonymousClass()||who.get()!=17)throw new AssertionError("void initializer callback identity/position");cell.length=n;}
 static void finish(){trace+="D";if(fail==2)throw error;}
}
class OrderedVoidOwner implements OrderedVoidRead {public int get(){return 99;}private OrderedVoidCell cell;private int number;OrderedVoidOwner(OrderedVoidCell cell,int n){this.cell=cell;number=n;}
 OrderedVoidRead make(){return new OrderedVoidRead(){{OrderedVoidEffects.observe(this,cell,number);OrderedVoidEffects.finish();}public int get(){return cell.length;}};}
}
class OrderedVoidDriver {public static void main(String[]args){int rows=0;for(boolean empty:new boolean[]{false,true})for(int fail:new int[]{0,1,2})for(int n:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){
 OrderedVoidEffects.trace="";OrderedVoidEffects.fail=fail;OrderedVoidCell cell=empty?null:new OrderedVoidCell();
 try{OrderedVoidRead value=new OrderedVoidOwner(cell,n).make();if(empty||fail!=0||!value.getClass().isAnonymousClass()||value.get()!=n||!OrderedVoidEffects.trace.equals("CD")||cell.length!=n)throw new AssertionError("void initializer value/identity/order");}
 catch(RuntimeException e){if(fail==1||!empty&&fail==2?e!=OrderedVoidEffects.error:(!empty||!(e instanceof NullPointerException)))throw new AssertionError("void initializer exception identity/phase",e);if(!OrderedVoidEffects.trace.equals(!empty&&fail==2?"CD":"C")||cell!=null&&cell.length!=(fail==2?n:17))throw new AssertionError("void initializer call/commit order");}
 rows++;
 }System.out.println(rows+":ordered:void:initializer");}}
`

func TestAdversarialOrderedVoidInitializerKeepsOriginalCallOrder(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			for _, shape := range []string{"void-only", "mixed-field", "virtual-call", "long-words"} {
				t.Run(shape, func(t *testing.T) {
					source, owner := orderedVoidInitializerFixture, "OrderedVoidOwner"
					if shape == "mixed-field" {
						source = strings.Replace(source, "interface OrderedVoidRead {int get();}", "interface OrderedVoidRead {int get();int stamp();}", 1)
						source = strings.Replace(source, "implements OrderedVoidRead {public int get(){return 99;}", "implements OrderedVoidRead {public int get(){return 99;}public int stamp(){return 99;}", 1)
						source = strings.Replace(source, "public int get(){return cell.length;}", "public int get(){return cell.length;}public int stamp(){return completed;}", 1)
						source = strings.Replace(source, "value.get()!=n||", "value.get()!=n||value.stamp()!=7||", 1)
						source = strings.Replace(source, "static void finish(){", "static int finish(){", 1)
						source = strings.Replace(source, "if(fail==2)throw error;}\n}", "if(fail==2)throw error;return 7;}\n}", 1)
						source = strings.Replace(source, "OrderedVoidEffects.finish();}public int get()", "}final int completed=OrderedVoidEffects.finish();public int get()", 1)
					}
					if shape == "virtual-call" {
						source = strings.Replace(source, "OrderedVoidEffects.observe(this,cell,number);", "this.begin(cell,number);", 1)
						source = strings.Replace(source, "public int get(){return cell.length;}", "public void begin(OrderedVoidCell c,int n){OrderedVoidEffects.observe(this,c,n);}public int get(){return cell.length;}", 1)
					}
					if shape == "long-words" {
						source = strings.ReplaceAll(source, "int get()", "long get()")
						source = strings.ReplaceAll(source, "int length=17", "long length=17")
						source = strings.ReplaceAll(source, "Cell cell,int n", "Cell cell,long n")
						source = strings.ReplaceAll(source, "private int number", "private long number")
						source = strings.Replace(source, "for(int n:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})", "for(long n:new long[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE})", 1)
					}
					if rename == "renamed" {
						source = strings.NewReplacer("OrderedVoidOwner", "ExecutionEnvelope", "observe", "inspect", "length", "extent").Replace(source)
						owner = "ExecutionEnvelope"
					}
					testNativePrivateSetterFixture(t, source, owner, "OrderedVoidDriver", "24:ordered:void:initializer\n")
				})
			}
		})
	}
}
