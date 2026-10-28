package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialPostIncrementArgumentKeepsJoinedLocal(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class OrdinalWriter {
 public int next,stored,lookups,writes;public boolean present;public OrdinalWriter sink;
 public static int created;public static boolean fail;public static final RuntimeException FAILURE=new RuntimeException("put");
 public OrdinalWriter(){created++;}
 public int find(int key,int candidate){lookups++;return present?stored:candidate;}
 public void put(int key,int value){writes++;stored=value;if(fail)throw FAILURE;present=true;}
 public int select(int key){int value;boolean found=false;
  if(sink!=null){if((value=sink.find(key,next))!=next)found=true;else next++;}
  else{(sink=new OrdinalWriter()).put(key,value=next++);}
  return found?-value:value;
 }
}
class OrdinalWriterDriver {public static void main(String[]args){int rows=0;
 int[]seeds={Integer.MIN_VALUE,Integer.MIN_VALUE+1,-32769,-17,-1,0,1,47,32768,Integer.MAX_VALUE};
 for(int seed:seeds)for(int previous:seeds)for(int shape=0;shape<3;shape++)for(boolean fail:new boolean[]{false,true}){
  OrdinalWriter x=new OrdinalWriter();x.next=seed;if(shape!=0){x.sink=new OrdinalWriter();x.sink.present=shape==2;x.sink.stored=previous;}
  OrdinalWriter old=x.sink;OrdinalWriter.created=0;OrdinalWriter.fail=fail;
  int expected=shape==2?previous:seed;boolean found=shape==2&&previous!=seed;int result=0;Throwable error=null;
  try{result=x.select(23);}catch(Throwable e){error=e;}
  boolean thrown=shape==0&&fail;
  if(error!=(thrown?OrdinalWriter.FAILURE:null)||!thrown&&result!=(found?-expected:expected)||x.next!=(found?seed:seed+1)||OrdinalWriter.created!=(shape==0?1:0)||x.sink==null||shape!=0&&x.sink!=old||x.sink.lookups!=(shape==0?0:1)||x.sink.writes!=(shape==0?1:0)||x.sink.stored!=(shape==0?seed:previous)||x.sink.present!=(shape==0?!fail:shape==2))throw new AssertionError(seed+":"+previous+":"+shape+":"+fail);rows++;
 }System.out.println(rows+":postincrement:joined-binding");}}
`
	for _, shape := range []string{"direct", "shared exit"} {
		text, expected := source, "600:postincrement:joined-binding\n"
		if shape == "shared exit" {
			text = strings.Split(source, "class OrdinalWriterDriver")[0]
			text = strings.ReplaceAll(text, "public int next,", "public OrdinalWriter external;public boolean enabled=true;public int next,")
			text = strings.ReplaceAll(text, "int value;boolean found=false;", "int value;outer:{if(external!=null){if((value=external.find(key,-1))!=-1)break outer;}if(!enabled)return 777;boolean found=false;")
			text = strings.ReplaceAll(text, "return found?-value:value;", "if(!found)return value;value=-value;}return -value;")
			text += `
class OrdinalWriterDriver {public static void main(String[]args){int rows=0;
 int[]seeds={Integer.MIN_VALUE,Integer.MIN_VALUE+1,-32769,-17,-1,0,1,47,32768,Integer.MAX_VALUE};
 for(int seed:seeds)for(int previous:seeds)for(int shape=0;shape<3;shape++)for(boolean fail:new boolean[]{false,true})for(int guard=0;guard<3;guard++)for(boolean enabled:new boolean[]{false,true}){
  OrdinalWriter x=new OrdinalWriter();x.next=seed;x.enabled=enabled;if(shape!=0){x.sink=new OrdinalWriter();x.sink.present=shape==2;x.sink.stored=previous;}if(guard!=0){x.external=new OrdinalWriter();x.external.present=guard==2;x.external.stored=previous;}
  OrdinalWriter old=x.sink;OrdinalWriter.created=0;OrdinalWriter.fail=fail;
  int value=shape==2?previous:seed;boolean found=shape==2&&previous!=seed,hit=guard==2&&previous!=-1,skipped=hit||!enabled;
  int expected=hit?-previous:!enabled?777:value;int result=0;Throwable error=null;try{result=x.select(23);}catch(Throwable e){error=e;}
  boolean thrown=!skipped&&shape==0&&fail;
  if(error!=(thrown?OrdinalWriter.FAILURE:null)||!thrown&&result!=expected||x.next!=(skipped||found?seed:seed+1)||OrdinalWriter.created!=(!skipped&&shape==0?1:0)||guard!=0&&x.external.lookups!=1||skipped&&x.sink!=old)throw new AssertionError("guard "+seed+":"+previous+":"+shape+":"+guard+":"+enabled);
  if(!skipped&&(x.sink==null||shape!=0&&x.sink!=old||x.sink.lookups!=(shape==0?0:1)||x.sink.writes!=(shape==0?1:0)||x.sink.stored!=(shape==0?seed:previous)||x.sink.present!=(shape==0?!fail:shape==2)))throw new AssertionError("effects");rows++;
 }System.out.println(rows+":postincrement:joined-binding");}}
`
			expected = "3600:postincrement:joined-binding\n"
		}
		t.Run(shape, func(t *testing.T) {
			for _, update := range []string{"instance increment", "instance decrement", "static increment", "static decrement"} {
				t.Run(update, func(t *testing.T) {
					variant := text
					if strings.HasPrefix(update, "static") {
						variant = strings.ReplaceAll(variant, "public int next,stored,lookups,writes;", "public static int next;public int stored,lookups,writes;")
					}
					if strings.HasSuffix(update, "decrement") {
						variant = strings.ReplaceAll(variant, "next++", "next--")
						variant = strings.ReplaceAll(variant, "seed+1", "seed-1")
					}
					for _, owner := range []string{"OrdinalWriter", "SerialRegistry"} {
						t.Run(owner, func(t *testing.T) {
							testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(variant, "OrdinalWriter", owner), owner, owner+"Driver", expected)
						})
					}
				})
			}
		})
	}
}
