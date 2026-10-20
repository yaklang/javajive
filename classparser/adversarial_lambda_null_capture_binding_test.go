package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialNullSingletonCaptureKeepsPhysicalDescriptor(t *testing.T) {
	const source = `import java.security.*;import java.util.*;
class CaptureDescriptorChoice {
 static String choose(String label,Locale locale,ClassLoader loader){return "loader:"+label+":"+locale.getLanguage()+":"+(loader==ClassLoader.getSystemClassLoader());}
 static String choose(String label,Locale locale,Object object){return "WRONG:Object";}
}
public class UncapturedLambdaNames {
 static ResourceBundle bundle(Locale locale,String label){ResourceBundle result;try{ClassLoader loader=null;PrivilegedAction<ResourceBundle> action=()->ResourceBundle.getBundle(label,locale,loader!=null?loader:ClassLoader.getSystemClassLoader());result=AccessController.doPrivileged(action);}catch(MissingResourceException missing){return null;}return result;}
 static String choose(Locale locale,String label){String result;try{ClassLoader loader=null;PrivilegedAction<String> action=()->CaptureDescriptorChoice.choose(label,locale,loader!=null?loader:ClassLoader.getSystemClassLoader());result=AccessController.doPrivileged(action);}catch(MissingResourceException missing){return "missing";}return result;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{int rows=0;StringBuilder trace=new StringBuilder();for(Locale locale:new Locale[]{Locale.ENGLISH,Locale.FRENCH,null})for(String label:new String[]{"absent.bundle.alpha","absent.bundle.beta",null}){try{trace.append("bundle:").append(UncapturedLambdaNames.bundle(locale,label)).append(';');if(locale==null||label==null)throw new AssertionError("original null boundary");}catch(NullPointerException expected){if(locale!=null&&label!=null)throw expected;trace.append("bundle:null-input;");}try{String value=UncapturedLambdaNames.choose(locale,label);String expected="loader:"+label+":"+locale.getLanguage()+":true";if(!value.equals(expected))throw new AssertionError(value);trace.append(value).append(';');}catch(NullPointerException expected){if(locale!=null)throw expected;trace.append("choice:null-locale;");}rows++;}if(rows!=9)throw new AssertionError(rows);int implementations=0;for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){if(method.isSynthetic()){implementations++;if(!Arrays.equals(method.getParameterTypes(),new Class<?>[]{String.class,Locale.class,ClassLoader.class}))throw new AssertionError("physical null capture descriptor "+method.toGenericString());}}if(implementations!=2)throw new AssertionError(implementations);System.out.println("9:uncaptured:lambda:parameters:"+trace);}
}
`
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, source, 2, "9", named) })
	}
}

// Reference identity, including a non-system loader, rejects treating an
// unknown or nullable word as the null singleton. The direct original body
// and real privileged invocation are observed independently.
func TestAdversarialNullableCaptureKeepsAllReferenceValues(t *testing.T) {
	const source = `import java.security.*;import java.lang.reflect.*;import java.util.*;
public class UncapturedLambdaNames {
 public static ClassLoader select(ClassLoader input,boolean empty){try{final ClassLoader capture=empty?null:input;PrivilegedAction<ClassLoader> action=()->capture;return AccessController.doPrivileged(action);}catch(SecurityException denied){throw denied;}}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{ClassLoader.class}))throw new AssertionError("physical capture");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);ClassLoader system=ClassLoader.getSystemClassLoader(),distinct=new ClassLoader(system){};int rows=0;StringBuilder trace=new StringBuilder();for(ClassLoader input:new ClassLoader[]{null,system,distinct})for(boolean empty:new boolean[]{false,true})for(int path=0;path<2;path++){ClassLoader expected=empty?null:input;ClassLoader actual=path==0?UncapturedLambdaNames.select(input,empty):(ClassLoader)implementation.invoke(null,expected);if(actual!=expected)throw new AssertionError("reference identity "+rows);trace.append(actual==null?0:actual==system?1:2);rows++;}if(rows!=12)throw new AssertionError(rows);System.out.println("12:uncaptured:lambda:parameters:"+trace);}
}`
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "12", named) })
	}
}

func TestAdversarialNullReferenceCaptureKeepsDescriptorKinds(t *testing.T) {
	const source = `import java.lang.reflect.*;import java.util.*;import java.util.function.*;
public class UncapturedLambdaNames {
 public static Supplier<CAPTURE> factory(){CAPTURE capture=null;return ()->capture;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{Method implementation=null;for(Method method:UncapturedLambdaNames.class.getDeclaredMethods())if(method.isSynthetic()){if(implementation!=null||!Arrays.equals(method.getParameterTypes(),new Class<?>[]{CAPTURE.class})||method.getReturnType()!=CAPTURE.class)throw new AssertionError("physical null capture");implementation=method;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);Supplier<CAPTURE> value=UncapturedLambdaNames.factory();if(value.get()!=null||implementation.invoke(null,new Object[]{null})!=null)throw new AssertionError("null became text/default");System.out.println("2:uncaptured:lambda:parameters:null");}
}`
	for _, kind := range []string{"ClassLoader", "String", "String[]"} {
		t.Run(kind, func(t *testing.T) {
			fixture := strings.ReplaceAll(source, "CAPTURE", kind)
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, fixture, 1, "2", named) })
			}
		})
	}
}
