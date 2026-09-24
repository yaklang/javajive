package core

import "fmt"

// ValidateReducible rejects irreducible normal-flow regions, including nested
// multi-entry cycles inside a unique outer header and handler-only normal flow.
// Exception edges are never back-edges and never SCC entries. Each analysis
// domain is the subgraph reachable via normal edges from one root: method entry
// (Nodes[0]) or a distinct EdgeException.To.
//
// Production uses cached dominance (T26) plus the back-edge / remaining-DAG
// characterization. Maximal-SCC single-entry is not sufficient.
func (g *SemanticCFG) ValidateReducible() error {
	if len(g.Nodes) == 0 {
		return nil
	}
	for _, root := range g.normalFlowRoots() {
		rootIdx := g.indexOfNode(root)
		if rootIdx < 0 {
			continue
		}
		analysis := g.GetOrCompute(AnalysisDominators, GraphAnalysisDomain{
			Roots:            []int{rootIdx},
			IncludeException: false,
		})
		if err := g.validateDomainReducible(root, rootIdx, analysis); err != nil {
			return err
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
