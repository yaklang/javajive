package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLazyCacheRetainsAllocationContinuation(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class LayeredMemo {
 public java.util.HashMap[] table;
 public int built,indexed;
 public int fail;
 public RuntimeException marker;
 public int index(int bucket){indexed++;if((fail&1)!=0)throw marker;return bucket;}
 public String build(String key){built++;if((fail&2)!=0)throw marker;return key+":"+built;}
 public String lookup(String key,int bucket,boolean retain){
  java.util.HashMap selected;
  lookup:{java.util.HashMap[] current=table;
   if(current==null){if(retain){current=new java.util.HashMap[4];table=current;}else{selected=null;break lookup;}}
   int at=index(bucket);selected=current[at];String known;
   if(selected==null){if(retain){selected=new java.util.HashMap(4);current[at]=selected;known=null;}else break lookup;}
   else known=(String)selected.get(key);
   if(known!=null)return known;
  }
  String value=build(key);if(retain)selected.put(key,value);return value;
 }
}
class LayeredMemoDriver {public static void main(String[]args){int rows=0;
 for(int initial=0;initial<3;initial++)for(int pattern=0;pattern<8;pattern++){
  LayeredMemo memo=new LayeredMemo();java.util.HashMap[] expected=initial==0?null:new java.util.HashMap[4];
  if(initial>0)memo.table=new java.util.HashMap[4];
  if(initial==2){expected[2]=new java.util.HashMap();memo.table[2]=new java.util.HashMap();String seed=new String("seed");expected[2].put("k1",seed);memo.table[2].put("k1",seed);}
  int builds=0,indexes=0;
  for(int call=0;call<32;call++){int bucket=(call*3+pattern)%4;String key="k"+(call%3);boolean retain=(pattern&(1<<(call%3)))!=0;
   java.util.HashMap selected=null;String want=null;
   if(expected!=null||retain){if(expected==null)expected=new java.util.HashMap[4];indexes++;selected=expected[bucket];if(selected==null&&retain){selected=new java.util.HashMap();expected[bucket]=selected;}if(selected!=null)want=(String)selected.get(key);}
   boolean cached=want!=null;if(want==null){want=key+":"+(++builds);if(retain)selected.put(key,want);}
   String actual=memo.lookup(key,bucket,retain);
   if(!actual.equals(want)||memo.built!=builds||memo.indexed!=indexes)throw new AssertionError("cache result/effects");
   if(cached&&actual!=memo.table[bucket].get(key))throw new AssertionError("cached identity");
   if((memo.table==null)!=(expected==null))throw new AssertionError("allocation");
   if(expected!=null)for(int i=0;i<4;i++){if((memo.table[i]==null)!=(expected[i]==null)||expected[i]!=null&&!memo.table[i].equals(expected[i]))throw new AssertionError("cache contents");}
   rows++;
  }
 }
 for(int initial=0;initial<3;initial++)for(boolean retain:new boolean[]{false,true})for(int fail=0;fail<4;fail++)for(boolean hit:new boolean[]{false,true}){
  LayeredMemo memo=new LayeredMemo();memo.fail=fail;memo.marker=new RuntimeException("same failure");String seed=new String("seed");String key=hit?"k1":"k2";
  if(initial>0)memo.table=new java.util.HashMap[4];if(initial==2){memo.table[2]=new java.util.HashMap();memo.table[2].put("k1",seed);}
  int indexes=initial>0||retain?1:0;boolean indexFailed=indexes!=0&&(fail&1)!=0;boolean cached=initial==2&&hit;
  int builds=indexFailed||cached?0:1;boolean thrown=indexFailed||builds!=0&&(fail&2)!=0;
  try{String actual=memo.lookup(key,2,retain);if(thrown)throw new AssertionError("missing throw");if(cached?actual!=seed:!actual.equals(key+":1"))throw new AssertionError("result identity/value");}
  catch(RuntimeException actual){if(!thrown||actual!=memo.marker)throw new AssertionError("exception identity",actual);}
  if(memo.indexed!=indexes||memo.built!=builds)throw new AssertionError("throw effects");
  if((memo.table!=null)!=(initial>0||retain))throw new AssertionError("outer allocation order");
  if(memo.table!=null){boolean bucket=initial==2||retain&&!indexFailed;if((memo.table[2]!=null)!=bucket)throw new AssertionError("inner allocation order");
   if(bucket){int size=(initial==2?1:0)+(!thrown&&builds!=0&&retain?1:0);if(memo.table[2].size()!=size)throw new AssertionError("store after failure");}
  }rows++;
 }
 System.out.println(rows+":cache:allocation:continuation:identity");}}
`
	for _, name := range []string{"LayeredMemo", "BucketCatalogue"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "LayeredMemo", name), name, name+"Driver", "816:cache:allocation:continuation:identity\n")
		})
	}
}
