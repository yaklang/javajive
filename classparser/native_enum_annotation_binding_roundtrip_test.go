package javaclassparser

import (
	"strings"
	"testing"
)

// An enum constant in an AnnotationDefault and one in an actual annotation
// must bind to the same original declaration as the method descriptor. The
// original JVM independently checks identities, defaults, values-array copies
// and runtime annotation invocation; compiling source alone is insufficient.
const nativeEnumAnnotationBindingFixture = `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME)
@interface BindingMark {
 enum Choice{FIRST,SECOND}
 Choice value() default Choice.FIRST;
 Choice[] choices() default {Choice.SECOND,Choice.FIRST};
}
@BindingMark(value=BindingMark.Choice.SECOND,choices={BindingMark.Choice.FIRST,BindingMark.Choice.SECOND})
class BindingTarget{}
class BindingDriver{public static void main(String[]args)throws Exception{
 BindingMark actual=BindingTarget.class.getAnnotation(BindingMark.class);
 if(actual==null||actual.value()!=BindingMark.Choice.SECOND||actual.choices().length!=2||actual.choices()[0]!=BindingMark.Choice.FIRST||actual.choices()[1]!=BindingMark.Choice.SECOND)throw new AssertionError("actual enum annotation binding");
 Object value=BindingMark.class.getMethod("value").getDefaultValue();
 BindingMark.Choice[] choices=(BindingMark.Choice[])BindingMark.class.getMethod("choices").getDefaultValue();
 if(value!=BindingMark.Choice.FIRST||choices.length!=2||choices[0]!=BindingMark.Choice.SECOND||choices[1]!=value||BindingMark.class.getMethod("value").getReturnType()!=BindingMark.Choice.class||BindingMark.Choice.class.getDeclaringClass()!=BindingMark.class)throw new AssertionError("default enum annotation declaration");
 BindingMark.Choice[] copy=actual.choices();copy[0]=null;if(actual.choices()[0]!=BindingMark.Choice.FIRST)throw new AssertionError("annotation array clone");
 System.out.println("enum:annotation:default:actual:binding:clone");}}`

func TestNativeEnumAnnotationDefaultAndActualBindingRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeEnumAnnotationBindingFixture, []string{"BindingMark", "BindingTarget"}, "BindingDriver", "enum:annotation:default:actual:binding:clone\n", nativeLexicalExactSignatures)
}
func TestNativeEnumAnnotationBindingRenamedRoundTrip(t *testing.T) {
	fixture := nativeEnumAnnotationBindingFixture
	for _, pair := range [][2]string{{"BindingMark", "Configuration"}, {"Choice", "Selection"}, {"FIRST", "LEFT"}, {"SECOND", "RIGHT"}, {"BindingTarget", "Settings"}} {
		fixture = strings.ReplaceAll(fixture, pair[0], pair[1])
	}
	testNativeIndependentFamilyFixture(t, fixture, []string{"Configuration", "Settings"}, "BindingDriver", "enum:annotation:default:actual:binding:clone\n", nativeLexicalExactSignatures)
}

// Equal simple enum names in independent families must not share a binding.
// Defaults and explicit values intentionally choose opposite constants.
func TestNativeEnumAnnotationIndependentSameNamedTypesRoundTrip(t *testing.T) {
	fixture := `class OtherPolicy{enum Choice{FIRST,SECOND}}` + nativeEnumAnnotationBindingFixture
	fixture = strings.Replace(fixture, "Choice value() default Choice.FIRST;", "Choice value() default Choice.FIRST;OtherPolicy.Choice other() default OtherPolicy.Choice.SECOND;", 1)
	fixture = strings.Replace(fixture, "if(actual==null||", "if(actual==null||actual.other()!=OtherPolicy.Choice.SECOND||", 1)
	fixture = strings.Replace(fixture, "Object value=", "if(BindingMark.class.getMethod(\"other\").getDefaultValue()!=OtherPolicy.Choice.SECOND||OtherPolicy.Choice.class.getDeclaringClass()!=OtherPolicy.class)throw new AssertionError(\"independent enum declaration\");Object value=", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"BindingMark", "BindingTarget", "OtherPolicy"}, "BindingDriver", "enum:annotation:default:actual:binding:clone\n", nativeLexicalExactSignatures)
}

func TestNativeEnumAnnotationNestedAndArrayValuesRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeEnumAnnotationBindingFixture, "Choice value() default Choice.FIRST;", `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @interface Inner{Choice value();} Inner[] nested() default {@Inner(Choice.SECOND),@Inner(Choice.FIRST)};Choice value() default Choice.FIRST;`, 1)
	fixture = strings.Replace(fixture, "if(actual==null||", "if(actual==null||actual.nested().length!=2||actual.nested()[0].value()!=BindingMark.Choice.SECOND||actual.nested()[1].value()!=BindingMark.Choice.FIRST||", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"BindingMark", "BindingTarget"}, "BindingDriver", "enum:annotation:default:actual:binding:clone\n", nativeLexicalExactSignatures)
}
