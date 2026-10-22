package iterrun

import "strings"

// CoverageSection is the plan report's step-6 section: Tracking completeness — which telemetry sources were captured, missing or partial for this execution.
type CoverageSection struct{}

func buildCoverageSection(in ReportInput) CoverageSection { return CoverageSection{} }

func writeCoverageSection(b *strings.Builder, s CoverageSection) {}
