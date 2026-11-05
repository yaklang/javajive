package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialCapturedMapperKeepsObjectJoin(t *testing.T) {
	const source = `
import java.util.*;import java.util.function.*;import java.util.stream.*;
public class SelectedObjects {
 public static Object select(List<Integer> input,int bias,StringBuilder trace){
  Object result=input==null||input.isEmpty()?new ArrayList<Integer>():input.stream().filter(Objects::nonNull).map(x->{trace.append(x).append(';');return x+bias;}).collect(Collectors.toList());return result;
 }
 public static Object choose(boolean first,Object token,boolean fail,RuntimeException marker,StringBuilder trace){
  Object result=first?new int[]{3,5}:((Supplier<Object>)(()->{trace.append("S;");if(fail)throw marker;return token;})).get();return result;
 }
 public static Object raw(List input,Object token,StringBuilder trace){
  Object result=input==null||input.isEmpty()?new LinkedList<Object>():input.stream().filter(Objects::nonNull).map(x->{trace.append(x).append(';');return token;}).collect(Collectors.toList());return result;
 }
}
class SelectedObjectsDriver {public static void main(String[]args){int rows=0;Object marker=new Object();
for(int length=-1;length<9;length++)for(int bias:new int[]{-99,-1,0,1,97})for(int pattern=0;pattern<4;pattern++){
 List<Integer> input=length<0?null:new ArrayList<Integer>();List<Integer> expected=new ArrayList<Integer>();StringBuilder want=new StringBuilder();if(input!=null)for(int i=0;i<length;i++){Integer x=(i+pattern)%4==0?null:i-3;input.add(x);if(x!=null){expected.add(x+bias);want.append(x).append(';');}}
 StringBuilder trace=new StringBuilder();Object result=SelectedObjects.select(input,bias,trace);if(!(result instanceof List)||!expected.equals(result)||!want.toString().equals(trace.toString()))throw new AssertionError("typed join");
 trace.setLength(0);Object raw=SelectedObjects.raw(input,marker,trace);if(!(raw instanceof List)||((List)raw).size()!=expected.size()||!want.toString().equals(trace.toString()))throw new AssertionError("erased join");for(Object x:(List)raw)if(x!=marker)throw new AssertionError("capture identity");rows++;}
RuntimeException failure=new IllegalStateException("identity");
for(Object token:new Object[]{null,marker,"text",new Object(),new ArrayList<Object>(),new LinkedList<Object>(),new int[0],new StringBuilder()})for(boolean first:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}){
 StringBuilder trace=new StringBuilder();Object result=null;RuntimeException thrown=null;try{result=SelectedObjects.choose(first,token,fail,failure,trace);}catch(RuntimeException e){thrown=e;}
 if(!first&&fail){if(thrown!=failure)throw new AssertionError("exception identity");}else{if(thrown!=null)throw new AssertionError("unexpected exception",thrown);if(first){if(!(result instanceof int[])||!Arrays.equals((int[])result,new int[]{3,5}))throw new AssertionError("array branch");}else if(result!=token)throw new AssertionError("arbitrary object identity");}
 if(!trace.toString().equals(first?"":"S;"))throw new AssertionError("selected supplier trace");rows++;}
System.out.println(rows+":object-join:captured-mapper:order:identity");}}
`
	for _, name := range []string{"SelectedObjects", "RenamedSelection"} {
		t.Run(name, func(t *testing.T) {
			text := strings.ReplaceAll(source, "SelectedObjects", name)
			testAuthoredOwnerAcrossSourceModes(t, text, name, name+"Driver", "232:object-join:captured-mapper:order:identity\n")
		})
	}
}
