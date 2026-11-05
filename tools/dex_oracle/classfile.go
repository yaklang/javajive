package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/yaklang/javajive/internal/mutf8"
)

const (
	maxJarEntries     = 100000
	maxJarExpanded    = int64(512 << 20)
	maxClassfileBytes = int64(32 << 20)
	maxConstantPool   = 65535
)

func inventoryJar(filename string) jarArtifact {
	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return jarArtifact{Present: false}
		}
		return jarArtifact{Present: false, Error: err.Error()}
	}
	result := jarArtifact{Present: true, Size: info.Size(), Entries: []string{}, Classes: []classInventory{}}
	if !info.Mode().IsRegular() || info.Size() > maxOutputJarBytes {
		result.Error = fmt.Sprintf("output JAR is not a regular file within %d bytes", maxOutputJarBytes)
		return result
	}
	result.SHA256, _, err = hashFile(filename, maxOutputJarBytes)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	r, err := zip.OpenReader(filename)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer r.Close()
	if len(r.File) > maxJarEntries {
		result.Error = fmt.Sprintf("JAR has too many entries: %d", len(r.File))
		return result
	}
	var expanded int64
	for _, f := range r.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !safeRelativePath(name) {
			result.Error = fmt.Sprintf("unsafe JAR entry %q", f.Name)
			return result
		}
		expanded += int64(f.UncompressedSize64)
		if expanded > maxJarExpanded {
			result.Error = "JAR expanded size exceeds limit"
			return result
		}
		result.Entries = append(result.Entries, f.Name)
		if strings.HasSuffix(f.Name, ".class") && !f.FileInfo().IsDir() {
			class, err := parseClassEntry(f)
			if err != nil {
				result.Error = fmt.Sprintf("parse %s: %v", f.Name, err)
				continue
			}
			class.Entry = f.Name
			result.Classes = append(result.Classes, class)
		}
	}
	sort.Strings(result.Entries)
	sort.Slice(result.Classes, func(i, j int) bool { return result.Classes[i].Entry < result.Classes[j].Entry })
	return result
}

func inventoryZip(filename string) archiveArtifact {
	info, err := os.Stat(filename)
	if err != nil {
		return archiveArtifact{Present: false, Error: err.Error()}
	}
	result := archiveArtifact{Present: true, Name: info.Name(), Size: info.Size(), Entries: []string{}}
	if !info.Mode().IsRegular() || info.Size() > maxOutputJarBytes {
		result.Error = "error zip is not a bounded regular file"
		return result
	}
	result.SHA256, _, err = hashFile(filename, maxOutputJarBytes)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	r, err := zip.OpenReader(filename)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer r.Close()
	if len(r.File) > maxJarEntries {
		result.Error = "error zip has too many entries"
		return result
	}
	var expanded int64
	for _, f := range r.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !safeRelativePath(name) {
			result.Error = fmt.Sprintf("unsafe zip entry %q", f.Name)
			return result
		}
		expanded += int64(f.UncompressedSize64)
		if expanded > maxJarExpanded {
			result.Error = "error zip expanded size exceeds limit"
			return result
		}
		result.Entries = append(result.Entries, f.Name)
	}
	sort.Strings(result.Entries)
	return result
}

func parseClassEntry(f *zip.File) (classInventory, error) {
	if int64(f.UncompressedSize64) > maxClassfileBytes {
		return classInventory{}, fmt.Errorf("class exceeds size limit")
	}
	r, err := f.Open()
	if err != nil {
		return classInventory{}, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, maxClassfileBytes+1))
	if err != nil {
		return classInventory{}, err
	}
	if int64(len(b)) > maxClassfileBytes || uint64(len(b)) != f.UncompressedSize64 {
		return classInventory{}, fmt.Errorf("class size mismatch")
	}
	return parseClass(b)
}

type classReader struct {
	r    *bytes.Reader
	utf8 []string
	cls  []uint16
}

func parseClass(data []byte) (classInventory, error) {
	c := &classReader{r: bytes.NewReader(data)}
	magic, err := c.u4()
	if err != nil || magic != 0xcafebabe {
		return classInventory{}, fmt.Errorf("invalid class magic")
	}
	if err := c.skip(4); err != nil {
		return classInventory{}, err
	}
	count, err := c.u2()
	if err != nil || count == 0 || count > maxConstantPool {
		return classInventory{}, fmt.Errorf("invalid constant pool count")
	}
	c.utf8 = make([]string, count)
	c.cls = make([]uint16, count)
	for i := uint16(1); i < count; i++ {
		tag, err := c.u1()
		if err != nil {
			return classInventory{}, err
		}
		switch tag {
		case 1:
			length, err := c.u2()
			if err != nil {
				return classInventory{}, err
			}
			value := make([]byte, length)
			if _, err := io.ReadFull(c.r, value); err != nil {
				return classInventory{}, err
			}
			units, err := mutf8.DecodeBounded(value, int(length))
			if err != nil {
				return classInventory{}, fmt.Errorf("invalid modified UTF-8: %w", err)
			}
			for index := 0; index < len(units); index++ {
				unit := units[index]
				switch {
				case unit >= 0xd800 && unit <= 0xdbff:
					if index+1 >= len(units) || units[index+1] < 0xdc00 || units[index+1] > 0xdfff {
						return classInventory{}, fmt.Errorf("unpaired surrogate in constant pool UTF-8")
					}
					index++
				case unit >= 0xdc00 && unit <= 0xdfff:
					return classInventory{}, fmt.Errorf("unpaired surrogate in constant pool UTF-8")
				}
			}
			c.utf8[i] = string(utf16.Decode(units))
		case 3, 4:
			if err := c.skip(4); err != nil {
				return classInventory{}, err
			}
		case 5, 6:
			if err := c.skip(8); err != nil {
				return classInventory{}, err
			}
			i++
		case 7:
			index, err := c.u2()
			if err != nil {
				return classInventory{}, err
			}
			c.cls[i] = index
		case 8, 16, 19, 20:
			if err := c.skip(2); err != nil {
				return classInventory{}, err
			}
		case 9, 10, 11, 12, 17, 18:
			if err := c.skip(4); err != nil {
				return classInventory{}, err
			}
		case 15:
			if err := c.skip(3); err != nil {
				return classInventory{}, err
			}
		default:
			return classInventory{}, fmt.Errorf("unknown constant pool tag %d", tag)
		}
	}
	if err := c.skip(2); err != nil {
		return classInventory{}, err
	}
	thisClass, err := c.u2()
	if err != nil {
		return classInventory{}, err
	}
	if err := c.skip(2); err != nil {
		return classInventory{}, err
	}
	name, err := c.className(thisClass)
	if err != nil {
		return classInventory{}, err
	}
	interfaces, err := c.u2()
	if err != nil {
		return classInventory{}, err
	}
	if err := c.skip(int64(interfaces) * 2); err != nil {
		return classInventory{}, err
	}
	fields, err := c.members()
	if err != nil {
		return classInventory{}, err
	}
	methods, err := c.members()
	if err != nil {
		return classInventory{}, err
	}
	if err := c.attributes(); err != nil {
		return classInventory{}, err
	}
	if c.r.Len() != 0 {
		return classInventory{}, fmt.Errorf("trailing class data")
	}
	sortMembers(fields)
	sortMembers(methods)
	return classInventory{Name: name, Fields: fields, Methods: methods}, nil
}

func (c *classReader) members() ([]classMember, error) {
	count, err := c.u2()
	if err != nil {
		return nil, err
	}
	result := make([]classMember, 0, count)
	for i := 0; i < int(count); i++ {
		if err := c.skip(2); err != nil {
			return nil, err
		}
		nameIndex, err := c.u2()
		if err != nil {
			return nil, err
		}
		descIndex, err := c.u2()
		if err != nil {
			return nil, err
		}
		name, err := c.utf(nameIndex)
		if err != nil {
			return nil, err
		}
		desc, err := c.utf(descIndex)
		if err != nil {
			return nil, err
		}
		result = append(result, classMember{Name: name, Descriptor: desc})
		if err := c.attributes(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *classReader) attributes() error {
	count, err := c.u2()
	if err != nil {
		return err
	}
	for i := 0; i < int(count); i++ {
		if err := c.skip(2); err != nil {
			return err
		}
		length, err := c.u4()
		if err != nil {
			return err
		}
		if int64(length) > maxClassfileBytes {
			return fmt.Errorf("attribute exceeds size limit")
		}
		if err := c.skip(int64(length)); err != nil {
			return err
		}
	}
	return nil
}

func (c *classReader) className(index uint16) (string, error) {
	if index == 0 || int(index) >= len(c.cls) || c.cls[index] == 0 {
		return "", fmt.Errorf("invalid class index %d", index)
	}
	return c.utf(c.cls[index])
}

func (c *classReader) utf(index uint16) (string, error) {
	if index == 0 || int(index) >= len(c.utf8) || c.utf8[index] == "" {
		return "", fmt.Errorf("invalid UTF8 index %d", index)
	}
	return c.utf8[index], nil
}

func (c *classReader) u1() (uint8, error) {
	value, err := c.r.ReadByte()
	return value, err
}

func (c *classReader) u2() (uint16, error) {
	var value uint16
	err := binary.Read(c.r, binary.BigEndian, &value)
	return value, err
}

func (c *classReader) u4() (uint32, error) {
	var value uint32
	err := binary.Read(c.r, binary.BigEndian, &value)
	return value, err
}

func (c *classReader) skip(n int64) error {
	if n < 0 || n > int64(c.r.Len()) {
		return io.ErrUnexpectedEOF
	}
	_, err := c.r.Seek(n, io.SeekCurrent)
	return err
}

func sortMembers(values []classMember) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Name == values[j].Name {
			return values[i].Descriptor < values[j].Descriptor
		}
		return values[i].Name < values[j].Name
	})
}
