package javaclassparser

// Ordinary and transactional reads route every original source role through
// the same original metadata, including named method-local declarations. This
// chooses a cache key only: access, layout and body ownership still require the
// complete source family to publish successfully.
func (z *JarFS) nativeMemberOriginalSourceOwner(obj *ClassObject) (string, bool) {
	if z == nil || obj == nil {
		return "", false
	}

	owner, _, _, member := originalMemberOwner(obj)
	if anonymousOwner, _, anonymous := originalAnonymousOwner(obj); anonymous {
		reader := z.nativeMemberReader(obj)
		var known bool
		owner, known = z.nativeAnonymousOutermostOwner(anonymousOwner, reader)
		if !known {
			return "", false
		}
		member = true
	}

	if !member {
		for _, a := range obj.Attributes {
			if raw, ok := a.(*UnparsedAttribute); ok && raw != nil && raw.Name == "EnclosingMethod" && len(raw.Info) == 4 {
				localOwner, known := sourceBridgeClassName(obj, uint16(raw.Info[0])<<8|uint16(raw.Info[1]))
				if !known {
					return "", false
				}
				bytes, found := z.enumSiblingResolver()(localOwner)
				if !found {
					return "", false
				}
				reader := z.nativeMemberReader(obj)
				enclosing, e := reader.parseResolved(bytes)
				if e != nil {
					return "", false
				}
				if _, known := originalMethodLocalOwner(obj, enclosing, reader.Work); known {
					owner = localOwner
					member = true
				}
				break
			}
		}
	}
	if !member {
		owner = obj.GetClassName()
	}
	if member {
		var known bool
		owner, known = z.nativeMemberOutermostNamedOwner(owner, z.nativeMemberReader(obj).Work)
		if !known {
			return "", false
		}
	}
	if !member && !z.nativeMemberRootCandidate(obj) {
		return "", false
	}

	return owner, true
}
