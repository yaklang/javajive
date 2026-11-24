package jarwar

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/inputkind"
	"github.com/yaklang/javajive/internal/jdecenv"
)

const (
	structuredSourcesRoot   = "sources"
	structuredResourcesRoot = "resources"
	structuredArchivesRoot  = "archives"
)

func decompileArchiveStructured(from, to string, options ArchiveOptions) (ArchiveReport, error) {
	if !isArchiveName(from) {
		return ArchiveReport{}, fmt.Errorf("structured archive export requires .jar, .war, .ear, .par, or .zip input: %s", from)
	}
	if err := prepareStructuredRoot(to); err != nil {
		return ArchiveReport{}, err
	}

	archiveOptions := javaclassparser.ArchiveOptionsFrom(javaclassparser.DecompileOptions{
		Context:       options.Context,
		Limits:        options.Limits,
		Work:          options.Work,
		TargetRelease: options.TargetRelease,
	})
	archiveOptions.AllowCaseFoldCollisions = options.RetainUnprovedSource
	zipFS, err := filesys.NewZipFSFromLocalWithOptions(from, filesys.WithArchiveOptions(archiveOptions))
	if err != nil {
		return ArchiveReport{}, fmt.Errorf("open archive %q: %w", from, err)
	}
	decompiler := javaclassparser.NewJarFS(zipFS)
	decompiler.SetAnalysisBudget(options.MaxAnalysisUpdates)
	decompiler.SetRetainUnprovedSource(options.RetainUnprovedSource)
	snap, bound := jdecenv.Current()
	liveEnv := bound && snap == nil
	if !bound {
		snap = jdecenv.CaptureProcess()
	}
	if !liveEnv {
		decompiler.SetEnvSnapshot(snap)
	}
	decompilerOwner, err := NewFromJarFS(decompiler)
	if err != nil {
		_ = zipFS.Close()
		return ArchiveReport{}, fmt.Errorf("create archive decompiler: %w", err)
	}

	var report ArchiveReport
	var usedFold map[string]int
	if options.RetainUnprovedSource {
		usedFold = make(map[string]int)
	}
	var exportErr error
	runStructured := func() error {
		return exportStructuredArchive(to, zipFS, decompiler, "", 0, options.MaxAnalysisUpdates, options.RetainUnprovedSource, usedFold, &report)
	}
	if liveEnv {
		exportErr = jdecenv.RunLive(runStructured)
	} else {
		exportErr = jdecenv.Run(snap, runStructured)
	}
	closeErr := decompilerOwner.Close()
	if exportErr != nil {
		return ArchiveReport{}, exportErr
	}
	if closeErr != nil {
		return ArchiveReport{}, fmt.Errorf("close archive %q: %w", from, closeErr)
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		if report.Entries[i].OutputPath != report.Entries[j].OutputPath {
			return report.Entries[i].OutputPath < report.Entries[j].OutputPath
		}
		return report.Entries[i].SourcePath < report.Entries[j].SourcePath
	})
	sort.Slice(report.Archives, func(i, j int) bool {
		return report.Archives[i].Path < report.Archives[j].Path
	})
	return report, nil
}

func prepareStructuredRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return pathRejected("empty root")
	}
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return pathRejected("symlink root")
		}
		if !info.IsDir() {
			return fmt.Errorf("structured export root is not a directory: %s", root)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create structured export root %q: %w", root, err)
	}
	for _, name := range []string{structuredSourcesRoot, structuredResourcesRoot, structuredArchivesRoot} {
		target, err := SafeJoin(root, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("create structured export directory %q: %w", target, err)
		}
	}
	return nil
}

func exportStructuredArchive(root string, zipFS *filesys.ZipFS, decompiler *javaclassparser.JarFS, archivePath string, depth, maxAnalysisUpdates int, retainUnproved bool, usedFold map[string]int, report *ArchiveReport) error {
	return walkStructuredDirectory(root, zipFS, decompiler, ".", archivePath, depth, maxAnalysisUpdates, retainUnproved, usedFold, report)
}

func walkStructuredDirectory(root string, zipFS *filesys.ZipFS, decompiler *javaclassparser.JarFS, directory, archivePath string, depth, maxAnalysisUpdates int, retainUnproved bool, usedFold map[string]int, report *ArchiveReport) error {
	entries, err := decompiler.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read archive directory %q: %w", provenancePath(archivePath, cleanArchivePath(directory)), err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		entryPath := path.Join(directory, javaclassparser.EntryOriginalName(entry))
		entryPath = cleanArchivePath(entryPath)
		if isArchiveName(entryPath) {
			if err := exportNestedArchive(root, zipFS, entryPath, archivePath, depth+1, maxAnalysisUpdates, retainUnproved, usedFold, report); err != nil {
				return err
			}
			continue
		}
		if entry.IsDir() {
			if err := walkStructuredDirectory(root, zipFS, decompiler, entryPath, archivePath, depth, maxAnalysisUpdates, retainUnproved, usedFold, report); err != nil {
				return err
			}
			continue
		}
		if err := exportStructuredFile(root, decompiler, entryPath, archivePath, retainUnproved, usedFold, report); err != nil {
			return err
		}
	}
	return nil
}

func exportNestedArchive(root string, parentZip *filesys.ZipFS, entryPath, parentArchive string, depth, maxAnalysisUpdates int, retainUnproved bool, usedFold map[string]int, report *ArchiveReport) error {
	raw, err := parentZip.ReadFile(entryPath)
	if err != nil {
		return fmt.Errorf("read nested archive %q: %w", provenancePath(parentArchive, entryPath), err)
	}
	provenance := provenancePath(parentArchive, entryPath)
	outputPath := path.Join(structuredArchivesRoot, provenance)
	if retainUnproved {
		outputPath = disambiguateCaseFoldPath(sanitizeBestEffortPath(outputPath), usedFold)
	} else if err := reserveStructuredOutput(report, outputPath); err != nil {
		return err
	}
	if err := writeStructuredFile(root, outputPath, raw); err != nil {
		return fmt.Errorf("preserve nested archive %q: %w", provenance, err)
	}
	report.Archives = append(report.Archives, NestedArchiveReport{
		Path:       provenance,
		Parent:     parentArchive,
		OutputPath: outputPath,
		Depth:      depth,
		Size:       int64(len(raw)),
	})

	payload := inputkind.StripJMODHeader(raw)
	nestedZip, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(payload), int64(len(payload)), parentZip.NestedArchiveOptions(depth)...)
	if err != nil {
		return fmt.Errorf("open nested archive %q: %w", provenance, err)
	}
	nestedDecompiler := javaclassparser.NewJarFS(nestedZip)
	nestedDecompiler.SetAnalysisBudget(maxAnalysisUpdates)
	nestedDecompiler.SetRetainUnprovedSource(retainUnproved)
	if snap, ok := jdecenv.Current(); ok && snap != nil {
		nestedDecompiler.SetEnvSnapshot(snap)
	}
	if err := exportStructuredArchive(root, nestedZip, nestedDecompiler, provenance, depth, maxAnalysisUpdates, retainUnproved, usedFold, report); err != nil {
		return err
	}
	return nil
}

func exportStructuredFile(root string, decompiler *javaclassparser.JarFS, entryPath, archivePath string, retainUnproved bool, usedFold map[string]int, report *ArchiveReport) error {
	outcome, err := decompiler.ReadClassOutcome(entryPath)
	if err != nil {
		return fmt.Errorf("read archive entry %q: %w", provenancePath(archivePath, entryPath), err)
	}
	data := outcome.Source
	kind := "resource"
	outputEntry := entryPath
	outputRoot := structuredResourcesRoot
	status := javaclassparser.ClassDecompileStatus("")
	var stubMethods []string
	var diagnostics []javaclassparser.DecompileDiagnostic
	if strings.EqualFold(path.Ext(entryPath), ".class") {
		kind = "source"
		outputEntry = strings.TrimSuffix(entryPath, path.Ext(entryPath)) + ".java"
		outputRoot = structuredSourcesRoot
		status = outcome.Status
		stubMethods = outcome.StubMethods
		diagnostics = outcome.Diagnostics
	}
	if archivePath != "" {
		outputEntry = provenancePath(archivePath, outputEntry)
	}
	outputPath := path.Join(outputRoot, outputEntry)
	if retainUnproved {
		sanitized := sanitizeBestEffortPath(outputPath)
		if kind == "source" {
			sanitized = sanitizeBestEffortClassPath(outputPath)
		}
		outputPath = disambiguateCaseFoldPath(sanitized, usedFold)
	} else if err := reserveStructuredOutput(report, outputPath); err != nil {
		return err
	}
	if err := writeStructuredFile(root, outputPath, data); err != nil {
		return fmt.Errorf("write archive entry %q: %w", provenancePath(archivePath, entryPath), err)
	}
	report.Entries = append(report.Entries, ArchiveEntryReport{
		Kind:        kind,
		SourcePath:  provenancePath(archivePath, entryPath),
		OutputPath:  outputPath,
		Archive:     archivePath,
		Size:        int64(len(data)),
		Status:      status,
		StubMethods: stubMethods,
		Diagnostics: diagnostics,
	})
	return nil
}

func reserveStructuredOutput(report *ArchiveReport, outputPath string) error {
	folded := strings.ToLower(outputPath)
	for _, entry := range report.Entries {
		if strings.ToLower(entry.OutputPath) == folded {
			return pathRejected("structured output collision")
		}
	}
	for _, archive := range report.Archives {
		if strings.ToLower(archive.OutputPath) == folded {
			return pathRejected("structured output collision")
		}
	}
	return nil
}

func writeStructuredFile(root, name string, data []byte) error {
	target, err := SafeJoin(root, name)
	if err != nil {
		return err
	}
	parent, err := SafeJoin(root, path.Dir(name))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return err
	}
	return os.Chmod(target, 0o644)
}

func cleanArchivePath(name string) string {
	name = strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
	if name == "." {
		return ""
	}
	return name
}

func provenancePath(archivePath, entryPath string) string {
	entryPath = cleanArchivePath(entryPath)
	if archivePath == "" {
		return entryPath
	}
	if entryPath == "" {
		return archivePath
	}
	return archivePath + "!/" + entryPath
}

func isArchiveName(name string) bool {
	lower := strings.ToLower(cleanArchivePath(name))
	for _, suffix := range []string{".jar", ".war", ".ear", ".aar", ".par", ".jmod", ".zip"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
