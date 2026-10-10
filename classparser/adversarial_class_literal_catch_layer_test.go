package javaclassparser

import "testing"

func TestAdversarialClassLiteralResolutionKeepsNestedHandlerPriority(t *testing.T) {
	roundTripGenericFlowUnits(t, "LiteralResolutionDriver", `
class DeniedLiteralType {}
class LiteralResolutionLoader extends ClassLoader {
 LiteralResolutionLoader(){super(LiteralResolutionDriver.class.getClassLoader());}
 protected Class<?> loadClass(String name,boolean resolve)throws ClassNotFoundException{
  if(name.equals("DeniedLiteralType"))throw new ClassNotFoundException(name);
  if(!name.equals("LiteralResolutionConsumer"))return super.loadClass(name,resolve);
  Class<?> found=findLoadedClass(name);if(found==null){try(java.io.InputStream in=getParent().getResourceAsStream(name+".class")){
   java.io.ByteArrayOutputStream bytes=new java.io.ByteArrayOutputStream();byte[] buffer=new byte[512];int n;while((n=in.read(buffer))!=-1)bytes.write(buffer,0,n);byte[] data=bytes.toByteArray();found=defineClass(name,data,0,data.length);
  }catch(java.io.IOException failure){throw new ClassNotFoundException(name,failure);}}
  if(resolve)resolveClass(found);return found;
 }
}
class LiteralResolutionConsumer {
 public static String run(int mode){String trace="body;";try{
  try{if(mode==0)return DeniedLiteralType.class.getName();return "normal:"+trace;}
  catch(LinkageError first){trace+="inner;";return DeniedLiteralType.class.getName();}
 }catch(LinkageError second){return "outer:"+trace;}}
}
public class LiteralResolutionDriver {
 public static void main(String[]args)throws Exception{for(int mode=0;mode<2;mode++){
  Class<?> consumer=Class.forName("LiteralResolutionConsumer",true,new LiteralResolutionLoader());java.lang.reflect.Method method=consumer.getDeclaredMethod("run",int.class);method.setAccessible(true);String actual=(String)method.invoke(null,mode);
  String expected=mode==0?"outer:body;inner;":"normal:body;";if(!expected.equals(actual))throw new AssertionError("linkage priority "+actual);System.out.println(actual);
 }}
}`, nil, []string{"LiteralResolutionConsumer"}, Precision, Compatibility, "legacy")
}
