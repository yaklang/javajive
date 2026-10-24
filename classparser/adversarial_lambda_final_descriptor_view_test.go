package javaclassparser

import "testing"

// The factory immediately enters a privileged call inside a protected region.
// A null-only local is solved as Object after capture construction. The final
// body still has to use the original implementation descriptor for overloads.
func TestAdversarialLateNullLambdaCaptureKeepsDescriptorOverloads(t *testing.T) {
	const source = `import java.security.*;import java.util.*;
class CaptureDescriptorChoice {
 static String choose(String label,Locale locale,ClassLoader loader){return "loader:"+label+":"+locale.getLanguage()+":"+(loader==ClassLoader.getSystemClassLoader());}
 static String choose(String label,Locale locale,Object object){return "WRONG:Object";}
}
public class LateNullCaptureDescriptor {
 static ResourceBundle bundle(Locale locale,String label){ResourceBundle result;try{ClassLoader loader=null;PrivilegedAction<ResourceBundle> action=()->ResourceBundle.getBundle(label,locale,loader!=null?loader:ClassLoader.getSystemClassLoader());result=AccessController.doPrivileged(action);}catch(MissingResourceException missing){return null;}return result;}
 static String choose(Locale locale,String label){String result;try{ClassLoader loader=null;PrivilegedAction<String> action=()->CaptureDescriptorChoice.choose(label,locale,loader!=null?loader:ClassLoader.getSystemClassLoader());result=AccessController.doPrivileged(action);}catch(MissingResourceException missing){return "missing";}return result;}
 public static void main(String[]args){int rows=0;for(Locale locale:new Locale[]{Locale.ENGLISH,Locale.FRENCH,null})for(String label:new String[]{"absent.bundle.alpha","absent.bundle.beta",null}){try{System.out.println("bundle:"+bundle(locale,label));if(locale==null||label==null)throw new AssertionError("original null boundary");}catch(NullPointerException expected){if(locale!=null&&label!=null)throw expected;System.out.println("bundle:null-input");}try{String value=choose(locale,label);String expected="loader:"+label+":"+locale.getLanguage()+":true";if(!value.equals(expected))throw new AssertionError(value);System.out.println(value);}catch(NullPointerException expected){if(locale!=null)throw expected;System.out.println("choice:null-locale");}rows++;}if(rows!=9)throw new AssertionError(rows);}
}`
	roundTripGenericFlow(t, "LateNullCaptureDescriptor", source, Precision, Compatibility, "legacy")
}
