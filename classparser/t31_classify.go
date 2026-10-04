package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func unsupportedBootstraps(obj *ClassObject) []string {
	var unknown []string
	for _, id := range classBootstrapIdentities(obj) {
		if _, ok := core.LookupBuiltin(id); !ok {
			unknown = append(unknown, id.Format())
		}
	}
	return unknown
}

func isBudgetErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "resource_limit") ||
		strings.Contains(msg, "analysis_budget_exceeded") ||
		strings.Contains(msg, "budget")
}

func noteUnsupportedBootstraps(result *DecompileResult, obj *ClassObject) {
	if result == nil || obj == nil {
		return
	}
	if result.Status == "invalid_input" || result.Status == "resource_limit" || result.Status == "canceled" {
		return
	}
	unknown := unsupportedBootstraps(obj)
	if len(unknown) == 0 {
		return
	}
	for _, d := range result.Diagnostics {
		if d.Code == "unsupported_bootstrap" {
			if result.Status == "complete" {
				result.Status = "unsupported"
			}
			return
		}
	}
	result.Diagnostics = append(result.Diagnostics, DecompileDiagnostic{
		Code:    "unsupported_bootstrap",
		Message: "unsupported_bootstrap: " + strings.Join(unknown, ","),
	})
	if result.Status == "complete" {
		result.Status = "unsupported"
	}
}
