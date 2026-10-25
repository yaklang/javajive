package javaclassparser

// A flat source unit needs widened access only for a member that a peer might
// reference. Build one conservative use index from the complete reciprocal JVM
// nest. Lexical ownership, dollar spelling and the source target version grant
// no such evidence. Unknown/missing peers retain the existing fallback.
type nestPrivateUseKey struct {
	name, descriptor string // original modified-UTF8 identities, not display text
}

func (c *ClassObjectDumper) nestDemotePrivateMember(member *MemberInfo) bool {
	if member == nil || member.AccessFlags&2 == 0 || !c.nestDemotePrivate() {
		return false
	}
	if !nativeProofWork(c.Work, 1) {
		return true
	}
	if !c.nestPrivateUsesKnown {
		c.nestPrivateUsesKnown = true
		c.nestPrivateUses = c.originalNestPrivateUseIndex()
	}
	name, named := sourceParameterUTF8Key(c.obj, member.NameIndex)
	descriptor, typed := sourceParameterUTF8Key(c.obj, member.DescriptorIndex)
	return !named || !typed || c.nestPrivateUses == nil || c.nestPrivateUses[nestPrivateUseKey{name, descriptor}]
}

func (c *ClassObjectDumper) originalNestPrivateUseIndex() map[nestPrivateUseKey]bool {
	if c == nil || c.obj == nil || !nativeModernSourceNestVersion(c.obj, c.Work) {
		return nil
	}
	resolve := c.nativeAnnotationDeclarationResolver()
	host := c.obj
	if hasOriginalNestAttribute(host, "NestHost") {
		names, valid := originalNestAttribute(host, "NestHost")
		if !valid || len(names) != 1 {
			return nil
		}
		var known bool
		host, known = resolve(names[0])
		if !known || host == nil || host.GetClassName() != names[0] {
			return nil
		}
	}
	if !nativeModernSourceNestVersion(host, c.Work) || hasOriginalNestAttribute(host, "NestHost") {
		return nil
	}
	names, valid := originalNestAttribute(host, "NestMembers")
	if !valid || len(names) == 0 || len(names) > 128 || c.Work.CheckAlloc(int64(len(names)+1)*512) != nil {
		return nil
	}
	objects := map[string]*ClassObject{host.GetClassName(): host}
	for _, name := range names {
		if objects[name] != nil || !nativeProofWork(c.Work, int64(len(name))+1) {
			return nil
		}
		peer := c.obj
		if name != c.obj.GetClassName() {
			var known bool
			peer, known = resolve(name)
			if !known {
				return nil
			}
		}
		if peer == nil || peer.GetClassName() != name || nestPackage(name) != nestPackage(host.GetClassName()) || !nativeModernSourceNestVersion(peer, c.Work) || hasOriginalNestAttribute(peer, "NestMembers") {
			return nil
		}
		parents, known := originalNestAttribute(peer, "NestHost")
		if !known || len(parents) != 1 || parents[0] != host.GetClassName() {
			return nil
		}
		objects[name] = peer
	}
	if objects[c.obj.GetClassName()] != c.obj {
		return nil
	}
	uses := map[nestPrivateUseKey]bool{}
	constants, textBytes := 0, 0
	for _, peer := range objects {
		if peer == c.obj {
			continue
		}
		if len(peer.ConstantPool) > 65535-constants {
			return nil
		}
		constants += len(peer.ConstantPool)
		if c.Work.CheckAlloc(int64(constants)*128+int64(textBytes)*2+int64(len(objects))*512) != nil {
			return nil
		}
		for _, constant := range peer.ConstantPool {
			if !nativeProofWork(c.Work, 1) {
				return nil
			}
			ref := nativeConstantMember(constant)
			if ref == nil {
				continue
			}
			// Overapproximate symbolic-owner resolution: a private field may
			// be reached through a subclass owner. A same-name/descriptor use
			// anywhere in a peer prevents narrowing without a hierarchy proof.
			// Unused CP entries and method-handle targets also remain uses.
			if _, known := sourceParameterClassKey(peer, ref.ClassIndex); !known || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(peer.ConstantPool) {
				return nil
			}
			nt, known := peer.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !known || nt == nil {
				return nil
			}
			name, named := sourceParameterUTF8Key(peer, nt.NameIndex)
			descriptor, typed := sourceParameterUTF8Key(peer, nt.DescriptorIndex)
			if !named || !typed || len(name)+len(descriptor) > (8<<20)-textBytes || !nativeProofWork(c.Work, int64(len(name)+len(descriptor))+1) {
				return nil
			}
			textBytes += len(name) + len(descriptor)
			if c.Work.CheckAlloc(int64(constants)*128+int64(textBytes)*2+int64(len(objects))*512) != nil {
				return nil
			}
			uses[nestPrivateUseKey{name, descriptor}] = true
		}
	}
	return uses
}
