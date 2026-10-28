package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialProtectedSkipBypassesUnassignedCommit(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class GuardedCollector {
 public static int reads,converted,committed,caught,failAt,readFailAt,commitFailAt;public static final RuntimeException FAILURE=new RuntimeException("convert");
 public static final RuntimeException READ_FAILURE=new RuntimeException("read");
 public static final RuntimeException COMMIT_FAILURE=new RuntimeException("commit");
 public static int store(Object value,int score){committed++;if(committed==commitFailAt)throw COMMIT_FAILURE;return score*31+((Integer)value).intValue();}
 public static int next(int[] data){reads++;if(reads==readFailAt)throw READ_FAILURE;return reads<=data.length?data[reads-1]:-1;}
 public static Object convert(int n){converted++;if(converted==failAt)throw FAILURE;return Integer.valueOf(n*7);}
 public static int collect(int[] data,boolean skip,boolean typed){int score=0;
  for(int item=next(data);item>=0;item=next(data)){
   if(item==5)continue;
   try{Object value;
    if(item==0){if(skip)continue;value=convert(2);}
    else{if(typed)value=convert(item+1);else value=convert(item);}
    score=store(value,score);
   }catch(RuntimeException failure){caught++;if(failure!=FAILURE)throw failure;return score;}
  }return score;
 }
}
class GuardedCollectorDriver{public static void main(String[]args){int rows=0;
 for(int size=0,powers=1;size<=4;size++,powers*=6)for(int bits=0;bits<powers;bits++)for(boolean skip:new boolean[]{false,true})for(boolean typed:new boolean[]{false,true})for(int failAt:new int[]{0,1,3})for(int readFailAt:new int[]{0,1,3})for(int commitFailAt:new int[]{0,1,3}){
  int[]data=new int[size];int word=bits;for(int i=0;i<size;i++){data[i]=word%6;word/=6;}
  int score=0,reads=0,converted=0,committed=0,caught=0;boolean readFailed=false,commitFailed=false;
  for(int i=0;i<=size;i++){reads++;if(reads==readFailAt){readFailed=true;break;}if(i==size)break;int item=data[i];if(item==5||item==0&&skip)continue;converted++;if(converted==failAt){caught++;break;}committed++;if(committed==commitFailAt){commitFailed=true;if(COMMIT_IS_PROTECTED)caught++;break;}score=score*31+(item==0?2:typed?item+1:item)*7;}
  GuardedCollector.reads=GuardedCollector.converted=GuardedCollector.committed=GuardedCollector.caught=0;GuardedCollector.failAt=failAt;GuardedCollector.readFailAt=readFailAt;GuardedCollector.commitFailAt=commitFailAt;
  int result=0;Throwable error=null;try{result=GuardedCollector.collect(data,skip,typed);}catch(Throwable x){error=x;}
  if(error!=(readFailed?GuardedCollector.READ_FAILURE:commitFailed?GuardedCollector.COMMIT_FAILURE:null)||!readFailed&&!commitFailed&&result!=score||GuardedCollector.reads!=reads||GuardedCollector.converted!=converted||GuardedCollector.committed!=committed||GuardedCollector.caught!=caught)throw new AssertionError(size+":"+bits+":"+skip+":"+typed+":"+failAt);rows++;
 }System.out.println(rows+":protected-skip:commit");}}
`
	for _, placement := range []string{"inside", "outside", "resuming"} {
		text := strings.ReplaceAll(source, "COMMIT_IS_PROTECTED", "true")
		if placement == "outside" {
			text = strings.ReplaceAll(source, "COMMIT_IS_PROTECTED", "false")
			text = strings.ReplaceAll(text, "try{Object value;", "Object value;try{")
			text = strings.ReplaceAll(text, "score=store(value,score);", "")
			text = strings.ReplaceAll(text, "return score;}\n  }return score;", "return score;}score=store(value,score);\n  }return score;")
		}
		if placement == "resuming" {
			text = strings.ReplaceAll(text, `public static final RuntimeException FAILURE=new RuntimeException("convert");`, `public static final IllegalArgumentException FAILURE=new IllegalArgumentException("convert");`)
			text = strings.ReplaceAll(text, `catch(RuntimeException failure){caught++;if(failure!=FAILURE)throw failure;return score;}`, `catch(IllegalArgumentException failure){caught++;}catch(RuntimeException failure){caught++;if(failure!=COMMIT_FAILURE)throw failure;}`)
			text = strings.ReplaceAll(text, `if(converted==failAt){caught++;break;}`, `if(converted==failAt){caught++;continue;}`)
			text = strings.ReplaceAll(text, `if(committed==commitFailAt){commitFailed=true;if(true)caught++;break;}`, `if(committed==commitFailAt){caught++;continue;}`)
		}
		t.Run(placement, func(t *testing.T) {
			for _, owner := range []string{"GuardedCollector", "FilteredAccumulator"} {
				t.Run(owner, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "GuardedCollector", owner), owner, owner+"Driver", "167940:protected-skip:commit\n")
				})
			}
		})
	}
}
