package iterrun

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"time"
)

// ReportInput is everything a plan report section reads. The page and the
// /api/report route gather it once; a section builder folds it and never
// reads the filesystem, the hook log or the record store itself, so every
// section is testable from a constructed input and a fixed clock.
type ReportInput struct {
	Plan PlanSummary
	// Record holds the plan execution's runs: observed runs from the run
	// record, or the read-only legacy view when none exist.
	Record RecordReport
	// RecordErr is set when the record could not be read at all; sections
	// report their data as unavailable rather than empty.
	RecordErr error
	// Rows are the timeline lanes: plan-file detail, merged with hook
	// history when HistoryLoaded.
	Rows []Row
	// Tokens is transcript-derived usage. Nil until history loads.
	Tokens        *PlanTokens
	HistoryLoaded bool
	// Now is the report clock. Tests inject it; the page uses time.Now.
	Now time.Time
}

// PlanReport is the plan report model: one section per tracked dimension.
// Each section type, its builder and its HTML writer live in their own
// report_<section>.go file.
type PlanReport struct {
	Coverage  CoverageSection  `json:"coverage"`
	Progress  ProgressSection  `json:"progress"`
	Evidence  EvidenceSection  `json:"evidence"`
	Ownership OwnershipSection `json:"ownership"`
	Time      TimeSection      `json:"time"`
	Rework    ReworkSection    `json:"rework"`
	Usage     UsageSection     `json:"usage"`
	Delivery  DeliverySection  `json:"delivery"`
}

// BuildPlanReport folds an input into every section.
func BuildPlanReport(in ReportInput) PlanReport {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	return PlanReport{
		Coverage:  buildCoverageSection(in),
		Progress:  buildProgressSection(in),
		Evidence:  buildEvidenceSection(in),
		Ownership: buildOwnershipSection(in),
		Time:      buildTimeSection(in),
		Rework:    buildReworkSection(in),
		Usage:     buildUsageSection(in),
		Delivery:  buildDeliverySection(in),
	}
}

// RenderPlanReport writes every section's HTML in page order.
func RenderPlanReport(rep PlanReport) string {
	var b strings.Builder
	b.WriteString(`<div id="plan-report">`)
	writeProgressSection(&b, rep.Progress)
	writeEvidenceSection(&b, rep.Evidence)
	writeCoverageSection(&b, rep.Coverage)
	writeUsageSection(&b, rep.Usage)
	writeTimeSection(&b, rep.Time)
	writeOwnershipSection(&b, rep.Ownership)
	writeReworkSection(&b, rep.Rework)
	writeDeliverySection(&b, rep.Delivery)
	b.WriteString(`</div>`)
	return b.String()
}

// errReportNotFound reports a project/plan/archive identity with no plan file.
var errReportNotFound = errors.New("plan not found")

// LoadReportInput gathers one plan execution's report input. With
// archiveFile it reads that archived execution; otherwise the live plan,
// following its newest archive when the live file is gone. With history it
// also reads the hook log and transcripts. It never writes anything.
func LoadReportInput(ctx context.Context, project, plan, archiveFile string, history bool, now time.Time) (ReportInput, error) {
	in := ReportInput{Now: now}
	var err error
	if archiveFile != "" {
		if project, err = safeDetailIdentity(project, archiveFile, "archive"); err != nil {
			return in, errors.Join(errReportNotFound, err)
		}
		if in.Plan, err = GetArchivedPlanSummary(project, archiveFile); err != nil {
			return in, errors.Join(errReportNotFound, err)
		}
		if in.Rows, err = BuildRowsFromArchive(archiveFile, in.Plan.Name, project, nil); err != nil {
			return in, err
		}
	} else {
		canonical, ierr := safeDetailIdentity(project, plan, "plans")
		if ierr != nil && (canonical == "" || !errors.Is(ierr, fs.ErrNotExist)) {
			return in, errors.Join(errReportNotFound, ierr)
		}
		project = canonical
		if _, perr := GetPlanSummary(project, plan); perr != nil && errors.Is(perr, fs.ErrNotExist) {
			if archived, aerr := ListArchivedPlans(project); aerr == nil {
				newest := ""
				for _, a := range archived {
					if a.Name == plan && a.ArchiveFile > newest {
						newest = a.ArchiveFile
					}
				}
				if newest != "" {
					return LoadReportInput(ctx, project, plan, newest, history, now)
				}
			}
		}
		if in.Plan, err = GetPlanSummary(project, plan); err != nil {
			return in, errors.Join(errReportNotFound, err)
		}
		if in.Rows, err = BuildRowsFromFilesystem(plan, project, nil); err != nil {
			return in, err
		}
	}
	in.Record, in.RecordErr = ReadRecordReport(project, in.Plan.Name, in.Plan.ArchiveFile)
	if history {
		events, herr := readEventsContext(ctx)
		if herr != nil {
			return in, herr
		}
		labels, _ := ReadLabels()
		started, _ := in.Plan.EffectiveStart()
		in.Rows = MergeRows(in.Rows, BuildRowsFromHookEvents(events, labels, in.Plan.Name, project, started))
		pt := PlanTokenUsage(in.Plan, events, labels)
		in.Tokens = &pt
		in.HistoryLoaded = true
	}
	return in, nil
}
