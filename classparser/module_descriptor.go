package javaclassparser

import (
	"fmt"
	"strings"
)

// JPMS module flags (JVMS 4.7.25).
const (
	moduleFlagOpen       = 0x0020
	moduleRequiresTrans  = 0x0020
	moduleRequiresStatic = 0x0040
	moduleFlagSynthetic  = 0x1000
	moduleFlagMandated   = 0x8000
)

type moduleRequireDirective struct {
	name       string
	transitive bool
	static     bool
}

type moduleExportDirective struct {
	name string // dotted package name
	to   []string
}

type moduleProvideDirective struct {
	service string
	with    []string
}

type moduleDescriptor struct {
	name     string
	open     bool
	requires []moduleRequireDirective
	exports  []moduleExportDirective
	opens    []moduleExportDirective
	uses     []string
	provides []moduleProvideDirective
}

type moduleBytes struct {
	data []byte
	off  int
	ok   bool
}

func (r *moduleBytes) u2() uint16 {
	if !r.ok || r.off+2 > len(r.data) {
		r.ok = false
		return 0
	}
	value := uint16(r.data[r.off])<<8 | uint16(r.data[r.off+1])
	r.off += 2
	return value
}

// u2s reads count u2 values. The count is bounded by the remaining bytes so a
// malformed attribute cannot request a huge slice.
func (r *moduleBytes) u2s(count int) []uint16 {
	if !r.ok || count < 0 || count > (len(r.data)-r.off)/2 {
		r.ok = false
		return nil
	}
	values := make([]uint16, count)
	for i := range values {
		values[i] = r.u2()
	}
	return values
}

func (r *moduleBytes) remaining() bool { return r.ok && r.off == len(r.data) }

func (c *ClassObject) moduleName(index uint16) (string, bool) {
	if c == nil {
		return "", false
	}
	name, err := c.getUtf8(index)
	if err != nil || name == "" {
		return "", false
	}
	return name, true
}

func (c *ClassObject) modulePackageName(index uint16) (string, bool) {
	name, ok := c.moduleName(index)
	if !ok {
		return "", false
	}
	return strings.ReplaceAll(name, "/", "."), true
}

// parseModuleDescriptor decodes the raw Module attribute (JVMS 4.7.25). Source
// cannot express ACC_SYNTHETIC directives, so those are omitted; the mandated
// java.base requirement javac always records is also implicit in source.
func parseModuleDescriptor(obj *ClassObject, raw []byte) (moduleDescriptor, bool) {
	var out moduleDescriptor
	r := &moduleBytes{data: raw, ok: true}
	name, ok := obj.moduleName(r.u2())
	flags := r.u2()
	_ = r.u2() // module_version_index: no source spelling
	if !r.ok || !ok {
		return out, false
	}
	out.name, out.open = name, flags&moduleFlagOpen != 0

	requiresCount := int(r.u2())
	requires := r.u2s(requiresCount * 3)
	for i := 0; i+2 < len(requires); i += 3 {
		moduleName, known := obj.moduleName(requires[i])
		reqFlags := requires[i+1]
		_ = requires[i+2] // requires_version_index
		if !known || reqFlags&moduleFlagSynthetic != 0 {
			continue
		}
		if moduleName == "java.base" && reqFlags&moduleFlagMandated != 0 {
			continue
		}
		out.requires = append(out.requires, moduleRequireDirective{
			name:       moduleName,
			transitive: reqFlags&moduleRequiresTrans != 0,
			static:     reqFlags&moduleRequiresStatic != 0,
		})
	}

	readExport := func() (moduleExportDirective, bool) {
		pkg, known := obj.modulePackageName(r.u2())
		flags := r.u2()
		targetCount := int(r.u2())
		targets := r.u2s(targetCount)
		if !r.ok || !known || flags&moduleFlagSynthetic != 0 {
			return moduleExportDirective{}, false
		}
		directive := moduleExportDirective{name: pkg}
		for _, index := range targets {
			target, known := obj.moduleName(index)
			if !known {
				return moduleExportDirective{}, false
			}
			directive.to = append(directive.to, target)
		}
		return directive, true
	}

	exportCount := r.u2()
	for i := 0; i < int(exportCount) && r.ok; i++ {
		directive, ok := readExport()
		if !ok {
			return out, false
		}
		out.exports = append(out.exports, directive)
	}

	openCount := r.u2()
	for i := 0; i < int(openCount) && r.ok; i++ {
		directive, ok := readExport()
		if !ok {
			return out, false
		}
		out.opens = append(out.opens, directive)
	}

	usesCount := int(r.u2())
	uses := r.u2s(usesCount)
	for _, index := range uses {
		name, known := obj.modulePackageName(index)
		if !known {
			return out, false
		}
		out.uses = append(out.uses, name)
	}

	providesCount := r.u2()
	for i := 0; i < int(providesCount) && r.ok; i++ {
		service, known := obj.modulePackageName(r.u2())
		withCount := int(r.u2())
		with := r.u2s(withCount)
		if !r.ok || !known {
			return out, false
		}
		directive := moduleProvideDirective{service: service}
		for _, index := range with {
			impl, known := obj.modulePackageName(index)
			if !known {
				return out, false
			}
			directive.with = append(directive.with, impl)
		}
		out.provides = append(out.provides, directive)
	}

	if !r.remaining() || out.name == "" {
		return out, false
	}
	return out, true
}

func (d moduleDescriptor) source() string {
	open := ""
	if d.open {
		open = "open "
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%smodule %s {\n", open, d.name)
	for _, req := range d.requires {
		modifiers := ""
		if req.transitive {
			modifiers += " transitive"
		}
		if req.static {
			modifiers += " static"
		}
		fmt.Fprintf(&b, "    requires%s %s;\n", modifiers, req.name)
	}
	writeExports := func(keyword string, directives []moduleExportDirective) {
		for _, directive := range directives {
			if len(directive.to) == 0 {
				fmt.Fprintf(&b, "    %s %s;\n", keyword, directive.name)
				continue
			}
			fmt.Fprintf(&b, "    %s %s to %s;\n", keyword, directive.name, strings.Join(directive.to, ", "))
		}
	}
	writeExports("exports", d.exports)
	writeExports("opens", d.opens)
	for _, use := range d.uses {
		fmt.Fprintf(&b, "    uses %s;\n", use)
	}
	for _, provide := range d.provides {
		fmt.Fprintf(&b, "    provides %s with %s;\n", provide.service, strings.Join(provide.with, ", "))
	}
	b.WriteString("}\n")
	return b.String()
}

// renderModuleDescriptor reconstructs a source-level module declaration from
// the original Module attribute. Without that attribute (or on malformed
// bytes) the caller keeps the synthetic-descriptor comment.
func (c *ClassObjectDumper) renderModuleDescriptor() (string, bool) {
	if c == nil || c.obj == nil {
		return "", false
	}
	for _, attribute := range c.obj.Attributes {
		unparsed, ok := attribute.(*UnparsedAttribute)
		if !ok || unparsed == nil || unparsed.Name != "Module" {
			continue
		}
		descriptor, ok := parseModuleDescriptor(c.obj, unparsed.Info)
		if !ok {
			return "", false
		}
		source := descriptor.source()
		if err := c.ensureOutput(int64(len(source))); err != nil {
			return "", false
		}
		return source, true
	}
	return "", false
}
