package resolver

import (
	"fmt"
	"strings"

	"github.com/scttfrdmn/strata/spec"
)

// ResolutionError is a structured error produced by the resolver pipeline.
// It carries actionable context so users know exactly what failed and how to fix it.
type ResolutionError struct {
	Stage     string
	Code      string
	Message   string
	Available []string // e.g. available versions when a layer is not found
}

func (e *ResolutionError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("[stage=%s code=%s] %s", e.Stage, e.Code, e.Message)
	}
	return fmt.Sprintf("[stage=%s code=%s] %s\n  Available: %s",
		e.Stage, e.Code, e.Message, strings.Join(e.Available, ", "))
}

func errLayerNotFound(name, version string, available []string) *ResolutionError {
	ref := name
	if version != "" {
		ref += "@" + version
	}
	var msg string
	if len(available) > 0 {
		msg = fmt.Sprintf("no layer found for %q\n  Available versions: %s\n  Run: strata search %s",
			ref, strings.Join(available, ", "), name)
	} else {
		msg = fmt.Sprintf("no layer found for %q\n  Run: strata search %s", ref, name)
	}
	return &ResolutionError{
		Stage:     "stage3",
		Code:      "LAYER_NOT_FOUND",
		Message:   msg,
		Available: available,
	}
}

func errFormationNotFound(nameVersion string) *ResolutionError {
	return &ResolutionError{
		Stage: "stage2",
		Code:  "FORMATION_NOT_FOUND",
		Message: fmt.Sprintf(
			"formation %q not found in registry\n  Run: strata search --formations %s",
			nameVersion, nameVersion),
	}
}

func errUnsatisfiedRequirement(layerName string, req spec.Requirement) *ResolutionError {
	hint := req.Name
	if req.MinVersion != "" {
		hint += "@" + req.MinVersion
	}
	return &ResolutionError{
		Stage: "stage4",
		Code:  "UNSATISFIED_REQUIREMENT",
		Message: fmt.Sprintf(
			"unsatisfied requirements for %q\n  Requires: %s\n  Not provided by base or resolved layers\n  Fix: add %q to your software list",
			layerName, req.String(), hint),
	}
}

func errConflict(layerA, layerB, filePath string) *ResolutionError {
	return &ResolutionError{
		Stage: "stage5",
		Code:  "FILE_CONFLICT",
		Message: fmt.Sprintf(
			"conflict detected\n  Both %q and %q provide %q with different content\n  Use only one of these layers",
			layerA, layerB, filePath),
	}
}

func errCapabilityConflict(layerA, layerB string, capability spec.Capability) *ResolutionError {
	return &ResolutionError{
		Stage: "stage5",
		Code:  "CAPABILITY_CONFLICT",
		Message: fmt.Sprintf(
			"conflict detected\n  Both %q and %q provide %q\n  Use only one implementation",
			layerA, layerB, capability.Name),
	}
}

func errBundleMissing(layerID string) *ResolutionError {
	return &ResolutionError{
		Stage:   "stage7",
		Code:    "BUNDLE_MISSING",
		Message: fmt.Sprintf("layer %q has no Sigstore bundle — unsigned layers cannot be used", layerID),
	}
}

func errRekorEntryMissing(layerID string) *ResolutionError {
	return &ResolutionError{
		Stage:   "stage7",
		Code:    "REKOR_ENTRY_MISSING",
		Message: fmt.Sprintf("layer %q has no Rekor transparency log entry — logging is mandatory", layerID),
	}
}
