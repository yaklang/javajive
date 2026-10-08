package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Ten SDK receiver/producer paths crossed with ten conditional argument trees
// exercise 100 different pre-initialization regions. The unchanged original
// driver derives each observation from the mask, API contract and raw long,
// rather than delegating assertions to a candidate-rebuilt helper.
func TestAdversarialMemberConstructorFrameHundredStructureBank(t *testing.T) {
	paths := []string{
		"f.getType()", "d.getPropertyType()", "f.getDeclaringClass()",
		"d.getReadMethod().getDeclaringClass()", "f.getGenericType().getClass()",
		"d.getReadMethod().getReturnType()", "d.getWriteMethod().getParameterTypes()[0]",
		"box.field.getType()", "new java.lang.reflect.Field[]{f}[0].getType()",
		"new java.beans.PropertyDescriptor[]{d}[0].getReadMethod().getReturnType()",
	}
	var source strings.Builder
	source.WriteString(`
class FrameBankBean {public long field;public int getValue(){return 0;}public void setValue(int n){}}
class FrameBankBox {volatile java.lang.reflect.Field field;FrameBankBox(java.lang.reflect.Field f){field=f;}}
class FrameBankParent {static int calls;final Class type;final boolean flag;final long word;final Object observed;
 FrameBankParent(Class t,boolean b,long n){calls++;type=t;flag=b;word=n;observed=capture();}Object capture(){return null;}}
public class FrameBankOwner {final Object token;FrameBankOwner(Object t){token=t;}
`)
	for path, producer := range paths {
		for depth := 1; depth <= 10; depth++ {
			value := producer
			for bit := depth - 1; bit >= 0; bit-- {
				value = fmt.Sprintf("((mask&%d)!=0?%s:String.class)", 1<<bit, value)
			}
			fmt.Fprintf(&source, "class Child%d_%d extends FrameBankParent {Child%d_%d(java.lang.reflect.Field f,java.beans.PropertyDescriptor d,FrameBankBox box,int mask,long n){super(%s,(mask&1024)!=0,new java.util.concurrent.atomic.AtomicLong(n).get());}Object capture(){return FrameBankOwner.this.token;}}\n", path, depth, path, depth, value)
			fmt.Fprintf(&source, "Object make%d_%d(java.lang.reflect.Field f,java.beans.PropertyDescriptor d,FrameBankBox box,int mask,long n){return new Child%d_%d(f,d,box,mask,n);}\n", path, depth, path, depth)
		}
	}
	source.WriteString(`}
class FrameBankDriver {
 public static void main(String[]args)throws Exception{Object token=new Object();FrameBankOwner owner=new FrameBankOwner(token);
  java.lang.reflect.Field f=FrameBankBean.class.getField("field");java.beans.PropertyDescriptor d=new java.beans.PropertyDescriptor("value",FrameBankBean.class.getMethod("getValue"),FrameBankBean.class.getMethod("setValue",int.class));FrameBankBox box=new FrameBankBox(f);
  Class[] leaves={long.class,int.class,FrameBankBean.class,FrameBankBean.class,Class.class,int.class,int.class,long.class,long.class,int.class};
  long[] words={Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE,0x12345678abcdef01L};int rows=0;
  for(int path=0;path<10;path++)for(int depth=1;depth<=10;depth++){
   java.lang.reflect.Method m=FrameBankOwner.class.getDeclaredMethod("make"+path+"_"+depth,java.lang.reflect.Field.class,java.beans.PropertyDescriptor.class,FrameBankBox.class,int.class,long.class);
   for(int row=0;row<12;row++){int low=row==0?1023:row==1?0:1023^(1<<(row-2));int mask=low|((row%2)*1024);long word=words[row%words.length];FrameBankParent.calls=0;
    // Every early-false arm must remain lazy, even with all SDK receivers null.
    FrameBankParent h=(FrameBankParent)m.invoke(owner,row==1?null:f,row==1?null:d,row==1?null:box,mask,word);
    Class expected=(low&((1<<depth)-1))==((1<<depth)-1)?leaves[path]:String.class;
    if(h.type!=expected||h.word!=word||h.flag!=(row%2!=0)||h.observed!=token||FrameBankParent.calls!=1)throw new AssertionError("API/branch/word/capture/once:"+path+":"+depth+":"+row);rows++;
   }
  }
  System.out.println(rows+":member-frames:100-structures:SDK:lazy:wide:capture");
 }
}`)
	testNativePrivateSetterSourceFixture(t, map[string]string{"FrameBankOwner.java": source.String()}, "FrameBankOwner", "FrameBankDriver", "1200:member-frames:100-structures:SDK:lazy:wide:capture\n")
}
