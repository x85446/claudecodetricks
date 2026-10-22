package iterrun

import "strings"

// UsageSection is the plan report's step-12 section: Plan-scoped usage by scope, model, provider, agent, tool and wake-up.
type UsageSection struct{}

func buildUsageSection(in ReportInput) UsageSection { return UsageSection{} }

func writeUsageSection(b *strings.Builder, s UsageSection) {}
