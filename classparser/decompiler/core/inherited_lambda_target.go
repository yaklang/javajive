package core

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Compose a declaration's variables back to the actual functional owner.
// Physical edges, declaration identity and every inherited path must agree;
// taking the first matching ancestor would accept conflicting diamonds.
func inheritedDeclaredSamSignature(ctx *class_context.ClassContext, declarations types.ClassSigProvider, root, method, descriptor string) string {
	if ctx == nil || ctx.InvocationMetadata == nil || declarations == nil || root == "" || method == "" || descriptor == "" {
		return ""
	}
	charge := func(n int) bool {
		return n >= 0 && (ctx.Work == nil || ctx.Work.Charge(workbudget.CounterGraphScans, int64(n)) == nil)
	}
	type declaration struct {
		metadata callbinding.Class
		methods  map[string]string
	}
	graph := map[string]declaration{}
	active := map[string]bool{}
	key := class_context.MethodDescKey(method, descriptor)
	var collect func(string, int) bool
	collect = func(owner string, depth int) bool {
		if depth > 32 || owner == "" || active[owner] || !charge(1) {
			return false
		}
		if _, known := graph[owner]; known {
			return true
		}
		if len(graph) >= 64 {
			return false
		}
		meta, known := ctx.InvocationMetadata(owner)
		sig, methods, declared := declarations(owner)
		if !known || !declared || meta.Name != owner || !meta.IsInterface || !meta.MembersComplete || !meta.ParentsComplete || len(meta.Parents) > 64 || len(meta.Methods) > 1024 || len(methods) > 2048 || len(sig) > 8192 || sig != meta.Signature || !charge(len(sig)+len(meta.Methods)+len(methods)) {
			return false
		}
		if ctx.Work != nil && ctx.Work.CheckAlloc(int64(len(sig)+len(methods)*128+len(meta.Methods)*128+len(meta.Parents)*64+256)) != nil {
			return false
		}
		_, references, valid := types.SignatureTypeVariableReferences(sig)
		formals := types.ClassFormalTypeParamNames(sig)
		if !valid || len(formals) > 64 {
			return false
		}
		own := map[string]bool{}
		for _, formal := range formals {
			if own[formal] {
				return false
			}
			own[formal] = true
		}
		for _, reference := range references {
			if !own[reference] {
				return false
			}
		}
		copyMethods := make(map[string]string, len(methods))
		for key, signature := range methods {
			if len(key)+len(signature) > 16384 || !charge(len(key)+len(signature)+1) {
				return false
			}
			copyMethods[key] = signature
		}
		_, selected := methods[key]
		matching := 0
		for _, member := range meta.Methods {
			if member.Name == method && member.Desc == descriptor {
				matching++
			}
		}
		if selected != (matching == 1) || matching > 1 {
			return false
		}
		meta.Parents = append([]string(nil), meta.Parents...)
		meta.Methods = append([]callbinding.Method(nil), meta.Methods...)
		graph[owner] = declaration{meta, copyMethods}
		active[owner] = true
		defer delete(active, owner)
		seen := map[string]bool{}
		for _, parent := range meta.Parents {
			if seen[parent] {
				return false
			}
			seen[parent] = true
			if parent != "java/lang/Object" && !collect(parent, depth+1) {
				return false
			}
		}
		return true
	}
	if !collect(root, 0) {
		return ""
	}
	formals := types.ClassFormalTypeParamNames(graph[root].metadata.Signature)
	if len(formals) == 0 {
		return ""
	}
	args := make([]types.JavaType, len(formals))
	for i, formal := range formals {
		args[i] = types.NewJavaTypeVariable(formal)
	}
	seed := types.NewParameterizedType(strings.ReplaceAll(root, "/", "."), args)
	provider := callbinding.Provider(func(owner string) (callbinding.Class, bool) {
		d, known := graph[owner]
		return d.metadata, known
	})
	var result string
	owners := make([]string, 0, len(graph))
	for owner := range graph {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		d := graph[owner]
		signature, declared := d.methods[key]
		if !declared {
			continue
		}
		matching := 0
		for _, member := range d.metadata.Methods {
			if member.Name == method && member.Desc == descriptor {
				if !member.Public || member.Static || member.Bridge || member.Signature != signature {
					return ""
				}
				matching++
			}
		}
		if matching != 1 || signature == "" {
			return ""
		}
		view, valid := types.ProjectInstantiatedSupertype(ctx, seed, owner, provider)
		projected, known := types.AsParameterizedType(view)
		parentFormals := types.ClassFormalTypeParamNames(d.metadata.Signature)
		if !valid || !known || len(projected.TypeArgs) != len(parentFormals) {
			return ""
		}
		sigma := map[string]string{}
		for i, formal := range parentFormals {
			if name, variable := types.TypeVariableName(projected.TypeArgs[i]); variable {
				sigma[formal] = "T" + name + ";"
			} else if literal, known := projected.TypeArgs[i].RawType().(*types.JavaClass); known && literal != nil && literal.Name != "" {
				sigma[formal] = "L" + strings.ReplaceAll(literal.Name, ".", "/") + ";"
			} else {
				return ""
			}
		}
		body, _, valid := directSamThrows(signature)
		tokens, parsed := directSamTokens(body, true)
		if !valid || !parsed {
			return ""
		}
		var out strings.Builder
		out.WriteByte('(')
		for i, token := range tokens {
			if i == len(tokens)-1 {
				out.WriteByte(')')
			}
			if strings.HasPrefix(token, "T") {
				token = sigma[token[1:len(token)-1]]
				if token == "" {
					return ""
				}
			}
			out.WriteString(token)
		}
		// Concrete checked exceptions do not solve an owner variable. Preserve
		// the full original throws list and substitute only tagged variables.
		for _, token := range strings.Split(signature, "^")[1:] {
			if strings.HasPrefix(token, "T") {
				token = sigma[token[1:len(token)-1]]
				if token == "" {
					return ""
				}
			}
			out.WriteByte('^')
			out.WriteString(token)
		}
		if result != "" && result != out.String() {
			return ""
		}
		result = out.String()
	}
	return result
}
