package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialContainerSwitchKeepsEachTransferOwner(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class StreamFold{
 public static int[] data,stack;public static int index,reads,moves,depth,pops,failAt;public static final RuntimeException FAILURE=new RuntimeException("pop");
 public static int read(){reads++;return index<data.length?data[index++]:1;}
 public static void push(int x){stack[depth++]=x;moves++;}
 public static int pop(){pops++;if(pops==failAt)throw FAILURE;return depth==0?-1:stack[--depth];}
 public static int fold(int mode,boolean skip){int score=0;
 outer:do{
  if(mode==0){for(int token=read();token!=1;token=read()){int value;
   switch(token){case 0:push(mode);mode=1;continue outer;case 6:push(mode);mode=0;continue;
    case 2:value=11;break;case 3:if(skip)continue;value=13;break;case 4:value=17;break;default:value=token*3;break;}
   score=score*31+value;
  }}else{array:while(true){int token=read();switch(token){
   case 0:push(mode);mode=0;continue outer;case 6:push(mode);mode=1;continue outer;case 1:break array;
   case 2:score=score*31+111;continue;case 3:if(!skip)score=score*31+113;continue;
   case 4:score=score*31+117;continue;default:score=score*31+token*3+100;continue;
  }}}
  mode=pop();
 }while(mode>=0);return score;}
}
class StreamFoldDriver{public static void main(String[]args){int rows=0;
 for(int size=0,powers=1;size<=4;size++,powers*=7)for(int bits=0;bits<powers;bits++)for(int initial=0;initial<2;initial++)for(boolean skip:new boolean[]{false,true})for(int failAt:new int[]{0,1,2}){
 int[] data=new int[size];int word=bits;for(int j=0;j<size;j++){data[j]=word%7;word/=7;}
 int expected=0,mode=initial,reads=0,moves=0,depth=0,pops=0;boolean failed=false;int[]stack=new int[size+1];
 while(mode>=0){int token=reads<size?data[reads]:1;reads++;
  if(token==1){pops++;if(pops==failAt){failed=true;break;}mode=depth==0?-1:stack[--depth];continue;}
  if(token==0||token==6){stack[depth++]=mode;moves++;if(token==0)mode=1-mode;continue;}
  if(token==3&&skip)continue;int value=token==2?11:token==3?13:token==4?17:token*3;expected=expected*31+value+(mode==0?0:100);
 }
 StreamFold.data=data;StreamFold.stack=new int[size+1];StreamFold.index=StreamFold.reads=StreamFold.moves=StreamFold.depth=0;
 StreamFold.pops=0;StreamFold.failAt=failAt;int actual=0;Throwable error=null;try{actual=StreamFold.fold(initial,skip);}catch(Throwable x){error=x;}if(error!=(failed?StreamFold.FAILURE:null)||!failed&&actual!=expected||StreamFold.reads!=reads||StreamFold.moves!=moves||StreamFold.depth!=depth||StreamFold.pops!=pops)throw new AssertionError(size+":"+bits+":"+initial+":"+skip+" score="+actual+"/"+expected+" reads="+StreamFold.reads+"/"+reads);rows++;
 }System.out.println(rows+":container-switch:owners");}}
`
	for _, guard := range []string{"word", "reference"} {
		t.Run(guard, func(t *testing.T) {
			text := source
			if guard == "reference" {
				text = strings.ReplaceAll(text, "public static int pop()", "public static Integer pop()")
				text = strings.ReplaceAll(text, "return depth==0?-1:stack[--depth];", "return depth==0?null:Integer.valueOf(stack[--depth]);")
				text = strings.ReplaceAll(text, "public static int fold(int mode,boolean skip){int score=0;", "public static int fold(int initial,boolean skip){Integer mode=Integer.valueOf(initial);int score=0;")
				text = strings.ReplaceAll(text, "while(mode>=0);", "while(mode!=null);")
			}
			for _, name := range []string{"StreamFold", "TokenReducer"} {
				t.Run(name, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "StreamFold", name), name, name+"Driver", "33612:container-switch:owners\n")
				})
			}
		})
	}
}
