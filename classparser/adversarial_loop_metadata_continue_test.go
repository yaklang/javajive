package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialMetadataBranchKeepsSharedContinue(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `public class RouteReader {
 public int updates,keys,values,commits,score;public static final RuntimeException FAILURE=new RuntimeException("effect");
 public int update(int n,int fail){updates++;if(fail==1)throw FAILURE;return score=n*7+1;}
 public int key(int n,int fail){keys++;if(fail==2)throw FAILURE;return n;}
 public int value(int n,int fail){values++;if(fail==3)throw FAILURE;return n==2?0:n*11;}
 public void commit(int k,int v,int fail){commits++;if(fail==4)throw FAILURE;score=score*31+k*7+v;}
 public int read(int[]input,boolean typed,boolean mode,boolean change,boolean skip,int fail){
  for(int i=0;i<input.length;i++){
   int k;
   if(input[i]==0){k=0;}
   else if(typed){k=key(input[i],fail);if(i==0&&mode&&k==1){if(change)update(input[i],fail);continue;}}
   else if(i==0&&mode&&input[i]<3){int s=key(input[i],fail);if(s==1){if(change)update(s,fail);continue;}else{k=s+5;}}
   else{k=key(input[i],fail);}
   int v=value(input[i],fail);if(v==0&&skip)continue;commit(k,v,fail);
  }return score;
 }
}
class RouteReaderDriver{public static void main(String[]args){int rows=0;
 for(int size=0,power=1;size<=4;size++,power*=4)for(int word=0;word<power;word++)for(boolean typed:new boolean[]{false,true})for(boolean mode:new boolean[]{false,true})for(boolean change:new boolean[]{false,true})for(boolean skip:new boolean[]{false,true})for(int fail=0;fail<5;fail++){
 int[]input=new int[size];int bits=word;for(int i=0;i<size;i++){input[i]=bits%4;bits/=4;}
 int score=0,keys=0,updates=0,values=0,commits=0;Throwable want=null;
 for(int i=0;i<size;i++){int n=input[i],k=0;if(n!=0){keys++;if(fail==2){want=RouteReader.FAILURE;break;}boolean metadata=i==0&&mode&&n==1;if(metadata){if(change){updates++;if(fail==1){want=RouteReader.FAILURE;break;}score=n*7+1;}continue;}k=(!typed&&i==0&&mode&&n<3)?n+5:n;}
 values++;if(fail==3){want=RouteReader.FAILURE;break;}int v=n==2?0:n*11;if(v==0&&skip)continue;commits++;if(fail==4){want=RouteReader.FAILURE;break;}score=score*31+k*7+v;}
 RouteReader x=new RouteReader();Throwable got=null;int result=0;try{result=x.read(input,typed,mode,change,skip,fail);}catch(Throwable e){got=e;}
 if(got!=want||got==null&&result!=score||x.score!=score||x.keys!=keys||x.updates!=updates||x.values!=values||x.commits!=commits)throw new AssertionError(size+":"+word+":"+typed+":"+mode+":"+change+":"+skip+":"+fail);rows++;
 }System.out.println(rows+":metadata:continue:effects");}}
`
	for _, step := range []string{"unit", "stride", "descending"} {
		t.Run(step, func(t *testing.T) {
			variant := source
			if step == "stride" {
				variant = strings.ReplaceAll(variant, "i<input.length;i++", "i<input.length;i+=2")
				variant = strings.ReplaceAll(variant, "i<size;i++){int n=", "i<size;i+=2){int n=")
			}
			if step == "descending" {
				variant = strings.ReplaceAll(variant, "int i=0;i<input.length;i++", "int i=input.length-1;i>=0;i--")
				variant = strings.ReplaceAll(variant, "int i=0;i<size;i++){int n=", "int i=size-1;i>=0;i--){int n=")
			}
			for _, owner := range []string{"RouteReader", "HeaderConsumer"} {
				t.Run(owner, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(variant, "RouteReader", owner), owner, owner+"Driver", "27280:metadata:continue:effects\n")
				})
			}
		})
	}
}
