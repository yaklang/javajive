package jarwar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/utils"
)

type JarWar struct {
	// License 存储 JAR/WAR 文件中的许可证信息
	// 通常从 META-INF/LICENSE 文件中读取
	License    string
	ManifestMF JarManifest
	fs         *javaclassparser.JarFS

	// 存储反编译失败的文件列表
	failedDecompiledFiles map[string]struct{}
	failedFilesLock       sync.RWMutex
}

func NewFromJarFS(fs *javaclassparser.JarFS) (*JarWar, error) {
	// Check jar or war
	if entries, err := fs.ReadDir("WEB-INF/"); err == nil && len(entries) > 0 {
		return &JarWar{
			fs:                    fs,
			failedDecompiledFiles: make(map[string]struct{}),
		}, nil
	} else if manifest, err := fs.ZipFS.ReadFile("META-INF/MANIFEST.MF"); err == nil && len(manifest) > 0 {
		if strings.Contains(string(manifest), "Manifest-Version") {
			result := &JarWar{
				ManifestMF:            ParseJarManifest(string(manifest)),
				fs:                    fs,
				failedDecompiledFiles: make(map[string]struct{}),
			}
			if license, err := fs.ZipFS.ReadFile("META-INF/LICENSE"); err == nil {
				result.License = string(license)
			}

			return result, nil
		}
	}
	// Plain ZIP/JAR/WAR/EAR without standard manifest metadata — still allow decompilation.
	return &JarWar{
		fs:                    fs,
		failedDecompiledFiles: make(map[string]struct{}),
	}, nil
}

func New(compressedFile string) (*JarWar, error) {
	return NewWithAnalysisBudget(compressedFile, 0)
}

// NewWithAnalysisBudget opens an archive with an explicit per-method
// reaching-definition budget. Positive budgets above 1,000,000 opt into the
// large-method graph scaling.
func NewWithAnalysisBudget(compressedFile string, maxAnalysisUpdates int) (*JarWar, error) {
	if utils.GetFirstExistedFile(compressedFile) == "" {
		return nil, fmt.Errorf("file not existed: %s", compressedFile)
	}
	if isJImage(compressedFile) {
		return nil, fmt.Errorf("JImage input is supported by AutoDecompile/DecompileArchive, not New: %s", compressedFile)
	}
	fs, err := javaclassparser.NewJarFSFromLocalWithAnalysisBudget(compressedFile, 0, maxAnalysisUpdates, nil)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create jar fs")
	}
	fsIns, err := NewFromJarFS(fs)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create jarwar")
	}
	return fsIns, nil
}

// Close releases the underlying archive file handle held by the JarFS/ZipFS.
// This is important on Windows, where an open file cannot be deleted by callers.
func (j *JarWar) Close() error {
	if j.fs != nil {
		return j.fs.Close()
	}
	return nil
}

func (j *JarWar) GetStructDump() string {
	return filesys.DumpTreeView(j.fs)
}

func (j *JarWar) DumpToLocalFileSystem(dir string) error {
	if utils.GetFirstExistedPath(dir) == "" {
		log.Info("output directory not existed, create it, os.MkdirAll")
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return utils.Wrap(err, "os.MkdirAll failed")
		}
	}

	if err := j.dumpDirectory(dir, "."); err != nil {
		return fmt.Errorf("archive file system dump failed: %w", err)
	}
	return nil
}

func (j *JarWar) dumpDirectory(root, directory string) error {
	entries, err := j.fs.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read archive directory %q: %w", directory, err)
	}
	for _, entry := range entries {
		entryPath := filepath.ToSlash(filepath.Join(directory, entry.Name()))
		info, statErr := j.fs.Stat(entryPath)
		if statErr != nil {
			return fmt.Errorf("stat archive entry %q: %w", entryPath, statErr)
		}
		if info.IsDir() {
			target, joinErr := SafeJoin(root, entryPath)
			if joinErr != nil {
				return fmt.Errorf("resolve archive directory %q: %w", entryPath, joinErr)
			}
			if mkdirErr := os.MkdirAll(target, 0o755); mkdirErr != nil {
				return fmt.Errorf("create archive directory %q: %w", entryPath, mkdirErr)
			}
			if walkErr := j.dumpDirectory(root, entryPath); walkErr != nil {
				return walkErr
			}
			continue
		}
		if err := j.dumpFile(root, entryPath); err != nil {
			return err
		}
	}
	return nil
}

func (j *JarWar) dumpFile(root, visiblePath string) error {
	entryPath := visiblePath
	if filepath.Ext(entryPath) == ".java" {
		if _, err := j.fs.ReadRawFile(entryPath); err != nil {
			entryPath = strings.TrimSuffix(entryPath, ".java") + ".class"
		}
	}

	if filepath.Ext(entryPath) == ".class" {
		originalRaw, err := j.fs.ReadRawFile(entryPath)
		if err != nil {
			return fmt.Errorf("read archive entry %q: %w", entryPath, err)
		}
		decompiled, err := j.fs.ReadFile(entryPath)
		if err != nil {
			j.failedFilesLock.Lock()
			j.failedDecompiledFiles[visiblePath] = struct{}{}
			j.failedFilesLock.Unlock()
			return fmt.Errorf("decompile archive entry %q: %w", entryPath, err)
		}
		javaName := strings.TrimSuffix(entryPath, ".class") + ".java"
		if err := writeFlatFile(root, javaName, decompiled); err != nil {
			return fmt.Errorf("write archive entry %q: %w", entryPath, err)
		}
		lines := strings.Count(string(decompiled), "\n") + 1
		log.Infof("Decompiled [%v] - Original size: %d bytes, Decompiled size: %d bytes, Lines of code: %d",
			visiblePath, len(originalRaw), len(decompiled), lines)
		return nil
	}

	raw, err := j.fs.ReadRawFile(entryPath)
	if err != nil {
		return fmt.Errorf("read archive entry %q: %w", entryPath, err)
	}
	if err := writeFlatFile(root, visiblePath, raw); err != nil {
		return fmt.Errorf("write archive entry %q: %w", entryPath, err)
	}
	return nil
}

func writeFlatFile(root, name string, data []byte) error {
	target, err := SafeJoin(root, name)
	if err != nil {
		return err
	}
	parent, err := SafeJoin(root, filepath.ToSlash(filepath.Dir(name)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}
