package core

import "fmt"

// ValidateReducible rejects irreducible normal-flow regions, including nested
// multi-entry cycles inside a unique outer header and handler-only normal flow.
// Each normal-flow domain starts at method entry or a distinct handler. A
// handler inside a loop can rejoin more than one enclosing loop level. Its
// artificial root must not erase headers already entered before the exception.
// Full-method dominance can prove those inherited NORMAL backedges; exception
// edges themselves are never classified as backedges or added to the DAG test.
//
// Production uses cached dominance (T26) plus the back-edge / remaining-DAG
// characterization. Maximal-SCC single-entry is not sufficient.
func (g *SemanticCFG) ValidateReducible() error {
	if len(g.Nodes) == 0 {
		return nil
	}
	var methodContext *GraphAnalysis
	for _, root := range g.normalFlowRoots() {
		rootIdx := g.indexOfNode(root)
		if rootIdx < 0 {
			continue
		}
		analysis := g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{
			Roots:            []int{rootIdx},
			IncludeException: false,
		})
		if err := g.validateDomainReducible(root, analysis, nil); err != nil {
			if root == g.Nodes[0] {
				return err
			}
			// Most methods need only the normal-domain check. Compute this
			// additional proof lazily for a handler whose artificial entry
			// makes an existing loop appear to have multiple entries.
			if methodContext == nil {
				methodContext = g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{
					Roots: []int{0}, IncludeException: true,
				})
			}
			if !methodContext.Dominates(0, rootIdx) {
				return err
			}
			if err = g.validateDomainReducible(root, analysis, methodContext); err != nil {
				return err
			}
		}
	}
	if err := g.ValidateExceptionRegions(); err != nil {
		return err
	}
	return nil
}

// irreducibleDiagnostic is kept for stable prefix matching in API tests.
func irreducibleDiagnostic(regionPC, rootPC uint16, extra string) error {
	if extra == "" {
		return fmt.Errorf("unsupported_irreducible_control_flow: region near PC %d (root PC %d)", regionPC, rootPC)
	}
	return fmt.Errorf("unsupported_irreducible_control_flow: region near PC %d (root PC %d) %s", regionPC, rootPC, extra)
}
