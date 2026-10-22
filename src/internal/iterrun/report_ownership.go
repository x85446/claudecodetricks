package iterrun

import "strings"

// OwnershipSection is the plan report's step-9 section: CLI and Conductor ownership, handoffs, blockers and admission decisions, with their delays.
type OwnershipSection struct{}

func buildOwnershipSection(in ReportInput) OwnershipSection { return OwnershipSection{} }

func writeOwnershipSection(b *strings.Builder, s OwnershipSection) {}
