package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialCaughtSelectionSkipsUnassignedContinuation(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class CandidatePicker {
 public static int attempted,scored,caught,metadata;
 public static String hints(int n){metadata++;return (n&1)==0?null:"hint";}
 public static Integer create(int n,int rejected){attempted++;if(n==rejected)throw new IllegalArgumentException("reject");return n*11;}
 public static int choose(int[] inputs,int rejected,boolean direct,boolean lenient,int stride,boolean reverse){int best=Integer.MAX_VALUE;int chosen=-1;
  java.util.List<RuntimeException> failures=null;
  for(int cursor=reverse?inputs.length-1:0;reverse?cursor>=0:cursor<inputs.length;cursor+=reverse?-stride:stride){int n=inputs[cursor];if(n<0)continue;Integer item;
   if(!direct){try{String hint=hints(n);if(hint==null){String fallback="f";if(n>0)hint=fallback;}item=create(n,rejected);}
    catch(IllegalArgumentException failure){caught++;if(failures==null)failures=new java.util.ArrayList<RuntimeException>();failures.add(failure);continue;}}
   else{if(n==rejected)continue;item=Integer.valueOf(n*11);}
   scored++;int weight=lenient?item.intValue():item.intValue()+7;
   if(weight<best){chosen=n;best=weight;}
  }return chosen;
 }
}
class CandidatePickerDriver{public static void main(String[]args){int count=0;
 for(int length=0;length<9;length++)for(int rejected=-1;rejected<9;rejected++)for(boolean direct:new boolean[]{false,true})for(boolean lenient:new boolean[]{false,true})for(int stride:new int[]{1,2,5})for(boolean reverse:new boolean[]{false,true}){
 int[] inputs=new int[length];int best=Integer.MAX_VALUE,want=-1,attempts=0,scores=0,failures=0,meta=0;
 for(int i=0;i<length;i++)inputs[i]=(i*5+length)%11-2;
 for(int visit=0;visit<length;visit+=stride){int n=inputs[reverse?length-1-visit:visit];if(n<0)continue;if(!direct){attempts++;meta++;if(n==rejected){failures++;continue;}}else if(n==rejected)continue;scores++;int weight=n*11+(lenient?0:7);if(weight<best){best=weight;want=n;}}
 CandidatePicker.attempted=CandidatePicker.scored=CandidatePicker.caught=CandidatePicker.metadata=0;
 int actual=CandidatePicker.choose(inputs,rejected,direct,lenient,stride,reverse);if(actual!=want||CandidatePicker.attempted!=attempts||CandidatePicker.scored!=scores||CandidatePicker.caught!=failures||CandidatePicker.metadata!=meta)throw new AssertionError("candidate effects");count++;
 }System.out.println(count+":catch:continue");}}
`
	for _, name := range []string{"CandidatePicker", "ScoreSelector"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "CandidatePicker", name), name, name+"Driver", "2160:catch:continue\n")
		})
	}
}
