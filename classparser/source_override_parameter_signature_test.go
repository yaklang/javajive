package javaclassparser

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialProjectedOverrideRequiresExactInstantiatedDeclaration(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	_, _, artifact := constructorSuffixFamilyRoundTrip(t, projectedOverrideSignatureFixture, "none", []string{"SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task", "SuffixOwner$Sub"}, "12:projected:override:identity:binding\n")
	base := artifact.source.jar.sourceConstructorLowering
	files := map[string][]byte{}
	for _, name := range []string{"SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task", "SuffixOwner$Sub"} {
		raw, e := base.jar.readJarEntry(name + ".class")
		if e != nil {
			t.Fatal(e)
		}
		files[name+".class"] = raw
	}
	for _, variant := range []string{"control", "same signature", "wrong fresh argument", "wrong fresh result", "changed throws", "missing parent", "missing owner", "unreturned parent", "private parent", "static parent", "final parent", "native parent", "duplicate declaration", "unrelated method", "wrong superclass argument", "private child", "unknown child binder", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child, parent := mustParseLowering(t, files["SuffixOwner$Sub.class"]), mustParseLowering(t, files["SuffixOwner$Task.class"])
			method := func(o *ClassObject) *MemberInfo {
				for _, m := range o.Methods {
					name, _ := sourceBridgeUTF8(o, m.NameIndex)
					if name == "apply" {
						return m
					}
				}
				t.Fatal("missing apply")
				return nil
			}
			m, p := method(child), method(parent)
			old, valid := nativeMethodLocalOriginalSignature(child, m.Attributes, nil)
			if !valid {
				t.Fatal("original signature")
			}
			fresh := "(LSuffixOwner$Pair;)TV;"
			var work *workbudget.Budget
			switch variant {
			case "same signature":
				fresh = old
			case "wrong fresh argument":
				fresh = "(LSuffixOwner$Pair<TK;TV;>;)TV;"
			case "wrong fresh result":
				fresh = "(LSuffixOwner$Pair;)Ljava/lang/Object;"
			case "changed throws":
				fresh += "^Ljava/io/IOException;"
			case "private parent":
				p.AccessFlags |= 2
			case "static parent":
				p.AccessFlags |= StaticFlag
			case "final parent":
				p.AccessFlags |= 16
			case "native parent":
				p.AccessFlags |= 256
			case "duplicate declaration":
				parent.Methods = append(parent.Methods, p)
			case "unrelated method":
				p.NameIndex = uint16(parent.ConstantPoolManager.AddUtf8Info("other"))
			case "wrong superclass argument":
				for _, attr := range child.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						s, _ := sourceBridgeUTF8(child, sig.SignatureIndex)
						sig.SignatureIndex = uint16(child.ConstantPoolManager.AddUtf8Info(strings.ReplaceAll(s, ".Task<TV;>", ".Task<TK;>")))
					}
				}
			case "private child":
				m.AccessFlags |= 2
			case "unknown child binder":
				old = strings.ReplaceAll(old, "TK;", "TUnknown;")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			modified := map[string][]byte{}
			for k, v := range files {
				modified[k] = v
			}
			modified["SuffixOwner$Task.class"] = parent.Bytes()
			modified["SuffixOwner$Sub.class"] = child.Bytes()
			if variant == "missing parent" {
				delete(modified, "SuffixOwner$Task.class")
			}
			if variant == "missing owner" {
				delete(modified, "SuffixOwner.class")
			}
			path := filepath.Join(t.TempDir(), "original.jar")
			if e := os.WriteFile(path, t23Zip(t, modified), 0600); e != nil {
				t.Fatal(e)
			}
			local, e := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer local.Close()
			active := map[string]constructorNativeSourceOrigin{}
			for k, v := range base.genericActive {
				active[k] = v
			}
			// Grant only the mutated parent bytes, so structural negatives
			// reach the proof instead of failing at an unrelated stale receipt.
			origin := active["SuffixOwner$Task"]
			origin.originalHash = sha256.Sum256(modified["SuffixOwner$Task.class"])
			active["SuffixOwner$Task"] = origin
			if variant == "unreturned parent" {
				delete(active, "SuffixOwner$Task")
			}
			a := &sourceConstructorLoweringAuthority{jar: local.source.jar, genericActive: active}
			if got := a.projectedOverrideSignature(child, m, old, fresh, work); got != (variant == "control") {
				t.Fatalf("accepted=%v variant=%s", got, variant)
			}
		})
	}
}
