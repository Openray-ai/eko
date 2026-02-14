// Package benchmarks contains memory ceiling and stress tests for the Ekō
// tokenization engine. These tests are longer-running than the micro-benchmarks
// in internal/core and are intended to be run separately.
//
// Run memory tests:
//
//	go test -v -run TestMemory -timeout 10m ./benchmarks/
//
// Run ceiling tests:
//
//	go test -v -run TestMemoryCeiling -timeout 30m ./benchmarks/
package benchmarks
