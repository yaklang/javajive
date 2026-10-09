package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func originalCaptureEncoding(t *testing.T, files map[string][]byte, name string, keepFlag bool) {
	t.Helper()
	obj, err := Parse(files[name+".class"])
	if err != nil {
		t.Fatal(err)
	}
	obj.ConstantPoolManager.AddUtf8Info("Synthetic")
	count := 0
	for _, field := range obj.Fields {
		if field.AccessFlags&0x1000 == 0 {
			continue
		}
		if field.AccessFlags&0x10 == 0 {
			t.Fatal("authored capture is not final")
		}
		field.Attributes = append(field.Attributes, &SyntheticAttribute{})
		if !keepFlag {
			field.AccessFlags &^= 0x1000
		}
		count++
	}
	if count == 0 {
		t.Fatal("fixture has no original synthetic captures")
	}
	files[name+".class"] = obj.Bytes()
}

func captureEncodingFixture(prefix string) string {
	f := strings.ReplaceAll(closedCalleeFixture, "CALL", "count=n;wide=w;this$0=input;ClosedEffects.mark();if(fail)throw ClosedEffects.failure")
	f = strings.ReplaceAll(f, "METHOD", "")
	f = strings.ReplaceAll(f, "RESULT_CHECK", "p.result!=0||p.reference!=null")
	return strings.ReplaceAll(f, "Closed", prefix)
}

// Parent/effects/driver bytes stay original. The oracle compares capture
// identity, wide/primitive values, hidden parent storage and abrupt effects.
func TestAdversarialConstructorCaptureSyntheticAttributeRoundTrip(t *testing.T) {
	for _, prefix := range []string{"Closed", "Archive"} {
		for _, encoding := range []string{"attribute", "both"} {
			t.Run(prefix+"/"+encoding, func(t *testing.T) {
				mutate := func(t *testing.T, files map[string][]byte) {
					originalCaptureEncoding(t, files, prefix+"Owner$Child", encoding == "both")
				}
				testIndependentFlatMutatedClosedCalleeFamily(t, captureEncodingFixture(prefix), prefix, "320:closed:method:capture:effects\n", []string{prefix + "Owner", prefix + "Owner$Child"}, mutate)
			})
		}
	}
}

func TestNativeConstructorCaptureSyntheticAttributeRoundTrip(t *testing.T) {
	for _, prefix := range []string{"Closed", "Archive"} {
		for _, encoding := range []string{"attribute", "both"} {
			t.Run(prefix+"/"+encoding, func(t *testing.T) {
				mutate := func(t *testing.T, files map[string][]byte) {
					originalCaptureEncoding(t, files, prefix+"Owner$Child", encoding == "both")
				}
				fixture := captureEncodingFixture(prefix)
				fixture = strings.Replace(fixture, `System.out.println(rows+`, `java.lang.reflect.Field captureField=`+prefix+`Owner.Child.class.getDeclaredField("this$0");if(!captureField.isSynthetic()||!java.lang.reflect.Modifier.isFinal(captureField.getModifiers())||captureField.getType()!=`+prefix+`Owner.class)throw new AssertionError("original synthetic capture reflection");System.out.println(rows+`, 1)
				testNativeIndependentMutatedFamilyFixture(t, fixture, []string{prefix + "Owner"}, prefix+"Driver", "320:closed:method:capture:effects\n", mutate, nativeLexicalExactSignatures)
			})
		}
	}
}

func TestAdversarialConstructorSyntheticAttributeStillRejectsEarlyObservation(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"ClosedObserve", "ArchiveObserve"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, encoding := range []string{"attribute", "both"} {
				t.Run(prefix+"/"+debug+"/"+encoding, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, strings.ReplaceAll(closedMethodObservationFixture, "ClosedObserve", prefix), debug)
					originalCaptureEncoding(t, files, prefix+"Owner$Child", encoding == "both")
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != "2:open:dispatch:observation\n" {
						t.Fatal("original capture observation", got)
					}
					resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
								t.Run(string(mode), func(t *testing.T) {
									var source string
									var err error
									raw := bytes.Clone(files[prefix+"Owner$Child.class"])
									if mode == "legacy" {
										source, err = DecompileWithResolver(raw, resolve)
									} else {
										var result DecompileResult
										result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
										source = result.Source
									}
									if err != nil || !strings.Contains(source, DecompileStubMarker) {
										t.Fatalf("synthetic evidence cannot certify unsafe capture motion: %v\n%s", err, source)
									}
								})
							}
						})
					}
				})
			}
		}
	}
}

func TestNativeCaptureABIProjectionPreservesSyntheticAndStorage(t *testing.T) {
	files := nativeCompileClasses(t, captureEncodingFixture("Closed"))
	raw := files["ClosedOwner$Child.class"]
	want := nativeBinaryShape(t, raw)
	for _, kind := range []string{"attribute", "both", "lost synthetic", "lost final", "wrong descriptor"} {
		t.Run(kind, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			obj.ConstantPoolManager.AddUtf8Info("Synthetic")
			found := false
			for _, field := range obj.Fields {
				if field.AccessFlags&0x1000 == 0 {
					continue
				}
				found = true
				switch kind {
				case "attribute":
					field.AccessFlags &^= 0x1000
					field.Attributes = append(field.Attributes, &SyntheticAttribute{})
				case "both":
					field.Attributes = append(field.Attributes, &SyntheticAttribute{})
				case "lost synthetic":
					field.AccessFlags &^= 0x1000
				case "lost final":
					field.AccessFlags &^= 0x10
				case "wrong descriptor":
					field.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("Ljava/lang/Object;"))
				}
			}
			if !found {
				t.Fatal("missing authored capture")
			}
			got := nativeBinaryShape(t, obj.Bytes())
			if equal := got == want; equal != (kind == "attribute" || kind == "both") {
				t.Fatalf("ABI equivalent=%v\n%s\n%s", equal, want, got)
			}
		})
	}
}
