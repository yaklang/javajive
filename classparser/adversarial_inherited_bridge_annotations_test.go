package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialInheritedBridgePreservesDeclarationAndParameterAnnotations(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original Java 8 compiler required")
	}
	fixture := `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @java.lang.annotation.Target({java.lang.annotation.ElementType.METHOD,java.lang.annotation.ElementType.PARAMETER}) @interface BridgeVisible {String label();int[] steps();}
@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.CLASS) @java.lang.annotation.Target({java.lang.annotation.ElementType.METHOD,java.lang.annotation.ElementType.PARAMETER}) @interface BridgeHidden {int value();}
` + inheritedCovariantBridgeFixture
	fixture = strings.Replace(fixture, "public String read(long n,double real,Object token)", `@BridgeVisible(label="method\u0000\ud801\udc00",steps={-1,0,2147483647}) @BridgeHidden(17) public String read(@BridgeVisible(label="wide",steps={1,2}) long n,@BridgeHidden(23) double real,Object token)`, 1)
	fixture = strings.Replace(fixture, `if(bridge==null)throw new AssertionError("lost inherited bridge");`, `if(bridge==null)throw new AssertionError("lost inherited bridge");
 BridgeVisible visible=bridge.getAnnotation(BridgeVisible.class);if(visible==null||!visible.label().equals("method\u0000\ud801\udc00")||!java.util.Arrays.equals(visible.steps(),new int[]{-1,0,Integer.MAX_VALUE}))throw new AssertionError("method annotation graph");
 int[] changed=visible.steps();changed[0]=99;if(visible.steps()[0]!=-1)throw new AssertionError("annotation array identity");
 java.lang.annotation.Annotation[][] parameters=bridge.getParameterAnnotations();if(parameters.length!=3||parameters[0].length!=1||parameters[1].length!=0||parameters[2].length!=0)throw new AssertionError("parameter annotation slots");
 BridgeVisible parameter=(BridgeVisible)parameters[0][0];if(!parameter.label().equals("wide")||!java.util.Arrays.equals(parameter.steps(),new int[]{1,2}))throw new AssertionError("wide parameter annotation");`, 1)
	for _, root := range []string{"BridgeOuter", "OtherAnnotatedOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				testIndependentInheritedBridgeWithOriginal(t, modern, java, native, strings.ReplaceAll(fixture, "BridgeOuter", root), root, debug, true, "36:inherited:covariant:bridge:dispatch:wide:exception\n", nil, func(t *testing.T, name string, original, fresh []byte, artifact *JarReconstructionArtifact) {
					finished, err := artifact.FinishClass(name+".class", fresh)
					if err != nil {
						t.Fatal("annotated bridge completion", err)
					}
					for _, raw := range [][]byte{original, finished} {
						object := mustParseLowering(t, raw)
						methodHidden, parameterHidden := false, false
						for _, method := range object.Methods {
							if method.AccessFlags != 0x1041 {
								continue
							}
							for _, attr := range method.Attributes {
								switch a := attr.(type) {
								case *RuntimeVisibleAnnotationsAttribute:
									if a.IsInvisible && len(a.Annotations) == 1 && a.Annotations[0].TypeName == "LBridgeHidden;" {
										pairs := a.Annotations[0].ElementValuePairs
										methodHidden = len(pairs) == 1 && pairs[0].Name == "value" && pairs[0].Tag == 'I' && pairs[0].Value.(*ConstantIntegerInfo).Value == 17
									}
								case *RuntimeVisibleParameterAnnotationsAttribute:
									if a.IsInvisible && a.NumParameters == 3 && len(a.ParameterAnnotations) == 3 && len(a.ParameterAnnotations[0]) == 0 && len(a.ParameterAnnotations[1]) == 1 && len(a.ParameterAnnotations[2]) == 0 && a.ParameterAnnotations[1][0].TypeName == "LBridgeHidden;" {
										pairs := a.ParameterAnnotations[1][0].ElementValuePairs
										parameterHidden = len(pairs) == 1 && pairs[0].Name == "value" && pairs[0].Tag == 'I' && pairs[0].Value.(*ConstantIntegerInfo).Value == 23
									}
								}
							}
						}
						if !methodHidden || !parameterHidden {
							t.Fatal("CLASS retention or parameter position changed")
						}
					}
				})
			})
		}
	}
}
