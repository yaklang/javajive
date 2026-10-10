package javaclassparser

import (
	"strings"
	"testing"
)

const foreignInitializerStoreFixture = `interface ForeignStoreRead {int get();int stamp();}
class ForeignStoreCell {int length=17;}
class ForeignStoreEffects {
 static String trace="";static int fail;static final RuntimeException error=new RuntimeException("original");
 static ForeignStoreCell receiver(ForeignStoreCell cell){trace+="R";if(fail==1)throw error;return cell;}
 static int rhs(int n){trace+="V";if(fail==2)throw error;return n;}
 static int finish(){trace+="D";return 7;}
}
class ForeignStoreOwner {private ForeignStoreCell cell;private int number;ForeignStoreOwner(ForeignStoreCell cell,int n){this.cell=cell;number=n;}
 ForeignStoreRead make(){return new ForeignStoreRead(){final ForeignStoreCell target=ForeignStoreEffects.receiver(cell);
 {target.length=ForeignStoreEffects.rhs(number);}
 final int completed=ForeignStoreEffects.finish();public int get(){return target.length;}public int stamp(){return completed;}};}
}
class ForeignStoreDriver {public static void main(String[]args){int rows=0;for(boolean empty:new boolean[]{false,true})for(int fail:new int[]{0,1,2})for(int n:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){
 ForeignStoreEffects.trace="";ForeignStoreEffects.fail=fail;ForeignStoreCell cell=empty?null:new ForeignStoreCell();
 try{ForeignStoreRead value=new ForeignStoreOwner(cell,n).make();if(empty||fail!=0||!value.getClass().isAnonymousClass()||value.get()!=n||value.stamp()!=7||!ForeignStoreEffects.trace.equals("RVD")||cell.length!=n)throw new AssertionError("foreign store value/identity/order");}
 catch(RuntimeException e){if(fail==0?(!empty||!(e instanceof NullPointerException)):e!=ForeignStoreEffects.error)throw new AssertionError("foreign store exception identity/phase",e);if(!ForeignStoreEffects.trace.equals(fail==1?"R":"RV")||cell!=null&&cell.length!=17)throw new AssertionError("foreign store receiver/RHS/store order");}
 rows++;
 }System.out.println(rows+":foreign:initializer:store");}}
`

func TestAdversarialAnonymousForeignInitializerStoreKeepsOriginalEffects(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			for _, shape := range []string{"int", "same-type-distinct-targets", "volatile-int", "long", "float", "double", "byte", "short", "char", "boolean"} {
				t.Run(shape, func(t *testing.T) {
					source, owner, want := foreignInitializerStoreFixture, "ForeignStoreOwner", "24:foreign:initializer:store\n"
					if shape == "same-type-distinct-targets" {
						source = strings.Replace(source, "private ForeignStoreCell cell;", "private ForeignStoreCell cell;private ForeignStoreCell other;ForeignStoreCell other(){return other;}", 1)
						source = strings.Replace(source, "this.cell=cell;number=n;", "this.cell=cell;other=new ForeignStoreCell();number=n;", 1)
						source = strings.Replace(source, "{target.length=ForeignStoreEffects.rhs(number);}", "final ForeignStoreCell decoy=ForeignStoreEffects.receiver(other);{target.length=ForeignStoreEffects.rhs(number);}", 1)
						source = strings.Replace(source, "try{ForeignStoreRead value=new ForeignStoreOwner(cell,n).make();", "ForeignStoreOwner owner=new ForeignStoreOwner(cell,n);try{ForeignStoreRead value=owner.make();", 1)
						source = strings.ReplaceAll(source, `"RVD"`, `"RRVD"`)
						source = strings.ReplaceAll(source, `fail==1?"R":"RV"`, `fail==1?"R":"RRV"`)
						source = strings.Replace(source, "rows++;", `if(owner.other().length!=17)throw new AssertionError("foreign store same-type receiver identity");rows++;`, 1)
					} else if shape == "volatile-int" {
						source = strings.Replace(source, "int length=17", "volatile int length=17", 1)

					} else if shape == "byte" || shape == "short" || shape == "char" {
						source = strings.ReplaceAll(source, "int get()", shape+" get()")
						source = strings.Replace(source, "int length=17", shape+" length=17", 1)
						source = strings.Replace(source, "target.length=ForeignStoreEffects.rhs(number)", "target.length=("+shape+")ForeignStoreEffects.rhs(number)", 1)
						source = strings.ReplaceAll(source, "value.get()!=n", "value.get()!=("+shape+")n")
						source = strings.ReplaceAll(source, "cell.length!=n", "cell.length!=("+shape+")n")
						source = strings.Replace(source, "Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE", "Integer.MIN_VALUE,-65537,-32769,-129,128,32768,65536,Integer.MAX_VALUE", 1)
						want = "48:foreign:initializer:store\n"
					} else if shape == "boolean" {
						source = strings.ReplaceAll(source, "int get()", "boolean get()")
						source = strings.ReplaceAll(source, "int length=17", "boolean length=false")
						source = strings.ReplaceAll(source, "static int rhs(int n)", "static boolean rhs(boolean n)")
						source = strings.ReplaceAll(source, "private int number", "private boolean number")
						source = strings.ReplaceAll(source, "ForeignStoreCell cell,int n", "ForeignStoreCell cell,boolean n")
						source = strings.ReplaceAll(source, "for(int n:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})", "for(boolean n:new boolean[]{false,true})")
						source = strings.ReplaceAll(source, "cell.length!=17", "cell.length")
						want = "12:foreign:initializer:store\n"
					} else if shape != "int" {
						source = strings.ReplaceAll(source, "int get()", shape+" get()")
						source = strings.ReplaceAll(source, "int length=17", shape+" length=17")
						source = strings.ReplaceAll(source, "static int rhs(int n)", "static "+shape+" rhs("+shape+" n)")
						source = strings.ReplaceAll(source, "private int number", "private "+shape+" number")
						source = strings.ReplaceAll(source, "ForeignStoreCell cell, int n", "ForeignStoreCell cell, "+shape+" n")
						source = strings.ReplaceAll(source, "ForeignStoreCell cell,int n", "ForeignStoreCell cell,"+shape+" n")
						bank := `Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE`
						if shape == "float" {
							bank = `0.0f,-0.0f,Float.intBitsToFloat(0x7fc12345),Float.POSITIVE_INFINITY,Float.MAX_VALUE`
							want = "30:foreign:initializer:store\n"
							source = strings.ReplaceAll(source, "value.get()!=n", "Float.floatToRawIntBits(value.get())!=Float.floatToRawIntBits(n)")
							source = strings.ReplaceAll(source, "cell.length!=n", "Float.floatToRawIntBits(cell.length)!=Float.floatToRawIntBits(n)")
						}
						if shape == "double" {
							bank = `0.0,-0.0,Double.longBitsToDouble(0x7ff8123456789abcL),Double.NEGATIVE_INFINITY,Double.MAX_VALUE`
							want = "30:foreign:initializer:store\n"
							source = strings.ReplaceAll(source, "value.get()!=n", "Double.doubleToRawLongBits(value.get())!=Double.doubleToRawLongBits(n)")
							source = strings.ReplaceAll(source, "cell.length!=n", "Double.doubleToRawLongBits(cell.length)!=Double.doubleToRawLongBits(n)")
						}
						source = strings.Replace(source, "for(int n:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})", "for("+shape+" n:new "+shape+"[]{"+bank+"})", 1)
					}
					if rename == "renamed" {
						source = strings.NewReplacer("ForeignStoreOwner", "MutationEnvelope", "length", "extent").Replace(source)
						owner = "MutationEnvelope"
					}
					testNativePrivateSetterFixture(t, source, owner, "ForeignStoreDriver", want)
				})
			}
		})
	}
}

func TestAdversarialAnonymousForeignPrimitiveStoreAcrossPackages(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner := foreignInitializerStoreFixture, "consumer/ForeignStoreOwner"
			source = strings.Replace(source, "class ForeignStoreCell {int length=17;}", "", 1)
			cell := "package cells;public class ForeignStoreCell {public int length=17;}"
			if rename == "renamed" {
				source = strings.NewReplacer("ForeignStoreOwner", "MutationEnvelope", "length", "extent").Replace(source)
				cell = strings.ReplaceAll(cell, "length", "extent")
				owner = "consumer/MutationEnvelope"
			}
			testNativePrivateSetterSourceFixture(t, map[string]string{"cells/ForeignStoreCell.java": cell, owner + ".java": "package consumer;import cells.ForeignStoreCell;\n" + source}, owner, "consumer.ForeignStoreDriver", "24:foreign:initializer:store\n")
		})
	}
}
