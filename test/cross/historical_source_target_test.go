package cross

import (
	"os"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

type historicalSourceCompilation struct {
	files                   []string
	units, failures, target int
	artifact                *classparser.JarSourceArtifact
	reconstruction          *classparser.JarReconstructionArtifact
}

func (p *historicalSourceCompilation) close() {
	if p.artifact != nil {
		p.artifact.Close()
	}
	if p.reconstruction != nil {
		p.reconstruction.Close()
	}
}

// The compiler's existing SDK selection is shared by source publication.
// An old bytecode version does not imply that all referenced APIs existed in
// that JDK. Versioned MR sources retain their separate physical compiler pass.
func historicalBaseCompileTarget(files []string, minimum int) int {
	if minimum < 8 {
		minimum = 8
	}
	base, _, _ := splitMRFiles(files)
	for _, file := range base {
		if minimum < 9 && isJava9SslAlpnFile(file) {
			minimum = 9
		}
	}
	return minimum
}

// Source capabilities bind a selected compiler, not merely the original
// bytecode version. If the existing SDK policy raises that target, discard
// provisional capabilities and render again before compiling or accepting
// metadata. This never grants a class an arbitrary target-version exception.
func historicalRenderAtCompilerTarget(t *testing.T, path, directory string, target int, policy string, resolve func(string) ([]byte, bool)) *historicalSourceCompilation {
	t.Helper()
	if target < 8 {
		target = 8
	}
	for attempt := 0; attempt < 2; attempt++ {
		p := &historicalSourceCompilation{target: target}
		var err error
		switch policy {
		case "reconstruction":
			p.reconstruction, err = classparser.NewJarReconstructionArtifactFromLocal(path, target, classparser.ModernJavac, resolve)
			if err == nil {
				p.files, p.units, p.failures = decompileAllFromSourceReader(t, path, directory, 0, p.reconstruction)
			}
		case "source":
			p.artifact, err = classparser.NewJarSourceArtifactFromLocal(path, target, classparser.ModernJavac, resolve)
			if err == nil {
				p.files, p.units, p.failures = decompileAllFromSourceReader(t, path, directory, 0, p.artifact)
			}
		case "plain":
			p.files, p.units, p.failures = decompileAllWithResolverAtSourceVersion(t, path, directory, 0, resolve, target)
		default:
			t.Fatal("unknown source artifact policy", policy)
		}
		if err != nil {
			p.close()
			t.Fatal("selected source compiler", err)
		}
		selected := historicalBaseCompileTarget(p.files, target)
		if selected == target {
			return p
		}
		p.close()
		// Only files emitted by this provisional attempt are removed. In particular
		// no target class, dependency, support fixture or previous observation is
		// eligible to be treated as generated source here.
		for _, file := range p.files {
			if err := os.Remove(file); err != nil {
				t.Fatal("discard provisional source", err)
			}
		}
		target = selected
	}
	t.Fatal("source/compiler target did not converge")
	return nil
}
