package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialParameterFallbackRetainsIncomingIdentity(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class ParameterFallback {
 public static java.util.Map<String,Integer> choose(Object first,Object second,java.util.Map<String,Integer> supplied,java.util.Comparator<String> order){
  supplied=supplied==null?new java.util.TreeMap<String,Integer>():supplied;
  order=order==null?java.util.Comparator.<String>naturalOrder():order;
  return finish(first,second,null,supplied,order);
 }
 public java.util.Map<String,Integer> chooseInstance(long prefix,double wide,java.util.Map<String,Integer> supplied){
  supplied=supplied==null?new java.util.TreeMap<String,Integer>():supplied;return supplied;
 }
 public static int allocations;
 public static CharSequence[] allocate(){allocations++;return new String[]{"fresh"};}
 public static CharSequence[] chooseArray(long prefix,double wide,CharSequence[] supplied){supplied=supplied==null?allocate():supplied;return supplied;}
 public static java.util.Map<String,Integer> finish(Object first,Object second,Object ignored,java.util.Map<String,Integer> supplied,java.util.Comparator<String> order){if(order.compare("a","b")==0)throw new AssertionError();return supplied;}
}
class ParameterFallbackDriver {public static void main(String[]args){int checks=0;
 for(int size=0;size<12;size++)for(int kind=0;kind<4;kind++)for(boolean custom:new boolean[]{false,true}){
  java.util.Map<String,Integer> given=kind==0?null:kind==1?new java.util.HashMap<String,Integer>():kind==2?new java.util.LinkedHashMap<String,Integer>():new java.util.TreeMap<String,Integer>();
  if(given!=null)for(int i=0;i<size;i++)given.put("x"+i,i*17-31);
  java.util.Comparator<String> order=custom?java.util.Collections.<String>reverseOrder():null;
  java.util.Map<String,Integer> result=ParameterFallback.choose(new Object(),new Object(),given,order);
  if(given!=null){if(result!=given||result.size()!=size)throw new AssertionError("incoming identity");for(int i=0;i<size;i++)if(result.get("x"+i).intValue()!=i*17-31)throw new AssertionError("contents");}
  else if(result==null||!result.isEmpty()||!(result instanceof java.util.TreeMap))throw new AssertionError("fallback allocation");checks++;
  java.util.Map<String,Integer> instance=new ParameterFallback().chooseInstance(177L,-9.25,given);
  if(given!=null?instance!=given:instance==null||!instance.isEmpty())throw new AssertionError("wide instance parameter");checks++;
  CharSequence[] array=kind==0?null:kind==1?new String[]{"retained"}:kind==2?new StringBuilder[]{new StringBuilder("mutable")}:new CharSequence[0];
  ParameterFallback.allocations=0;CharSequence[] chosen=ParameterFallback.chooseArray(-2147483649L,17.75,array);
  if(array!=null?chosen!=array:chosen==null||chosen.length!=1||!chosen[0].equals("fresh"))throw new AssertionError("array identity");
  if(ParameterFallback.allocations!=(array==null?1:0))throw new AssertionError("lazy allocation");checks++;
 }System.out.println(checks+":parameter:fallback:identity");}}
`
	for _, name := range []string{"ParameterFallback", "MapSelection"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "ParameterFallback", name), name, name+"Driver", "288:parameter:fallback:identity\n")
		})
	}
}
