package javaclassparser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

// One unchanged foreign constructor caller is parsed independently into every
// plan. Its unused UTF8 constant changes retained input size, not declarations,
// bytecode, dependency edges or emitted bodies. Count the actual model copies.
func TestAdversarialSourceComponentBuildersBoundRetainedForeignCallerModels(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "ordinary"
		if independent {
			name = "independent"
		}
		t.Run(name, func(t *testing.T) {
			const count = 16
			var source, calls strings.Builder
			source.WriteString("abstract class RetainedParent{final int seen;RetainedParent(){seen=read();}abstract int read();}\n")
			names := make([]string, count)
			for i := 0; i < count; i++ {
				root, peer := fmt.Sprintf("RetainedFamily%d", i), fmt.Sprintf("RetainedFamily%d", (i+1)%count)
				if independent {
					root += ".Capsule"
					peer += ".Capsule"
					fmt.Fprintf(&source, "class RetainedFamily%d{static ", i)
					names[i] = fmt.Sprintf("RetainedFamily%d$Capsule", i)
				} else {
					names[i] = root
				}
				declared := root
				if independent {
					declared = "Capsule"
				}
				fmt.Fprintf(&source, "class %s{private final int value;%s(int value){this.value=value;}static class Token{final int value;Token(int value){this.value=value;}}final class Leaf extends RetainedParent{final %s.Token peer;Leaf(%s.Token peer){this.peer=peer;}int read(){return %s.this.value;}int sum(){return read()+peer.value;}}Leaf make(%s.Token peer){return new Leaf(peer);}}\n", declared, declared, peer, peer, declared, peer)
				if independent {
					source.WriteString("}\n")
				}
				fmt.Fprintf(&calls, "new %s.Token(%d);", root, i)
			}
			fmt.Fprintf(&source, "class RetainedCaller{static void touch(){%s}}", calls.String())
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"RetainedFamily0.java": source.String()}, "none", "8")
			caller, err := Parse(files["RetainedCaller.class"])
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&caller.ConstantPool)
			pool.AddUtf8Info(strings.Repeat("x", 32768))
			padded := caller.Bytes()
			for _, padding := range []bool{false, true} {
				label := "compact caller"
				if padding {
					label = "distinct retained padded caller copies"
				}
				t.Run(label, func(t *testing.T) {
					input := map[string][]byte{}
					for name, raw := range files {
						input[name] = bytes.Clone(raw)
					}
					if padding {
						input["RetainedCaller.class"] = padded
					}
					z := nativeArchive(t, input)
					defer z.Close()
					snap := snapshotJDECEnv()
					parts, cyclic, known := z.nativeMemberDependencyAdmission(names[0], nil, snap)
					if !known || !cyclic || len(parts) != count {
						t.Fatal("original component not established", known, cyclic, len(parts))
					}
					work := workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: int64(len(padded))})
					z.archive = &archiveFSState{ctx: context.Background(), budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{})}
					var results map[string]*nativeMemberCacheEntry
					if independent {
						results = z.buildNativeIndependentTransaction(parts, snap)
					} else {
						results = z.buildNativeMemberTransaction(parts, snap)
					}
					if !padding {
						if len(results) != count || z.nativeMembersBytes == 0 || z.sourceOwnership.bytes != z.nativeMembersBytes {
							t.Fatal("compact original component did not complete", len(results), work.Check())
						}
					} else if results != nil || z.nativeMembersBytes != 0 || z.sourceOwnership.bytes != 0 {
						t.Fatal("oversized retained models published component source")
					}
				})
			}
		})
	}
}
