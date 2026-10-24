package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialFlatEnumLambdaPublicationRequiresCompletedAllocationPackets(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, `enum PublicationEnum {
 A { public Long apply(){return ((java.util.function.Function<PublicationEnum,Long>)PublicationEnum::read).apply(A);}}, B;
 private Long read(){return Long.valueOf(7);}public Long apply(){return null;}
}`, "PublicationEnum", "none", false)
	jar := filepath.Join(t.TempDir(), "publication.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	d := a.source.jar.nativeMemberReader(mustParseLowering(t, files["PublicationEnum.class"]))
	d.options.sourceConstructorLowering = a.source.jar.sourceConstructorLowering
	source, err := d.DumpClass()
	if err != nil || len(d.sourceFlatEnumBodies) != 1 || len(d.sourceFlatEnumPackets) != 2 {
		t.Fatal("actual typed constant publication", err, source)
	}
	authority := a.source.jar.sourceLambdaMetadata
	pub := authority.publication("PublicationEnum", source)
	if pub == nil || len(pub.plans) != 1 || authority.plan(pub.plans[0].owner) != nil {
		t.Fatal("provisional child acquired active authority")
	}
	for _, variant := range []string{"control", "missing packet", "reordered packets", "packet changed with source", "body changed with packet and source", "missing body", "duplicate body", "source in comment", "body changed", "source hash changed", "original hash changed", "wrong allocation", "wrong forwarding", "duplicate owner", "degraded owner", "parent changed", "work", "output"} {
		t.Run(variant, func(t *testing.T) {
			copy := *d
			copy.sourceFlatEnumBodies = slices.Clone(d.sourceFlatEnumBodies)
			copy.sourceFlatEnumPackets = slices.Clone(d.sourceFlatEnumPackets)
			copy.sourceFlatEnumPacketSeal = slices.Clone(d.sourceFlatEnumPacketSeal)
			member := &copy.sourceFlatEnumBodies[0]
			proof := *member.proof
			member.proof = &proof
			publication := *member.publication
			member.publication = &publication
			text := source
			switch variant {
			case "missing packet":
				copy.sourceFlatEnumPackets = copy.sourceFlatEnumPackets[:1]
			case "reordered packets":
				slices.Reverse(copy.sourceFlatEnumPackets)
			case "packet changed with source":
				copy.sourceFlatEnumPackets[0].source = strings.Replace(copy.sourceFlatEnumPackets[0].source, member.body, " {}", 1)
				text = strings.Replace(text, member.body, " {}", 1)
			case "body changed with packet and source":
				changed := member.body + " "
				copy.sourceFlatEnumPackets[0].source = strings.Replace(copy.sourceFlatEnumPackets[0].source, member.body, changed, 1)
				copy.sourceFlatEnumPacketSeal = slices.Clone(copy.sourceFlatEnumPackets)
				text = strings.Replace(text, member.body, changed, 1)
				member.body = changed
			case "missing body":
				text = strings.Replace(text, member.body, " {}", 1)
			case "duplicate body":
				text = strings.Replace(text, member.body, member.body+member.body, 1)
			case "source in comment":
				text = "enum PublicationEnum { /*" + strings.Join([]string{copy.sourceFlatEnumPackets[0].source, copy.sourceFlatEnumPackets[1].source}, "") + "*/ A, B; }"
			case "body changed":
				member.body += " "
			case "source hash changed":
				member.sourceHash[0] ^= 1
			case "original hash changed":
				publication.originalHash[0] ^= 1
			case "wrong allocation":
				proof.plan.constant = "B"
			case "wrong forwarding":
				proof.superPC++
			case "duplicate owner":
				copy.sourceFlatEnumBodies = append(copy.sourceFlatEnumBodies, *member)
			case "degraded owner":
				copy.sourceDegraded = true
			case "parent changed":
				copy.obj = mustParseLowering(t, bytes.Clone(files["PublicationEnum.class"]))
				copy.obj.AccessFlags ^= 0x10
			case "work":
				copy.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				copy.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			plans, closed := copy.flatEnumPublicationsClosed(text)
			if closed != (variant == "control") || closed && len(plans) != 1 || !closed && plans != nil {
				t.Fatalf("allocation/source closure=%v variant=%s", closed, variant)
			}
			if authority.plan(pub.plans[0].owner) != nil {
				t.Fatal("local closure activated unpublished source")
			}
		})
	}
	if err := authority.sourceReturned("PublicationEnum", []byte(source+" ")); err != nil || authority.plan(pub.plans[0].owner) != nil {
		t.Fatal("changed source acquired exact receipt", err)
	}
	if err := authority.sourceReturned("PublicationEnum", []byte(source)); err != nil || authority.plan(pub.plans[0].owner) != pub.plans[0] {
		t.Fatal("complete source did not activate owned lambda", err)
	}
}
