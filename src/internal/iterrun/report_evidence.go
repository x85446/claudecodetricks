package iterrun

import "strings"

// EvidenceSection is the plan report's step-8 section: Observed validation evidence per requirement and attempt, with freshness against covered code.
type EvidenceSection struct{}

func buildEvidenceSection(in ReportInput) EvidenceSection { return EvidenceSection{} }

func writeEvidenceSection(b *strings.Builder, s EvidenceSection) {}
