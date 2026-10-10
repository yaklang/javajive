package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

// The named constructor already proves the physical slot-1 enclosing SUPER
// path. A mixed anonymous forest must compose that same original certificate,
// without pretending that its parameter is THIS or accepting arbitrary reads.
func TestAdversarialMemberSuperEnclosingPathComposesWithAnonymousForest(t *testing.T) {
	for _, root := range []string{"SuperForestOwner", "RenamedSuperForestOwner"} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deep=%v", root, deep), func(t *testing.T) {
				source := `public class SuperForestOwner {
 private final long bias;private SuperForestOwner(long bias){this.bias=bias;}
 public interface Value{long get();}
 public class Base{public final long base;Base(long n){base=n;}}
 public class Layer {public class View extends Base{private final long seed;private View(long seed){super(seed);this.seed=seed;}public Value value(final long delta){return new Value(){public long get(){return SuperForestOwner.this.bias+View.this.seed+base+delta;}};}}public View make(long seed){return new View(seed);}}
 public Layer.View make(long seed){return new Layer().make(seed);}
 public static SuperForestOwner create(long bias){return new SuperForestOwner(bias);}
}

class SuperForestDriver{public static void main(String[]args){int count=0;for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){SuperForestOwner owner=SuperForestOwner.create(bias);SuperForestOwner.Layer.View view=owner.make(seed);SuperForestOwner.Value value=view.value(delta);if(view.getClass().getDeclaringClass()!=SuperForestOwner.Layer.class||value.getClass().getEnclosingClass()!=SuperForestOwner.Layer.View.class||!value.getClass().getEnclosingMethod().getName().equals("value"))throw new AssertionError("named/anonymous/super ownership");long want=java.math.BigInteger.valueOf(bias).add(java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(2))).add(java.math.BigInteger.valueOf(delta)).longValue();if(value.get()!=want||view.base!=seed)throw new AssertionError("enclosing super/capture/binding/overflow");count++;}System.out.println(count+":named:super:anonymous:overflow");}}
`
				if deep {
					source = strings.Replace(source, " public class Layer {", " public class Scope { public class Layer {", 1)
					source = strings.Replace(source, " public Layer.View make", " Layer layer(){return new Layer();}} public Scope.Layer.View make", 1)
					source = strings.ReplaceAll(source, "return new Layer().make(seed)", "return new Scope().layer().make(seed)")
					source = strings.ReplaceAll(source, "SuperForestOwner.Layer", "SuperForestOwner.Scope.Layer")
				}
				source = strings.ReplaceAll(source, "SuperForestOwner", root)
				testNativePrivateSetterCompiledFixture(t, root, "SuperForestDriver", "125:named:super:anonymous:overflow\n", func(t *testing.T, debug string) map[string][]byte {
					files := nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
					z := nativeArchive(t, files)
					defer z.Close()
					obj, err := Parse(files[root+".class"])
					if err != nil {
						t.Fatal(err)
					}
					p := z.nativeMemberReader(obj).planNativeMemberFamily()
					if p == nil {
						t.Fatal("original named plan missing")
					}
					if p.children[root+"$Layer$View"] == nil && p.children[root+"$Scope$Layer$View"] == nil {
						t.Fatal("original owned constructor missing")
					}
					return files
				})
			})
		}
	}
}

func TestAdversarialMemberSuperForestCertificateMustMatchOriginalInstructions(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, nativeMemberAncestorSuperFixture, debug)
		for _, variant := range []string{"original", "cached pc", "cached base", "cached owner", "cached descriptor", "cached parameter", "cached cycle", "missing cached path", "unprojected", "wrong constructor descriptor", "wrong capture pc", "wrong capture receiver", "wrong capture parameter", "wrong delegate pc", "wrong delegate owner", "wrong delegate descriptor", "slot overwrite", "wrong load slot", "changed field owner", "changed field name", "changed field descriptor", "alternate entry", "work", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				original := map[string][]byte{}
				for name, raw := range files {
					original[name] = bytes.Clone(raw)
				}
				z := nativeArchive(t, original)
				defer z.Close()
				root, err := Parse(original["AncestorMemberOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				reader := z.nativeMemberReader(root)
				p := reader.planNativeMemberFamily()
				if p == nil {
					t.Fatal("complete original named plan")
				}
				child := p.children["AncestorMemberOwner$Layer$Child"]
				var code *CodeAttribute
				var desc string
				for _, method := range child.object.Methods {
					name, _ := sourceBridgeUTF8(child.object, method.NameIndex)
					if name != "<init>" {
						continue
					}
					desc, _ = sourceBridgeUTF8(child.object, method.DescriptorIndex)
					for _, attr := range method.Attributes {
						if c, ok := attr.(*CodeAttribute); ok {
							code = c
						}
					}
				}
				ctor := child.constructors[desc]
				if ctor == nil || ctor.enclosingSuperPath == nil || code == nil {
					t.Fatal("original physical super path")
				}
				d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(child.object.ConstantPool, i) })
				if err := d.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := constructorMotionOps(d)
				entries, valid := nativeMemberLexicalControlEntries(d, code, nil)
				if !valid {
					t.Fatal("original control boundaries")
				}
				fieldOp := -1
				for i, op := range ops {
					if int(op.CurrentOffset) == ctor.enclosingSuperPath.pc {
						fieldOp = i
					}
				}
				if fieldOp < 0 {
					t.Fatal("original field read")
				}
				ref := child.object.ConstantPool[core.Convert2bytesToInt(ops[fieldOp].Data)-1].(*ConstantFieldrefInfo)
				nt := child.object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				var work *workbudget.Budget
				switch variant {
				case "cached pc":
					ctor.enclosingSuperPath.pc++
				case "cached base":
					ctor.enclosingSuperPath.basePC++
				case "cached owner":
					ctor.enclosingSuperPath.owner = "Foreign"
				case "cached descriptor":
					ctor.enclosingSuperPath.descriptor = "Ljava/lang/Object;"
				case "cached parameter":
					ctor.enclosingSuperPath.parameterOwner = "Foreign"
				case "cached cycle":
					ctor.enclosingSuperPath.prior = ctor.enclosingSuperPath
				case "missing cached path":
					ctor.enclosingSuperPath = nil
				case "unprojected":
					ctor.projectedSuper = false
				case "wrong constructor descriptor":
					ctor.descriptor = "()V"
				case "wrong capture pc":
					ctor.capturePC++
				case "wrong capture receiver", "wrong capture parameter":
					for i, op := range ops {
						if int(op.CurrentOffset) == ctor.capturePC {
							at := i - 2
							if variant == "wrong capture parameter" {
								at = i - 1
							}
							ops[at].Instr = core.InstrInfos[core.OP_ALOAD_2]
						}
					}
				case "wrong delegate pc":
					ctor.delegatePC++
				case "wrong delegate owner":
					ctor.delegateOwner = "Foreign"
				case "wrong delegate descriptor":
					ctor.delegateDescriptor = "()V"
				case "slot overwrite":
					ops[len(ops)-1].Instr = core.InstrInfos[core.OP_ASTORE_1]
				case "wrong load slot":
					ops[fieldOp-1].Instr = core.InstrInfos[core.OP_ALOAD_2]
				case "changed field owner":
					ref.ClassIndex = child.object.ThisClass
				case "changed field name":
					nt.NameIndex = sourceBridgePoolString(t, child.object, "foreign")
				case "changed field descriptor":
					nt.DescriptorIndex = sourceBridgePoolString(t, child.object, "Ljava/lang/Object;")
				case "alternate entry":
					entries = append(entries, int(ops[fieldOp].CurrentOffset))
				case "work":
					work = workbudget.New(context.Background(), workbudget.Limits{MaxRequestWork: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				path, closed := nativeMemberConstructorSuperRead(child, p, desc, ops, entries, work, reader.buildInvocationMetadata())
				if closed != (variant == "original") {
					t.Fatalf("closed=%v path=%#v", closed, path)
				}
			})
		}
	}
}
