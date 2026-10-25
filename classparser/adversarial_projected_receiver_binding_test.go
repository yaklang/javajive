package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialProjectedReceiverFieldStoresKeepDeclaredBinder(t *testing.T) {
	fixture := `class SuffixOwner<T extends Comparable<T>> {
 class Node {T value;Node left,right;Node(T value){this.value=value;}
  void swapLeft(){T previous=value;value=left.value;left.value=previous;}
  void swapRight(){T previous=value;value=right.value;right.value=previous;}
 }
 Node make(T a,T b,T c){Node n=new Node(a);n.left=new Node(b);n.right=new Node(c);return n;}
}
class SuffixDriver {public static void main(String[]args){int rows=0;
 for(String a:new String[]{null,"",new String("alpha")})for(String b:new String[]{null,new String("beta"),"中文"})for(String c:new String[]{null,"gamma"}){
 SuffixOwner<String>.Node n=new SuffixOwner<String>().make(a,b,c);
 n.swapLeft();if(n.value!=b||n.left.value!=a||n.right.value!=c)throw new AssertionError("left exchange");
 n.swapRight();if(n.value!=c||n.left.value!=a||n.right.value!=b)throw new AssertionError("right exchange");
 n.left=n;n.swapLeft();if(n.value!=c||n.left!=n||n.right.value!=b)throw new AssertionError("alias exchange");rows++;
 }System.out.println(rows+":receiver:field:exchange:alias");}}
`
	for _, root := range []string{"SuffixOwner", "RenamedReceiverScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []struct {
				target   int
				compiler SourceCompilerProfile
			}{{8, ModernJavac}, {21, ModernJavac}, {8, NativeJavac8}} {
				t.Run(fmt.Sprintf("%s/%s/%s/%d", root, debug, profile.compiler, profile.target), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Node"}, "18:receiver:field:exchange:alias\n", 8, profile.target, profile.compiler)
				})
			}
		}
	}
}

func TestAdversarialProjectedReceiverGenericCallKeepsArgumentBinder(t *testing.T) {
	fixture := `class SuffixOwner<K,V,R> {
 interface Conversion<A,B>{B encode(A value);A decode(B value);}
 final Conversion<V,R> conversion;
 SuffixOwner(Conversion<V,R> conversion){this.conversion=conversion;}
 class Entry implements java.util.Map.Entry<K,V>{
  final java.util.Map.Entry<K,R> storage;Entry(java.util.Map.Entry<K,R> storage){this.storage=storage;}
  public K getKey(){return storage.getKey();}
  public V getValue(){return conversion.decode(storage.getValue());}
  public V setValue(V value){R previous=storage.setValue(conversion.encode(value));return previous==null?null:conversion.decode(previous);}
 }
 Entry make(java.util.Map.Entry<K,R> storage){return new Entry(storage);}
}
class SuffixDriver {static String trace="";public static void main(String[]args){int rows=0;
 SuffixOwner.Conversion<String,Integer> conversion=new SuffixOwner.Conversion<String,Integer>(){
  public Integer encode(String s){trace+="E";return s==null?null:Integer.valueOf(s);}
  public String decode(Integer n){trace+="D";return n==null?null:n.toString();}
 };
 for(Integer old:new Integer[]{null,Integer.valueOf(Integer.MIN_VALUE),Integer.valueOf(7)})for(String next:new String[]{null,"8","-7","invalid"}){
 java.util.Map.Entry<String,Integer> storage=new java.util.AbstractMap.SimpleEntry<String,Integer>("key",old);
 SuffixOwner<String,String,Integer>.Entry entry=new SuffixOwner<String,String,Integer>(conversion).make(storage);trace="";
 try{String previous=entry.setValue(next);if("invalid".equals(next))throw new AssertionError("missing failure");
 if(!java.util.Objects.equals(previous,old==null?null:old.toString())||!java.util.Objects.equals(storage.getValue(),next==null?null:Integer.valueOf(next))||!trace.equals(old==null?"E":"ED"))throw new AssertionError("argument/store/result/order");
 }catch(NumberFormatException expected){if(!"invalid".equals(next)||storage.getValue()!=old||!trace.equals("E"))throw new AssertionError("abrupt conversion before store");}
 rows++;}System.out.println(rows+":receiver:generic:argument:effects");}}
`
	for _, root := range []string{"SuffixOwner", "RenamedConversionScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []struct {
				target   int
				compiler SourceCompilerProfile
			}{{8, ModernJavac}, {21, ModernJavac}, {8, NativeJavac8}} {
				t.Run(fmt.Sprintf("%s/%s/%s/%d", root, debug, profile.compiler, profile.target), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Entry"}, "12:receiver:generic:argument:effects\n", 8, profile.target, profile.compiler)
				})
			}
		}
	}
}
