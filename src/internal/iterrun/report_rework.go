package iterrun

import "strings"

// ReworkSection is the plan report's step-11 section: Failed attempts, retries, restarts, reopened requirements and recovery totals.
type ReworkSection struct{}

func buildReworkSection(in ReportInput) ReworkSection { return ReworkSection{} }

func writeReworkSection(b *strings.Builder, s ReworkSection) {}
