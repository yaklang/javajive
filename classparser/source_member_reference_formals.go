package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

// Reference metadata distinguishes two flat declarations: a member without
// formals redeclares its nearest owner's formals, while a member with its own
// formals keeps that declaration and projects hidden outer binders away. This
// second case must not change expression inference's enclosing-formal API.
func (c *ClassObjectDumper) buildSiblingReferenceSourceFormals(returned func(string, []byte) bool) func(string) ([]string, bool) {
	inherited := c.buildSiblingSourceClassFormals()
	if inherited == nil {
		return nil
	}
	return func(internal string) ([]string, bool) {
		if names, known := inherited(internal); known {
			return names, true
		}
		if c.nativeMemberLookup != nil && c.nativeMemberLookup(internal) != nil || c.nativeMemberRoot != nil && c.nativeMemberRoot.allocationClass(internal) != nil {
			return nil, false
		}
		if c.FuncCtx != nil && c.FuncCtx.DeclarationSourceName != nil {
			if _, known := c.FuncCtx.DeclarationSourceName(internal); known {
				return nil, false
			}
		}
		raw, known := c.foldSiblingResolver(internal)
		if !known || !nativeProofWork(c.Work, int64(len(raw))+1) || returned != nil && !returned(internal, raw) {
			return nil, false
		}
		obj, err := c.parseResolved(raw)
		if err != nil || obj.GetClassName() != internal || obj.MajorVersion < 49 || obj.MajorVersion > 52 || obj.MinorVersion != 0 {
			return nil, false
		}
		_, _, flags, member := originalMemberOwner(obj)
		sig, valid := nativeMethodLocalOriginalSignature(obj, obj.Attributes, c.Work)
		names := types.ClassFormalTypeParamNames(sig)
		if !member || flags&StaticFlag != 0 || !valid || len(names) == 0 || len(names) > 128 || !originalMemberScopeCapturesClosed(obj, c.Work) {
			return nil, false
		}
		d := NewClassObjectDumper(obj)
		d.Work = c.Work
		scopes, local, valid := d.flattenedMethodLexicalScopesResolved(c.nativeAnnotationDeclarationResolver())
		if !valid || local || len(scopes) < 2 {
			return nil, false
		}
		projected, valid := types.ProjectHiddenLexicalClassSignature(scopes)
		if !valid || !memberOwnScopeProjectedClassSignature(obj, projected, scopes, c.Work) {
			return nil, false
		}
		return names, true
	}
}
