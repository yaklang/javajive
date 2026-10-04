package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// ClassHierarchyFromBytes extracts super_class/interfaces and a content hash
// from class bytes. Parse does not load or initialize the class.
func ClassHierarchyFromBytes(data []byte) (types.ClassHierarchyIdentity, error) {
	obj, err := Parse(data)
	if err != nil {
		return types.ClassHierarchyIdentity{}, err
	}
	return types.ClassHierarchyIdentity{
		BinaryName:  obj.GetClassName(),
		SuperClass:  obj.GetSupperClassName(),
		Interfaces:  obj.GetInterfacesName(),
		ContentHash: types.ContentHash(data),
	}, nil
}

// SuperTypeProviderFromClassBytes builds an isolated MetadataHierarchyProvider
// from in-memory class files keyed by JVM internal name.
func SuperTypeProviderFromClassBytes(classes map[string][]byte, targetVersion, optionsKey string, cancel <-chan struct{}) (*types.MetadataHierarchyProvider, types.SuperTypeProvider) {
	blobs := make([][]byte, 0, len(classes))
	normalized := map[string][]byte{}
	for name, data := range classes {
		key := strings.ReplaceAll(name, ".", "/")
		normalized[key] = data
		blobs = append(blobs, data)
	}
	resolver := func(internal string) (types.ClassHierarchyIdentity, bool) {
		internal = strings.ReplaceAll(internal, ".", "/")
		data, ok := normalized[internal]
		if !ok {
			return types.ClassHierarchyIdentity{}, false
		}
		ident, err := ClassHierarchyFromBytes(data)
		if err != nil {
			return types.ClassHierarchyIdentity{}, false
		}
		return ident, true
	}
	p := types.NewMetadataHierarchyProvider(resolver, types.ClasspathDigest(blobs...), targetVersion, optionsKey, cancel)
	return p, p.AsSuperTypeProvider()
}
