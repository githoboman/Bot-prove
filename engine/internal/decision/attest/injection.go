package attest

import (
	"regexp"
	"strings"
)

// injectionPatterns is a small, deliberately conservative set of markers
// used by the safety facet to detect obvious prompt-injection or
// exfiltration attempts. The list is not intended to be exhaustive — it
// exists so that the fixture reproducer can demonstrate a REJECT path
// on a well-known malicious input. In production the safety facet is
// expected to run a much richer policy engine behind the same interface.
var injectionPatterns = []*regexp.Regexp{
	// Direct override instructions. The `(all\s+)?(previous\s+|prior\s+)?`
	// prefix accepts any combination of "all", "previous", "prior" between
	// the verb and the target noun, in either order.
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous\s+|prior\s+)?(instructions|rules|policies|prompts?)`),
	regexp.MustCompile(`(?i)disregard\s+(the\s+|all\s+|any\s+)?(system\s+|previous\s+|prior\s+)?(message|prompt|instructions)`),
	// Exfiltration markers.
	regexp.MustCompile(`(?i)print (your |the )?(system prompt|api[_ ]?key|secret|private key)`),
	regexp.MustCompile(`(?i)reveal (your |the )?(system prompt|hidden|secret|token)`),
	// Role escalation.
	regexp.MustCompile(`(?i)act as (the )?(root|admin|system|owner)`),
	regexp.MustCompile(`(?i)switch to developer mode`),
}

// SafetyFacet inspects a decision payload for known prompt-injection or
// exfiltration markers and returns a FacetVerdict for FacetSafety. It is
// deliberately deterministic: the same payload always produces the same
// verdict, so the on-chain commit hash is reproducible.
func SafetyFacet(payload []byte) FacetVerdict {
	text := string(payload)
	for _, re := range injectionPatterns {
		if loc := re.FindStringIndex(text); loc != nil {
			marker := strings.TrimSpace(text[loc[0]:loc[1]])
			return FacetVerdict{
				Kind:       FacetSafety,
				Verdict:    VerdictReject,
				Confidence: 1.0,
				Reason:     "prompt-injection marker detected: " + marker,
			}
		}
	}
	return FacetVerdict{
		Kind:       FacetSafety,
		Verdict:    VerdictApprove,
		Confidence: 1.0,
		Reason:     "no known injection markers",
	}
}
