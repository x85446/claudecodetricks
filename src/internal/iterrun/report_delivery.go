package iterrun

import "strings"

// DeliverySection is the plan report's step-13 section: Delivery outcomes: commit, PR, review/CI, merge, install/deploy and runtime verification.
type DeliverySection struct{}

func buildDeliverySection(in ReportInput) DeliverySection { return DeliverySection{} }

func writeDeliverySection(b *strings.Builder, s DeliverySection) {}
