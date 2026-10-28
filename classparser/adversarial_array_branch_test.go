package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialArrayArmKeepsConstructionBeforeUse(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
import java.util.*;
public class ChoiceStore {
 public List<Object> result;public int cursor=99;public static int calls;public static boolean fail;public static final Object TOKEN=new Object();public static final RuntimeException FAILURE=new RuntimeException("input");
 public static Object fallback(){calls++;if(fail)throw FAILURE;return TOKEN;}
 public static <T> List<T> copy(List<T> x){return new ArrayList<T>(x);}
 public static <T> List<T> copy(T... x){return new ArrayList<T>(Arrays.asList(x));}
 public void choose(List<Object> input,Object direct){
  if(direct!=null){result=Collections.singletonList(direct);}
  else {List<Object> selected=input;result=__SELECTOR__;}
  cursor=0;
 }
}
class ChoiceStoreDriver{public static void main(String[]args){int rows=0;Object other=new Object();
 for(int kind=0;kind<4;kind++)for(boolean direct:new boolean[]{false,true})for(boolean fail:new boolean[]{false,true}){
  List<Object> input=kind==0?null:new ArrayList<Object>();if(kind>=2)input.add(other);if(kind==3)input.add(null);
  ChoiceStore owner=new ChoiceStore();List<Object> before=Collections.singletonList(new Object());owner.result=before;
  ChoiceStore.calls=0;ChoiceStore.fail=fail;Throwable error=null;try{owner.choose(input,direct?other:null);}catch(Throwable x){error=x;}
  boolean fallback=!direct&&kind<2;int calls=fallback?1:0;
  if(ChoiceStore.calls!=calls||error!=(fallback&&fail?ChoiceStore.FAILURE:null))throw new AssertionError("effects "+kind+":"+direct+":"+fail);
  if(error!=null){if(owner.result!=before||owner.cursor!=99)throw new AssertionError("abrupt field update");}
  else {if(owner.cursor!=0)throw new AssertionError("continuation");List<Object> expect=direct?Collections.singletonList(other):fallback?Collections.singletonList(ChoiceStore.TOKEN):input;if(!owner.result.equals(expect))throw new AssertionError("selection");if(!direct&&owner.result==input)throw new AssertionError("copy identity");}
  rows++;
 }System.out.println(rows+":array-arm:effects");}}
`
	for _, predicate := range []string{"selected!=null&&!selected.isEmpty()?copy(selected):__FALLBACK__", "selected==null||selected.isEmpty()?__FALLBACK__:copy(selected)"} {
		t.Run(predicate, func(t *testing.T) {
			for _, form := range []string{"copy(fallback())", "copy(new Object[]{fallback()})"} {
				t.Run(form, func(t *testing.T) {
					for _, name := range []string{"ChoiceStore", "RouteFree"} {
						t.Run(name, func(t *testing.T) {
							text := strings.ReplaceAll(source, "__SELECTOR__", predicate)
							text = strings.ReplaceAll(strings.ReplaceAll(text, "__FALLBACK__", form), "ChoiceStore", name)
							testAuthoredOwnerAcrossSourceModes(t, text, name, name+"Driver", "16:array-arm:effects\n")
						})
					}
				})
			}
		})
	}
}
