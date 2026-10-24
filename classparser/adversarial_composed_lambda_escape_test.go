package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Native compiler lambda names and a temporary checked-escape helper create
// independent obligations in one fresh class. Both must discharge; rejecting
// the second obligation merely for observing the first prevents composition.
// A named lexical member selects the native owning-family renderer. Its root
// must publish the same narrowly scoped temporary-helper receipt as a flat
// declaration. No original family class may appear on the rebuild classpath.
// Exceptions attributes describe Java source declarations, not verifier edges.
// An authored original can therefore propagate the same checked instance with
// an empty declaration. Source recovery needs a temporary generic rethrow, but
// a completed artifact must preserve the original method universe as well as
// the original exception identity, finally order and normal result.
func TestAdversarialLambdaAndCheckedEscapeObligationsCompose(t *testing.T) {
	javac, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"EscapeOwner", "RenamedEscapeOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, target := range []int{8} {
				t.Run(fmt.Sprintf("%s/%s/target%d", owner, debug, target), func(t *testing.T) {
					fixture := `class EscapeFailure extends Exception {EscapeFailure(){super("same");}}
class EscapeEffects {static String trace="";static final EscapeFailure checked=new EscapeFailure();static final RuntimeException unchecked=new IllegalStateException("same");static int calculate(int mode)throws EscapeFailure {trace+="C";if(mode==0)throw checked;if(mode==1)throw unchecked;return mode*31+7;}}
public class ` + owner + ` {public static class Child {public static int delta(){return 17;}}static final java.util.function.IntUnaryOperator SHIFT=(x)->x*3+1;static java.util.function.IntUnaryOperator make(int base){return (x)->x+base;}public static int exercise(int mode)throws EscapeFailure {try{return SHIFT.applyAsInt(EscapeEffects.calculate(mode))+make(mode).applyAsInt(5);}finally{EscapeEffects.trace+="F";}}}
class EscapeDriver {public static void main(String[]args){if(` + owner + `.Child.delta()!=17||` + owner + `.Child.class.getDeclaringClass()!=` + owner + `.class||` + owner + `.class.getDeclaredMethods().length!=4)throw new AssertionError("original method universe");try{if(` + owner + `.class.getDeclaredMethod("exercise",int.class).getExceptionTypes().length!=0)throw new AssertionError("original empty exceptions");}catch(NoSuchMethodException e){throw new AssertionError(e);}int rows=0;for(int mode:new int[]{0,1,2,-1,Integer.MIN_VALUE,Integer.MAX_VALUE}){EscapeEffects.trace="";try{int value=` + owner + `.exercise(mode);int expected=java.math.BigInteger.valueOf(mode).multiply(java.math.BigInteger.valueOf(94)).add(java.math.BigInteger.valueOf(27)).intValue();if(mode<2&&mode>=0||value!=expected||!EscapeEffects.trace.equals("CF"))throw new AssertionError("normal value/order");}catch(Throwable e){if(e!=(mode==0?EscapeEffects.checked:mode==1?EscapeEffects.unchecked:null)||!EscapeEffects.trace.equals("CF"))throw new AssertionError("exception identity/order",e);}rows++;}System.out.println(rows+":escape:identity:finally:method-universe");}}`
					if owner == "RenamedEscapeOwner" {
						// Exercise an implicit receiver as well as a static lambda;
						// the non-BMP identifier also distinguishes UTF-8 from MUTF-8.
						fixture = strings.ReplaceAll(fixture, "static java.util.function.IntUnaryOperator make", "int bias; java.util.function.IntUnaryOperator make")
						fixture = strings.ReplaceAll(fixture, "x+base;", "x+base+bias;")
						fixture = strings.ReplaceAll(fixture, "+make(mode)", "+new "+owner+"().make(mode)")
						fixture = strings.ReplaceAll(fixture, "make(", "构造𐐀(")
					}
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
					object := mustParseLowering(t, files[owner+".class"])
					// Alpha-rename only private lambda implementations and their
					// own symbolic method references. This is a valid independently
					// instrumented original, with unchanged Code and bootstrap
					// handle targets. Application string constants are untouched.
					renamed := 0
					for _, method := range object.Methods {
						oldName, _ := sourceBridgeUTF8(object, method.NameIndex)
						if !strings.HasPrefix(oldName, "lambda$") {
							continue
						}
						cut := strings.LastIndexByte(oldName, '$')
						ordinal, err := strconv.Atoi(oldName[cut+1:])
						if err != nil {
							t.Fatal(err)
						}
						oldIndex := method.NameIndex
						nameIndex := uint16(object.ConstantPoolManager.AddUtf8Info(oldName[:cut+1] + strconv.Itoa(ordinal+11)))
						for _, constant := range object.ConstantPool {
							ref := nativeConstantMember(constant)
							if ref == nil {
								continue
							}
							target, known := sourceBridgeClassName(object, ref.ClassIndex)
							if !known || target != owner {
								continue
							}
							nt, ok := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
							if !ok || nt.NameIndex != oldIndex || nt.DescriptorIndex != method.DescriptorIndex {
								continue
							}
							object.ConstantPool = append(object.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: nameIndex, DescriptorIndex: nt.DescriptorIndex})
							ref.NameAndTypeIndex = uint16(len(object.ConstantPool))
						}
						method.NameIndex = nameIndex
						renamed++
					}
					if renamed != 2 {
						t.Fatal("two independent lambda implementations", renamed)
					}
					removed := 0
					for _, m := range object.Methods {
						if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "exercise" {
							attrs := []AttributeInfo{}
							for _, a := range m.Attributes {
								if _, ok := a.(*ExceptionsAttribute); ok {
									removed++
								} else {
									attrs = append(attrs, a)
								}
							}
							m.Attributes = attrs
						}
					}
					if removed != 1 {
						t.Fatal("authored original Exceptions entry")
					}
					files[owner+".class"] = object.Bytes()
					run := func(dir string) {
						t.Helper()
						for _, assertions := range []string{"-ea", "-da"} {
							log, err := exec.Command(java, "-Xverify:all", assertions, "-cp", dir, "EscapeDriver").CombinedOutput()
							if err != nil || string(log) != "6:escape:identity:finally:method-universe\n" {
								t.Fatal("original-first independent escape oracle", err, string(log))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
						t.Fatal(err)
					}
					artifact, err := NewJarReconstructionArtifactFromLocal(jar, target, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer artifact.Close()
					source, err := artifact.source.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("complete source", err, string(source))
					}
					support := map[string][]byte{}
					for n, b := range files {
						if n != owner+".class" && !strings.HasPrefix(n, owner+"$") {
							support[n] = b
						}
					}
					out := sourceArtifactWriteFiles(t, support)
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", fmt.Sprint(target), "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("fresh compilation without original owner", err, string(log), string(source))
					}
					unreturned, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					if result, err := artifact.FinishClass(owner+".class", unreturned); err == nil || result != nil {
						t.Fatal("unreturned native owning source granted executable completion")
					}
					published, err := artifact.ReadFile(owner + ".class")
					if err != nil || !bytes.Equal(source, published) {
						t.Fatal("same complete native source must activate", err)
					}
					if debug == "none" {
						checkComposedEscapeAuthorityBoundaries(t, owner, files[owner+".class"], unreturned, artifact)
					}
					for _, binary := range []string{owner, owner + "$Child"} {
						fresh, err := os.ReadFile(filepath.Join(out, binary+".class"))
						if err != nil {
							t.Fatal(err)
						}
						before := bytes.Clone(fresh)
						result, err := artifact.FinishClass(binary+".class", fresh)
						if err != nil || result == nil {
							t.Fatal("temporary exception helper must be discharged", err)
						}
						if !bytes.Equal(fresh, before) {
							t.Fatal("fresh input modified")
						}
						if err := os.WriteFile(filepath.Join(out, binary+".class"), result, 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(out)
				})
			}
		}
	}
}

// Mutate the independently compiled inputs and the actual activated receipts,
// never a fabricated positive plan. The executable pass must preserve the
// caller's bytes and refuse composition when any identity edge loses proof.
func checkComposedEscapeAuthorityBoundaries(t *testing.T, owner string, original, compiled []byte, artifact *JarReconstructionArtifact) {
	t.Helper()
	base := artifact.source.jar
	origin := base.sourceConstructorLowering.escapeActive[owner]
	plan := base.sourceLambdaMetadata.plan(owner)
	if plan == nil || len(plan.methods) != 2 || origin.helper == "" || plan.sourceHash != origin.sourceHash {
		t.Fatal("two actual obligations from one completed source required")
	}
	for _, variant := range []string{"control", "missing authority", "pending plan", "wrong owner", "stale original", "different source", "empty source", "duplicate original", "duplicate generated", "changed descriptor", "ordinary target", "missing implementation", "ordinary collision", "public implementation", "instance protocol", "wrong version", "helper alias"} {
		t.Run("authority/"+variant, func(t *testing.T) {
			p := *plan
			p.methods = append([]sourceLambdaMetadataMethod(nil), plan.methods...)
			lambda := &sourceLambdaMetadataAuthority{active: map[string]*sourceLambdaMetadataPlan{owner: &p}}
			authority := &sourceConstructorLoweringAuthority{jar: &JarFS{sourceLambdaMetadata: lambda}, escapeActive: map[string]checkedEscapeSourceOrigin{owner: origin}}
			object := mustParseLowering(t, bytes.Clone(compiled))
			var implementation, ordinary *MemberInfo
			for _, m := range object.Methods {
				n, _ := sourceBridgeUTF8(object, m.NameIndex)
				d, _ := sourceBridgeUTF8(object, m.DescriptorIndex)
				if n == p.methods[0].generated.name && d == p.methods[0].generated.descriptor {
					implementation = m
				}
				if n == "exercise" {
					ordinary = m
				}
			}
			if implementation == nil || ordinary == nil {
				t.Fatal("fixture declarations required")
			}
			switch variant {
			case "missing authority":
				authority.jar.sourceLambdaMetadata = nil
			case "pending plan":
				lambda.plans, lambda.active = lambda.active, nil
			case "wrong owner":
				p.owner = "IndependentOwner"
			case "stale original":
				p.originalHash[0] ^= 1
			case "different source":
				p.sourceHash = sha256.Sum256([]byte("independent returned source"))
			case "empty source":
				p.sourceHash = [32]byte{}
			case "duplicate original":
				p.methods[1].original = p.methods[0].original
			case "duplicate generated":
				p.methods[1].generated = p.methods[0].generated
			case "changed descriptor":
				p.methods[0].generated.descriptor = "()V"
			case "ordinary target":
				p.methods[0].original = sourceLambdaMethodIdentity{"exercise", "(I)I"}
			case "missing implementation":
				implementation.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("independentImplementation"))
			case "ordinary collision":
				ordinary.NameIndex, ordinary.DescriptorIndex = implementation.NameIndex, implementation.DescriptorIndex
			case "public implementation":
				implementation.AccessFlags = 0x1009
			case "instance protocol":
				implementation.AccessFlags ^= 0x0008
			case "wrong version":
				object.MajorVersion = 53
			case "helper alias":
				p.methods[0].generated.name = origin.helper
			}
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative must reach executable proof", err)
			}
			before := bytes.Clone(input)
			result, err := authority.lowerCheckedEscapeHelper(owner, original, input, nil)
			if !bytes.Equal(input, before) {
				t.Fatal("caller input mutated")
			}
			if variant == "control" {
				if err != nil || bytes.Equal(result, input) || len(mustParseLowering(t, result).Methods)+1 != len(object.Methods) {
					t.Fatal("composed proof did not discharge exactly one helper", err)
				}
			} else if err == nil && !bytes.Equal(result, input) || err != nil && result != nil {
				t.Fatal("unproved or partial executable output", err)
			}
		})
	}
}
