package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorClassLiteralKeepsOriginalResolutionAndInitialization(t *testing.T) {
	for _, shape := range []string{"class and array mirrors", "missing symbolic target"} {
		t.Run(shape, func(t *testing.T) {
			fixture := constructorUnknownAbruptCaptureFixture + `class AbruptLiteralState{static int initialized;}class AbruptLiteralPayload{static{AbruptLiteralState.initialized++;}}class AbruptLiteralMissing{}`
			fixture = strings.Replace(fixture, "trace=trace*31+1;", `trace=trace*31+1;Class<?> mirror=n==70000?AbruptLiteralMissing.class:n==7?AbruptLiteralPayload[][].class:AbruptLiteralPayload.class;if(AbruptLiteralState.initialized!=0||mirror==null||mirror.isArray()!=(n==7))throw new AssertionError("class mirror/resolution must not initialize target");`, 1)
			fixture = strings.Replace(fixture, "}catch(NullPointerException e){", `}catch(NoClassDefFoundError e){if(n!=70000||AbruptParent.trace!=1||AbruptLiteralState.initialized!=0||e.getCause()==null)throw new AssertionError("original linkage failure/effect order",e);System.out.println(n+":linkage:"+(value==x));}catch(NullPointerException e){`, 1)
			fixture = strings.Replace(fixture, "Object x=new Object();", `String[] cp=System.getProperty("java.class.path").split(java.io.File.pathSeparator);java.io.File targetRoot=new java.io.File(cp[0]).getCanonicalFile(),originalRoot=new java.io.File(cp[cp.length-1]).getCanonicalFile();if(!new java.io.File(AbruptChild.class.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(targetRoot))throw new AssertionError("target fallback");for(Class<?> type:new Class<?>[]{AbruptParent.class,AbruptDriver.class,AbruptLiteralState.class})if(!new java.io.File(type.getProtectionDomain().getCodeSource().getLocation().toURI()).getCanonicalFile().equals(originalRoot))throw new AssertionError("original producer/driver origin");Object x=new Object();`, 1)
			for _, prefix := range []string{"Abrupt", "Elm"} {
				t.Run(prefix, func(t *testing.T) {
					var mutate func(*testing.T, map[string][]byte)
					want := "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n"
					if shape == "missing symbolic target" {
						want = strings.ReplaceAll(want, "70000:ok:", "70000:linkage:")
						mutate = func(t *testing.T, files map[string][]byte) {
							o, e := Parse(append([]byte(nil), files[prefix+"Parent.class"]...))
							if e != nil {
								t.Fatal(e)
							}
							changed := 0
							for _, cp := range o.ConstantPool {
								if c, ok := cp.(*ConstantClassInfo); ok {
									n, _ := sourceBridgeUTF8(o, c.NameIndex)
									if n == prefix+"LiteralMissing" {
										c.NameIndex = sourceBridgePoolString(t, o, prefix+"AbsentSymbolicTarget")
										changed++
									}
								}
							}
							if changed != 1 {
								t.Fatalf("original symbolic class roots=%d", changed)
							}
							files[prefix+"Parent.class"] = o.Bytes()
						}
					}
					testConstructorReceiverTypeViewMutatedFixture(t, strings.ReplaceAll(fixture, "Abrupt", prefix), prefix, mutate, want)
				})
			}
		})
	}
}
