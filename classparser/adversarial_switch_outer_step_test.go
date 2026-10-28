package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSwitchKeepsValueJoinBeforeOuterStep(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class NestedSelector {
 public static int cursor,visits,commits,groups,score;public static int[]data;
 public static int next(){visits++;return cursor<data.length?data[cursor++]:0;}
 public static void collect(boolean escape,boolean filter){
  while(cursor<data.length){int token=next();
   if(token!=5){inner:while(token!=0){
    if(filter&&token==4){token=next();continue;}
    int value;
    switch(token){case 1:value=11;break;case 2:if(escape)break inner;value=13;break;case 3:if(escape)break inner;value=17;break;case 4:value=0;break;default:value=token*7;break;}
    score=score*31+value;commits++;token=next();
   }}groups++;
  }
 }
}
class NestedSelectorDriver{public static void main(String[]args){int rows=0;
 for(int size=0,powers=1;size<=5;size++,powers*=6)for(int bits=0;bits<powers;bits++)for(boolean escape:new boolean[]{false,true})for(boolean filter:new boolean[]{false,true}){
  int[]data=new int[size];int word=bits;for(int i=0;i<size;i++){data[i]=word%6;word/=6;}
  int cursor=0,visits=0,commits=0,groups=0,score=0;boolean inside=false;
  while(inside||cursor<size){int token=cursor<size?data[cursor++]:0;visits++;
   if(!inside&&token==5){groups++;continue;}inside=true;
   if(token==0||(token==2||token==3)&&escape){inside=false;groups++;continue;}
   if(filter&&token==4)continue;int value=token==1?11:token==2?13:token==3?17:token==4?0:token*7;score=score*31+value;commits++;
  }
  NestedSelector.data=data;NestedSelector.cursor=NestedSelector.visits=NestedSelector.commits=NestedSelector.groups=NestedSelector.score=0;
  NestedSelector.collect(escape,filter);
  if(NestedSelector.cursor!=cursor||NestedSelector.visits!=visits||NestedSelector.commits!=commits||NestedSelector.groups!=groups||NestedSelector.score!=score)throw new AssertionError(size+":"+bits+":"+escape+":"+filter);rows++;
 }System.out.println(rows+":switch:outer-step");}}
`
	for _, abrupt := range []bool{false, true} {
		label, text := "value", source
		if abrupt {
			label = "throw"
			text = strings.ReplaceAll(text, "public static int cursor", `public static final RuntimeException FAILURE=new RuntimeException("token"); public static int cursor`)
			text = strings.ReplaceAll(text, "default:value=token*7;break;", "default:throw FAILURE;")
			text = strings.ReplaceAll(text, "boolean inside=false;", "boolean inside=false,failed=false;")
			text = strings.ReplaceAll(text, "if(filter&&token==4)continue;int value", "if(filter&&token==4)continue;if(token==5){failed=true;break;}int value")
			text = strings.ReplaceAll(text, "NestedSelector.collect(escape,filter);", "Throwable error=null;try{NestedSelector.collect(escape,filter);}catch(Throwable e){error=e;}if(error!=(failed?NestedSelector.FAILURE:null))throw new AssertionError(\"failure identity\",error);")
		}
		t.Run(label, func(t *testing.T) {
			for _, owner := range []string{"NestedSelector", "GroupedReader"} {
				t.Run(owner, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "NestedSelector", owner), owner, owner+"Driver", "37324:switch:outer-step\n")
				})
			}
		})
	}
}
