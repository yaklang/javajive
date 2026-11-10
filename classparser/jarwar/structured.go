package jarwar

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/inputkind"
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
	zipFS, err := filesys.NewZipFSFromLocalWithOptions(from, filesys.WithArchiveOptions(archiveOptions))
	if err != nil {
		return ArchiveReport{}, fmt.Errorf("open archive %q: %w", from, err)
	}
	decompiler := javaclassparser.NewJarFS(zipFS)
	decompiler.SetAnalysisBudget(options.MaxAnalysisUpdates)
	decompiler.SetRetainUnprovedSource(false)
	decompilerOwner, err := NewFromJarFS(decompiler)
	if err != nil {
		_ = zipFS.Close()
		return ArchiveReport{}, fmt.Errorf("create archive decompiler: %w", err)
	}

	var report ArchiveReport
	exportErr := exportStructuredArchive(to, zipFS, decompiler, "", 0, &report)
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

func exportStructuredArchive(root string, zipFS *filesys.ZipFS, decompiler *javaclassparser.JarFS, archivePath string, depth int, report *ArchiveReport) error {
	return walkStructuredDirectory(root, zipFS, decompiler, ".", archivePath, depth, report)
}

func walkStructuredDirectory(root string, zipFS *filesys.ZipFS, decompiler *javaclassparser.JarFS, directory, archivePath string, depth int, report *ArchiveReport) error {
	entries, err := decompiler.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read archive directory %q: %w", provenancePath(archivePath, cleanArchivePath(directory)), err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		entryPath := path.Join(directory, javaclassparser.EntryOriginalName(entry))
		entryPath = cleanArchivePath(entryPath)
		if isArchiveName(entryPath) {
			if err := exportNestedArchive(root, zipFS, entryPath, archivePath, depth+1, report); err != nil {
				return err
			}
			continue
		}
		if entry.IsDir() {
			if err := walkStructuredDirectory(root, zipFS, decompiler, entryPath, archivePath, depth, report); err != nil {
				return err
			}
			continue
		}
		if err := exportStructuredFile(root, decompiler, entryPath, archivePath, report); err != nil {
			return err
		}
	}
	return nil
}

func exportNestedArchive(root string, parentZip *filesys.ZipFS, entryPath, parentArchive string, depth int, report *ArchiveReport) error {
	raw, err := parentZip.ReadFile(entryPath)
	if err != nil {
		return fmt.Errorf("read nested archive %q: %w", provenancePath(parentArchive, entryPath), err)
	}
	provenance := provenancePath(parentArchive, entryPath)
	outputPath := path.Join(structuredArchivesRoot, provenance)
	if err := reserveStructuredOutput(report, outputPath); err != nil {
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
	nestedDecompiler.SetRetainUnprovedSource(false)
	if err := exportStructuredArchive(root, nestedZip, nestedDecompiler, provenance, depth, report); err != nil {
		return err
	}
	return nil
}

func exportStructuredFile(root string, decompiler fs.ReadFileFS, entryPath, archivePath string, report *ArchiveReport) error {
	data, err := decompiler.ReadFile(entryPath)
	if err != nil {
		return fmt.Errorf("read archive entry %q: %w", provenancePath(archivePath, entryPath), err)
	}
	kind := "resource"
	outputEntry := entryPath
	outputRoot := structuredResourcesRoot
	if strings.EqualFold(path.Ext(entryPath), ".class") {
		kind = "source"
		outputEntry = strings.TrimSuffix(entryPath, path.Ext(entryPath)) + ".java"
		outputRoot = structuredSourcesRoot
	}
	if archivePath != "" {
		outputEntry = provenancePath(archivePath, outputEntry)
	}
	outputPath := path.Join(outputRoot, outputEntry)
	if err := reserveStructuredOutput(report, outputPath); err != nil {
		return err
	}
	if err := writeStructuredFile(root, outputPath, data); err != nil {
		return fmt.Errorf("write archive entry %q: %w", provenancePath(archivePath, entryPath), err)
	}
	report.Entries = append(report.Entries, ArchiveEntryReport{
		Kind:       kind,
		SourcePath: provenancePath(archivePath, entryPath),
		OutputPath: outputPath,
		Archive:    archivePath,
		Size:       int64(len(data)),
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
