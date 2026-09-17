package coordinator

import (
	"fmt"
)

type ValidationClassification string

const (
	ValidAdvisory     ValidationClassification = "valid_advisory"
	UnknownCapability ValidationClassification = "unknown_capability"
	InvalidTarget     ValidationClassification = "invalid_target"
	InvalidArguments  ValidationClassification = "invalid_arguments"
	PolicyRejected    ValidationClassification = "policy_rejected"
	Duplicate         ValidationClassification = "duplicate"
)

func (c *Coordinator) ValidateRecommendation(capability, target string, arguments interface{}) (ValidationClassification, string) {
	// 1. Target check
	c.Store.Mu.RLock()
	knownTarget := false
	if target == "all" {
		knownTarget = true
	} else {
		for _, w := range c.Store.Workers {
			if w.ID == target || w.NodeName == target || hasLabel(w.Labels, target) {
				knownTarget = true
				break
			}
		}
	}
	c.Store.Mu.RUnlock()
	if !knownTarget {
		return InvalidTarget, fmt.Sprintf("target %q is not a known worker or label", target)
	}

	// 2. Capability check (For now, just check if it's a known agent or execution profile)
	// ForgeGrid doesn't have a rigid action registry other than Execution Profiles and standard capabilities
	knownCapabilities := map[string]bool{
		"agent:codex": true, "agent:antigravity": true, "powershell": true, "bash": true, "python": true,
		"docker": true, "git": true, "inventory": true,
	}
	if !knownCapabilities[capability] {
		return UnknownCapability, fmt.Sprintf("capability %q is not registered in ForgeGrid", capability)
	}

	// 3. Arguments check (Must be a map[string]interface{} representing Parameters)
	if arguments != nil {
		if _, ok := arguments.(map[string]interface{}); !ok {
			return InvalidArguments, "arguments must be a structured key-value object, not a string or array"
		}
	}

	return ValidAdvisory, "valid for advisory execution"
}

func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}
