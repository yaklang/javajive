package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedFailureAllocationRetainsBothGuards(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
class First extends Exception{} class Second extends Exception{}
class Failure extends RuntimeException{static int created;int tag;Failure(String text,Throwable cause){super(text,cause);created++;}}
public class GuardedFactory{
 public static Integer cache;public static int loads,makes;public static final Object TOKEN=new Object();public static final First ONE=new First();public static final Second TWO=new Second();
 public static Integer load(){loads++;return Integer.valueOf(1);}
 public static Object make(int mode)throws First,Second{makes++;if(mode==2)throw ONE;if(mode==3)throw TWO;return TOKEN;}
 public static Object build(int mode){Failure failure;
  if(mode==0||__SECOND__){failure=new Failure("blocked",null);}
  else {try{return make(mode);}catch(First cause){failure=new Failure("first",cause);}catch(Second cause){failure=new Failure("second",cause);}}
  failure.tag=mode;throw failure;
 }
}
class GuardedFactoryDriver{public static void main(String[]args){int rows=0;
 for(int mode=-1;mode<=5;mode++)for(Integer initial:new Integer[]{null,0,1,2,3,99}){
  GuardedFactory.cache=initial;GuardedFactory.loads=GuardedFactory.makes=Failure.created=0;
  boolean lazy=__LAZY__,load=lazy&&mode!=0&&initial==null;Integer expectedCache=load?Integer.valueOf(1):initial;
  boolean blocked=mode==0||mode==(lazy?(expectedCache==null?1:expectedCache.intValue()):1);
  Throwable cause=blocked?null:mode==2?GuardedFactory.ONE:mode==3?GuardedFactory.TWO:null;
  boolean fails=blocked||cause!=null;Failure error=null;Object result=null;try{result=GuardedFactory.build(mode);}catch(Failure x){error=x;}
  if((error!=null)!=fails)throw new AssertionError("completion "+mode+":"+initial);
  if(error!=null){String text=blocked?"blocked":mode==2?"first":"second";if(error.tag!=mode||!error.getMessage().equals(text)||error.getCause()!=cause)throw new AssertionError("failure identity");}
  else if(result!=GuardedFactory.TOKEN)throw new AssertionError("result identity");
  if(Failure.created!=(fails?1:0)||GuardedFactory.makes!=(blocked?0:1)||GuardedFactory.loads!=(load?1:0)||!java.util.Objects.equals(GuardedFactory.cache,expectedCache))throw new AssertionError("effects");rows++;
 }System.out.println(rows+":shared-failure:guards");}}
`
	for _, lazy := range []bool{false, true} {
		key, condition, flag := "direct", "mode==1", "false"
		if lazy {
			key, condition, flag = "lazy", "mode==(cache==null?cache=load():cache).intValue()", "true"
		}
		text := strings.ReplaceAll(strings.ReplaceAll(source, "__SECOND__", condition), "__LAZY__", flag)
		t.Run(key, func(t *testing.T) {
			for _, name := range []string{"GuardedFactory", "PairedFilter"} {
				t.Run(name, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "GuardedFactory", name), name, name+"Driver", "42:shared-failure:guards\n")
				})
			}
		})
	}
}
