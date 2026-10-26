package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedReferenceFrontierKeepsBothStores(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class ReferenceFrontier {
 public static void event(StringBuilder trace,int n,int fail,RuntimeException marker){trace.append(n).append(';');if(n==fail)throw marker;}
 public static char read(String s,int i,StringBuilder trace,int fail,RuntimeException marker){event(trace,i,fail,marker);return s.charAt(i);}
 public static String execute(String s,String naming,StringBuilder trace,int fail,RuntimeException marker){
  String first,second;char c0,c1;
  if(s.length()>1 && (c0=read(s,0,trace,fail,marker))>='A' && c0<='Z' && (c1=read(s,1,trace,fail,marker))>='A' && c1<='Z' && (naming==null || naming.isEmpty())) {
   char[] a=s.toCharArray();a[0]=(char)(a[0]+32);first=new String(a);a[1]=(char)(a[1]+32);second=new String(a);
  } else {first=null;second=null;}
  java.util.function.Supplier<String> use=()->String.valueOf(first)+":"+String.valueOf(second);
  event(trace,2,fail,marker);String captured=use.get();
  return captured+":"+String.valueOf(first)+":"+String.valueOf(second);
 }
}
class ReferenceFrontierDriver {
 public static void main(String[]args){int rows=0;
  for(String s:new String[]{"","A","AB","Ab","aB","@B","[B","A@","A[","XYtail","ZZ"})for(String naming:new String[]{null,"","named"})for(int fail=-1;fail<3;fail++){
   boolean read0=s.length()>1,read1=read0&&s.charAt(0)>='A'&&s.charAt(0)<='Z';
   boolean selected=read1&&s.charAt(1)>='A'&&s.charAt(1)<='Z'&&(naming==null||naming.length()==0);
   String a=selected?Character.toLowerCase(s.charAt(0))+s.substring(1):"null";
   String b=selected?a.substring(0,1)+Character.toLowerCase(s.charAt(1))+s.substring(2):"null";
   String wanted=a+":"+b+":"+a+":"+b;StringBuilder expected=new StringBuilder();boolean thrown=false;
   for(int e:new int[]{read0?0:-1,read1?1:-1,2})if(e>=0){expected.append(e).append(';');if(e==fail){thrown=true;break;}}
   StringBuilder trace=new StringBuilder();RuntimeException marker=new RuntimeException("identity");
   try{String got=ReferenceFrontier.execute(s,naming,trace,fail,marker);if(thrown||!wanted.equals(got))throw new AssertionError("value "+got+" expected "+wanted);}
   catch(RuntimeException caught){if(!thrown||caught!=marker)throw new AssertionError("exception identity",caught);}
   if(!expected.toString().equals(trace.toString()))throw new AssertionError("trace "+trace+" expected "+expected);rows++;
  }System.out.println(rows+":shared-reference-frontier:two-stores:captured-identity");
 }
}
`
	for _, owner := range []string{"ReferenceFrontier", "SelectedNames"} {
		t.Run(owner, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "ReferenceFrontier", owner), owner, owner+"Driver", "132:shared-reference-frontier:two-stores:captured-identity\n")
		})
	}
}
