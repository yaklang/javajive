package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedFallbackReturnRetainsItsSelectedEdge(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class FastDecision {
 public static int commits,early,fallbacks;
 public static final RuntimeException FAILURE=new RuntimeException("fallback");
 public static long quick(int n){early++;return n*17L;}
 public static long slow(boolean fail){fallbacks++;if(fail)throw FAILURE;return -701L;}
 public static long choose(int mode,int a,int b,boolean fail){
  fast:{if(mode==0)break fast;if(mode==1)return quick(a);
   long result;
   if(a<0)result=a*31L;
   else if(b<0)result=b*43L;
   else {result=(a+1L)*(b+3L);if(mode==2&&a==b)break fast;}
   commits++;return result;
  }
  return slow(fail);
 }
}
class FastDecisionDriver{public static void main(String[]args){int count=0;
 int[] edges={Integer.MIN_VALUE,-Integer.MAX_VALUE,-3,-1,0,1,3,127,32768,Integer.MAX_VALUE-1,Integer.MAX_VALUE};
 for(int mode=0;mode<5;mode++)for(int a:edges)for(int b:edges)for(boolean fail:new boolean[]{false,true}){
  boolean fallback=mode==0||(mode==2&&a>=0&&b>=0&&a==b);
  long want=mode==1?a*17L:fallback?-701L:a<0?a*31L:b<0?b*43L:(a+1L)*(b+3L);
  FastDecision.commits=FastDecision.early=FastDecision.fallbacks=0;RuntimeException caught=null;long actual=0;
  try{actual=FastDecision.choose(mode,a,b,fail);}catch(RuntimeException failure){caught=failure;}
  if(fallback&&fail){if(caught!=FastDecision.FAILURE)throw new AssertionError("failure identity");}
  else if(caught!=null||actual!=want)throw new AssertionError("selected value "+mode+":"+a+":"+b+":"+actual+":"+want);
  if(FastDecision.commits!=(mode!=1&&!fallback?1:0)||FastDecision.early!=(mode==1?1:0)||FastDecision.fallbacks!=(fallback?1:0))throw new AssertionError("selected effects");count++;
 }System.out.println(count+":selected:fallback");}}
`
	for _, width := range []string{"long", "int"} {
		for _, name := range []string{"FastDecision", "CandidateRoute"} {
			t.Run(width+"/"+name, func(t *testing.T) {
				fixture := strings.ReplaceAll(source, "FastDecision", name)
				if width == "int" {
					fixture = strings.NewReplacer("public static long choose", "public static int choose", "return quick(a)", "return (int)quick(a)", "return result;", "return (int)result;", "return slow(fail)", "return (int)slow(fail)", "long want=", "long want=(int)(", ";\n  "+name+".commits", ");\n  "+name+".commits").Replace(fixture)
				}
				testAuthoredOwnerAcrossSourceModes(t, fixture, name, name+"Driver", "1210:selected:fallback\n")
			})
		}
	}
}
