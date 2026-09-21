// Package performance_baseline is the T32 staged performance harness.
//
// It measures the existing JavaJive pipeline without changing production
// decompiler algorithms. Stages that cannot be split through exported APIs
// are labeled as combined, opcode-only, or reruns-decompile. T24 SemanticCFG,
// T25 sparse reaching, and T26 GetOrCompute counters are production-backed;
// pending_t24_t25_t26 is false.
package performance_baseline
