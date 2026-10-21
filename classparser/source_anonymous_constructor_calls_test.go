package javaclassparser

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeSourceStaticAnonymousConstructorCallsRequireOriginalProofAndThrows(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for original static anonymous constructor")
	}
	fixture := strings.NewReplacer("MAKE_STATIC", "static", "OWNER_SEED", "7L", "FACTORY_CALL", "ArtifactLambdaOwner").Replace(artifactAnonymousSuperInitializerFixture)
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "ArtifactLambdaOwner", "none", false)
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	root, err := Parse(bytes.Clone(files["ArtifactLambdaOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(bytes.Clone(files["ArtifactLambdaOwner$1.class"]))
	if err != nil {
		t.Fatal(err)
	}
	d := artifact.jar.nativeMemberReader(root)
	metadata := d.buildInvocationMetadata()
	owner, method, owned := originalAnonymousOwner(object)
	if !owned {
		t.Fatal("actual anonymous ownership")
	}
	original := d.nativeJavac8AnonymousConstructor(object, owner, method, owner, nil, nil, metadata, nil)
	if original == nil || len(original.superParams) != 2 || original.expressionInitializer == nil {
		t.Fatal("actual two-port SUPER and eager object initializer proof")
	}
	for _, variant := range []string{"control", "nil child", "missing descriptor", "changed SUPER descriptor", "changed SUPER PC", "changed SUPER port", "changed capture port", "changed capture PC", "shared port", "nil declarations", "parent missing", "parent incomplete", "parent unknown throws", "parent checked throws", "parent unchecked throws", "initializer missing", "initializer incomplete", "initializer unknown throws", "initializer checked throws", "initializer unchecked throws", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			copy := *original
			child := &copy
			var work *workbudget.Budget
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) {
				declaration, known := metadata(name)
				prefix := "parent "
				target := "ArtifactLambdaOwner$Parent"
				if strings.HasPrefix(variant, "initializer ") {
					prefix, target = "initializer ", "java/util/concurrent/atomic/AtomicBoolean"
				}
				if name != target || !known {
					return declaration, known
				}
				switch variant {
				case prefix + "missing":
					return callbinding.Class{}, false
				case prefix + "incomplete":
					declaration.MembersComplete = false
				}
				declaration.Methods = append([]callbinding.Method(nil), declaration.Methods...)
				for i := range declaration.Methods {
					method := &declaration.Methods[i]
					if method.Name != "<init>" {
						continue
					}
					switch variant {
					case prefix + "unknown throws":
						method.ExceptionsKnown = false
					case prefix + "checked throws":
						method.Exceptions = []string{"java/io/IOException"}
					case prefix + "unchecked throws":
						method.Exceptions = []string{"java/lang/RuntimeException"}
					}
				}
				return declaration, known
			})
			switch variant {
			case "nil child":
				child = nil
			case "missing descriptor":
				child.descriptor = ""
			case "changed SUPER descriptor":
				child.superDescriptor = "(I)V"
			case "changed SUPER PC":
				child.superPC++
			case "changed SUPER port":
				child.superParams = append([]int(nil), child.superParams...)
				child.superParams[0]++
			case "changed capture port", "changed capture PC":
				child.fields, child.capturePCs = maps.Clone(child.fields), maps.Clone(child.capturePCs)
				for field := range child.fields {
					if variant == "changed capture port" {
						child.fields[field]++
					} else {
						child.capturePCs[field]++
					}
					break
				}
			case "shared port":
				child.sharedParameterRoles = true
			case "nil declarations":
				provider = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := sourceAnonymousStaticConstructorDomain(child, provider, work); got != (variant == "control") {
				t.Fatalf("eager-call declared-exception closure=%v", got)
			}
		})
	}
}
