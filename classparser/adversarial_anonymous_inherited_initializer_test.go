package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

const nativeAnonymousInheritedFieldInitializerFixture = `class InheritedInitEffects {
 static String trace="";static final RuntimeException error=new RuntimeException("original");
 static Object read(Object value){trace+="R";if(value instanceof String&&value.equals("boom"))throw error;return value;}
 static int finish(){trace+="D";return 7;}
}
class InheritedInitParent<T> {protected T value;InheritedInitParent(T input){value=input;}Object get(){return value;}int stamp(){return 0;}void change(){value=null;}}
class InheritedInitOwner {InheritedInitParent<Object> make(Object input){return new InheritedInitParent<Object>(input){
 final Object snapshot=InheritedInitEffects.read(value);final int completed=InheritedInitEffects.finish();Object get(){return snapshot;}int stamp(){return completed;}
};}}
class InheritedInitDriver {public static void main(String[]args){int rows=0;Object token=new Object();for(Object input:new Object[]{null,token,"text","boom"}){
 InheritedInitEffects.trace="";try{InheritedInitParent<Object> child=new InheritedInitOwner().make(input);child.change();if(input=="boom"||!child.getClass().isAnonymousClass()||child.get()!=input||child.stamp()!=7||!InheritedInitEffects.trace.equals("RD"))throw new AssertionError("inherited initializer declaration/identity/order");}
 catch(RuntimeException e){if(input!="boom"||e!=InheritedInitEffects.error||!InheritedInitEffects.trace.equals("R"))throw new AssertionError("inherited initializer abrupt identity/order",e);}rows++;
 }System.out.println(rows+":inherited:initializer:field");}}
`

func TestAdversarialAnonymousInheritedInitializerKeepsOriginalFieldBinding(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			for _, shape := range []string{"erased-object", "generic-string", "generic-number", "generic-array", "nearest-shadow"} {
				t.Run(shape, func(t *testing.T) {
					source, owner := nativeAnonymousInheritedFieldInitializerFixture, "InheritedInitOwner"
					switch shape {
					case "generic-string":
						source = strings.ReplaceAll(source, "InheritedInitParent<Object>", "InheritedInitParent<String>")
						source = strings.Replace(source, "make(Object input)", "make(String input)", 1)
						source = strings.Replace(source, `Object token=new Object();for(Object input:new Object[]{null,token,"text","boom"})`, `for(String input:new String[]{null,new String("text"),"text","boom"})`, 1)
						source = strings.Replace(source, "InheritedInitEffects.read(value)", "InheritedInitEffects.read((Object)value)", 1)
						source = strings.Replace(source, "static int finish()", `static Object read(String input){trace+="S";return "wrong overload";}static int finish()`, 1)
					case "generic-number", "generic-array":
						typ, inputs, fail := "Number", `null,new Integer(7),Integer.valueOf(Integer.MIN_VALUE),Integer.valueOf(-1)`, `input!=null&&input.intValue()==-1`
						readFail := `value instanceof Number&&((Number)value).intValue()==-1`
						if shape == "generic-array" {
							typ = "String[]"
							inputs = `null,new String[]{"text"},new String[0],new String[]{"boom"}`
							fail = `input!=null&&input.length==1&&"boom".equals(input[0])`
							readFail = `value instanceof String[]&&((String[])value).length==1&&"boom".equals(((String[])value)[0])`
						}
						source = strings.ReplaceAll(source, "InheritedInitParent<Object>", "InheritedInitParent<"+typ+">")
						source = strings.Replace(source, "make(Object input)", "make("+typ+" input)", 1)
						source = strings.Replace(source, "InheritedInitEffects.read(value)", "InheritedInitEffects.read((Object)value)", 1)
						source = strings.Replace(source, `value instanceof String&&value.equals("boom")`, readFail, 1)
						source = strings.Replace(source, "static int finish()", `static Object read(`+typ+` input){trace+="S";return "wrong overload";}static int finish()`, 1)
						source = source[:strings.Index(source, "class InheritedInitDriver")] + `class InheritedInitDriver {public static void main(String[]args){int rows=0;for(` + typ + ` input:new ` + typ + `[]{` + inputs + `}){boolean failed=` + fail + `;InheritedInitEffects.trace="";try{InheritedInitParent<` + typ + `> child=new InheritedInitOwner().make(input);child.change();if(failed||!child.getClass().isAnonymousClass()||child.get()!=input||child.stamp()!=7||!InheritedInitEffects.trace.equals("RD"))throw new AssertionError("inherited initializer declaration/identity/order");}catch(RuntimeException e){if(!failed||e!=InheritedInitEffects.error||!InheritedInitEffects.trace.equals("R"))throw new AssertionError("inherited initializer abrupt identity/order",e);}rows++;}System.out.println(rows+":inherited:initializer:field");}}`
					case "nearest-shadow":
						source = strings.Replace(source, "class InheritedInitOwner", `class InheritedInitMiddle extends InheritedInitParent<Object>{protected Object value;InheritedInitMiddle(Object input){super(new Object());value=input;}}
class InheritedInitOwner`, 1)
						source = strings.Replace(source, "new InheritedInitParent<Object>(input)", "new InheritedInitMiddle(input)", 1)
					}
					if rename == "renamed" {
						source = strings.ReplaceAll(source, "InheritedInitOwner", "ReadEnvelope")
						source = regexp.MustCompile(`\bvalue\b`).ReplaceAllString(source, "payload")
						owner = "ReadEnvelope"
					}
					testNativePrivateSetterFixture(t, source, owner, "InheritedInitDriver", "4:inherited:initializer:field\n")
				})
			}
		})
	}
}

func TestAdversarialAnonymousProtectedInheritedInitializerAcrossPackages(t *testing.T) {
	for _, shape := range []string{"erased-object", "generic-string"} {
		t.Run(shape, func(t *testing.T) {
			source := nativeAnonymousInheritedFieldInitializerFixture
			parent := `class InheritedInitParent<T> {protected T value;InheritedInitParent(T input){value=input;}Object get(){return value;}int stamp(){return 0;}void change(){value=null;}}`
			source = strings.Replace(source, parent, "", 1)
			source = strings.ReplaceAll(source, "Object get()", "public Object get()")
			source = strings.ReplaceAll(source, "int stamp()", "public int stamp()")
			if shape == "generic-string" {
				source = strings.ReplaceAll(source, "InheritedInitParent<Object>", "InheritedInitParent<String>")
				source = strings.Replace(source, "make(Object input)", "make(String input)", 1)
				source = strings.Replace(source, `Object token=new Object();for(Object input:new Object[]{null,token,"text","boom"})`, `for(String input:new String[]{null,new String("text"),"text","boom"})`, 1)
				source = strings.Replace(source, "InheritedInitEffects.read(value)", "InheritedInitEffects.read((Object)value)", 1)
				source = strings.Replace(source, "static int finish()", `static Object read(String input){trace+="S";return "wrong overload";}static int finish()`, 1)
			}
			sources := map[string]string{
				"basepkg/InheritedInitParent.java": `package basepkg;public class InheritedInitParent<T> {protected T value;public InheritedInitParent(T input){value=input;}public Object get(){return value;}public int stamp(){return 0;}public void change(){value=null;}}`,
				"childpkg/InheritedInitOwner.java": "package childpkg;import basepkg.InheritedInitParent;\n" + source,
			}
			testNativePrivateSetterSourceFixture(t, sources, "childpkg/InheritedInitOwner", "childpkg.InheritedInitDriver", "4:inherited:initializer:field\n")
		})
	}
}
