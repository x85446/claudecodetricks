package iterrun

import "strings"

// TimeSection is the plan report's step-10 section: Elapsed time partitioned by cause, without summing concurrent spans into wall time.
type TimeSection struct{}

func buildTimeSection(in ReportInput) TimeSection { return TimeSection{} }

func writeTimeSection(b *strings.Builder, s TimeSection) {}
