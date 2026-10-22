package iterrun

import "strings"

// ProgressSection is the plan report's step-7 section: Verified progress — plan outcome, implemented tasks, passed validations, blocked requirements and lane lifecycle counts, each in its own unit.
type ProgressSection struct{}

func buildProgressSection(in ReportInput) ProgressSection { return ProgressSection{} }

func writeProgressSection(b *strings.Builder, s ProgressSection) {}
