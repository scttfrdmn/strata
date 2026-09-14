package strata

// NewClientFromRegistry exposes the internal registry-injection seam
// (newClientFromRegistry) to this package's external test package. It is defined
// in a _test.go file, so it is compiled only into the test binary and is never
// part of the public API — which is the point of #76: a Client built from a
// registry.Client is usable in-module (tests), but registry.Client is internal,
// so no external module could ever call it.
var NewClientFromRegistry = newClientFromRegistry
