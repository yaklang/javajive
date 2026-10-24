package javaclassparser

import (
	"bytes"
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
					checkInheritedBridgeAnnotationLoweringBoundaries(t, name, original, fresh, artifact)
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

func checkInheritedBridgeAnnotationLoweringBoundaries(t *testing.T, name string, original, fresh []byte, artifact *JarReconstructionArtifact) {
	t.Helper()
	for _, variant := range []string{"lost declaration", "changed value", "changed array order", "changed retention", "duplicate table", "lost parameter", "moved parameter", "short parameter table", "helper annotation"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(fresh))
			var method *MemberInfo
			for _, candidate := range object.Methods {
				n, _ := sourceBridgeUTF8(object, candidate.NameIndex)
				if n == "read" && (variant == "helper annotation" && candidate.AccessFlags == 1 || variant != "helper annotation" && candidate.AccessFlags == 0x1041) {
					method = candidate
				}
			}
			if method == nil {
				t.Fatal("missing genuine compiled forwarding declaration")
			}
			changed := false
			for i, attr := range method.Attributes {
				switch a := attr.(type) {
				case *RuntimeVisibleAnnotationsAttribute:
					if a.IsInvisible || changed {
						continue
					}
					switch variant {
					case "lost declaration", "helper annotation":
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
					case "changed value":
						for _, pair := range a.Annotations[0].ElementValuePairs {
							if pair.Tag == 's' {
								pair.Value = &ConstantUtf8Info{Value: "changed"}
							}
						}
					case "changed array order":
						for _, pair := range a.Annotations[0].ElementValuePairs {
							if pair.Tag == '[' {
								items := pair.Value.([]*ElementValuePairAttribute)
								items[0], items[1] = items[1], items[0]
							}
						}
					case "changed retention":
						a.IsInvisible = true
					case "duplicate table":
						method.Attributes = append(method.Attributes, a)
					default:
						continue
					}
					changed = true
				case *RuntimeVisibleParameterAnnotationsAttribute:
					if a.IsInvisible || changed {
						continue
					}
					switch variant {
					case "lost parameter":
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
					case "moved parameter":
						a.ParameterAnnotations[0], a.ParameterAnnotations[1] = a.ParameterAnnotations[1], a.ParameterAnnotations[0]
						// This attribute preserves its original serialized table.
						// Move the complete first row ahead of the two empty rows
						// in that payload as well, so the parser sees the mutation.
						if len(a.Info) < 7 || a.Info[0] != 3 || !bytes.Equal(a.Info[len(a.Info)-4:], []byte{0, 0, 0, 0}) {
							t.Fatal("authored visible table must annotate only parameter zero")
						}
						info := append([]byte{3, 0, 0}, a.Info[1:len(a.Info)-4]...)
						a.Info = append(info, 0, 0)
					case "short parameter table":
						a.NumParameters--
						a.ParameterAnnotations = a.ParameterAnnotations[:2]
						a.Info = append([]byte{2}, a.Info[1:len(a.Info)-2]...)
						a.AttrLen = uint32(len(a.Info))
					default:
						continue
					}
					changed = true
				}
				if changed {
					break
				}
			}
			if !changed {
				t.Fatal("mutation did not reach its intended metadata table")
			}
			input := object.Bytes()
			if bytes.Equal(input, fresh) {
				t.Fatal("metadata mutation was not serialized")
			}
			got, err := artifact.source.jar.sourceConstructorLowering.lowerInheritedBridges(name, original, input, nil)
			if err == nil && !bytes.Equal(got, input) || err != nil && got != nil {
				t.Fatal("metadata mismatch exposed a partial executable repair", err)
			}
		})
	}
}
