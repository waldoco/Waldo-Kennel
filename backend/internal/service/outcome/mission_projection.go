package outcome

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
)

const MissionProjectionVersion = 1

type MissionAttention struct {
	Kind, Summary, QuestionID, Generation string
}
type MissionSession struct {
	RefID                            string
	Seq                              int64
	SessionID, Harness, Mode, Status string
	BoundAt                          time.Time
}
type MissionExecutionBinding struct {
	Provider, ModelSelection, Model string
}
type MissionChangeSummary struct {
	Additions, Deletions, FilesChanged                 int64
	SourceAttemptID, ArtifactVersion, MeasurementState string
}
type MissionLink struct{ Kind, ID, Label, State string }
type MissionAttempt struct {
	ID                   string
	Number               int64
	Status               string
	CreatedAt, UpdatedAt time.Time
	Session              *MissionSession
}
type MissionNode struct {
	WorkUnitID, PlanRevisionID, Title string
	Role                              string
	Inputs                            []domain.WorkUnitInput
	DependsOn                         []domain.WorkUnitID
	ScheduleState                     string
	BlockingDependencies              []domain.WorkUnitID
	BlockedReason, BlockedDetail      string
	CriterionIDs                      []domain.CriterionID
	CriterionReady                    map[domain.CriterionID]bool
	CurrentAttempt                    *MissionAttempt
	ExecutionBinding                  *MissionExecutionBinding
	ChangeSummary                     *MissionChangeSummary
	Links                             []MissionLink
	Attention                         *MissionAttention
	NextAction                        string
	Responsibility                    string
	UpdatedAt                         time.Time
	Generation                        int64
}
type MissionEdge struct{ From, To domain.WorkUnitID }
type MissionProjection struct {
	Version                       int
	MissionLabel                  string
	OutcomeID                     domain.OutcomeID
	MissionID                     domain.ResponsibilitySpaceID
	ContractRevisionNumber        int64
	PlanRevisionID                domain.PlanRevisionID
	PlanRevisionNumber            int64
	TopologyFingerprint           string
	TopologyGeneration            int64
	Generation                    int64
	UpdatedAt                     time.Time
	Nodes                         []MissionNode
	Edges                         []MissionEdge
	NextRunnableID, CustodyHeldBy domain.WorkUnitID
	NoRunnableReason              string
}

// GetMissionProjection composes renderer-safe graph truth from the same
// approved Plan, proof and admission state used by the scheduler. Session
// status and responsibility remain explicitly unknown unless durable facts
// establish them.
func (s *Service) GetMissionProjection(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (MissionProjection, error) {
	record, found, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return MissionProjection{}, err
	}
	if !found {
		return MissionProjection{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	schedule, err := s.GetSchedule(ctx, outcomeID, planID)
	if err != nil {
		return MissionProjection{}, err
	}
	if schedule.Plan.OutcomeID != record.ID || schedule.Plan.ContractRevisionNumber != record.CurrentRevisionNumber {
		return MissionProjection{}, fmt.Errorf("mission projection lineage does not bind the current Outcome")
	}
	projectID, ok, err := s.store.GetOutcomeProjectID(ctx, outcomeID)
	if err != nil {
		return MissionProjection{}, err
	}
	if !ok {
		return MissionProjection{}, fmt.Errorf("mission projection responsibility space has no Project lineage")
	}
	missionLabel := ""
	if projects, ok := s.store.(interface {
		GetProject(context.Context, string) (domain.ProjectRecord, bool, error)
	}); ok {
		project, found, err := projects.GetProject(ctx, string(projectID))
		if err != nil {
			return MissionProjection{}, err
		}
		if found {
			missionLabel = strings.TrimSpace(project.DisplayName)
		}
	}
	attention := map[domain.WorkUnitID]domain.NeedsYouQuestion{}
	if s.needsYou != nil {
		questions, err := s.needsYou.ListCurrentNeedsYouQuestions(ctx, outcomeID)
		if err != nil {
			return MissionProjection{}, err
		}
		for _, q := range questions {
			if q.OutcomeID == outcomeID && q.PlanRevisionID == planID && !q.WorkUnitID.IsZero() {
				attention[q.WorkUnitID] = q
			}
		}
	}
	view, err := composeMissionProjection(record, schedule, attention, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return s.store.LatestAttemptSessionRef(ctx, id)
	})
	if err != nil {
		return MissionProjection{}, err
	}
	view.MissionLabel = missionLabel
	if s.documents != nil && s.admission != nil {
		for i := range view.Nodes {
			spec, found, err := s.admission.GetApprovedExecutableSpec(ctx, planID, domain.WorkUnitID(view.Nodes[i].WorkUnitID))
			if err != nil {
				return MissionProjection{}, err
			}
			if !found || spec.OutcomeID != outcomeID || spec.ContractRevisionNumber != record.CurrentRevisionNumber || spec.DocumentContextID.IsZero() {
				continue
			}
			doc, found, err := s.documents.GetDocumentContext(ctx, spec.DocumentContextID)
			if err != nil {
				return MissionProjection{}, err
			}
			if found && doc.OutcomeID == outcomeID && doc.Revision == spec.DocumentContextRevision && doc.Digest == spec.DocumentContextDigest && doc.Approved() {
				view.Nodes[i].Links = append(view.Nodes[i].Links, MissionLink{Kind: "document_context", ID: string(doc.ID), Label: "Approved documents", State: "available"})
			}
		}
	}
	evidence := []domain.EvidenceItem(nil)
	var currentContractID domain.ContractRevisionID
	if s.proof != nil {
		revisions, err := s.store.ListContractRevisions(ctx, outcomeID)
		if err != nil {
			return MissionProjection{}, err
		}
		for _, revision := range revisions {
			if revision.Number == record.CurrentRevisionNumber {
				currentContractID = revision.ID
				break
			}
		}
		if currentContractID.IsZero() {
			return MissionProjection{}, fmt.Errorf("mission projection current Contract lineage is unavailable")
		}
		evidence, err = s.proof.ListEvidenceItems(ctx, outcomeID)
		if err != nil {
			return MissionProjection{}, err
		}
	}
	if s.receipts != nil {
		for i := range view.Nodes {
			n := &view.Nodes[i]
			if n.CurrentAttempt == nil {
				continue
			}
			receipt, found, err := s.receipts.GetAttemptReceipt(ctx, domain.AttemptID(n.CurrentAttempt.ID))
			if err != nil {
				return MissionProjection{}, err
			}
			if !found || receipt.OutcomeID != outcomeID || receipt.PlanRevisionID != planID || string(receipt.WorkUnitID) != n.WorkUnitID || receipt.ContractRevisionNumber != record.CurrentRevisionNumber {
				continue
			}
			state := "unavailable"
			if receipt.RetentionState.Complete() {
				state = "available"
			}
			n.Links = append(n.Links, MissionLink{Kind: "retained_result", ID: string(receipt.AttemptID), Label: "Retained result", State: state})
			n.ChangeSummary = measuredMissionChanges(receipt)
			for _, item := range evidence {
				if item.ContractRevisionID != currentContractID {
					continue
				}
				if item.SubjectType == domain.ProofSubjectAttempt && item.SubjectID == string(receipt.AttemptID) && item.SubjectRevision == receipt.ArtifactVersion {
					n.Links = append(n.Links, MissionLink{Kind: "evidence", ID: string(item.ID), Label: item.Summary, State: "available"})
				}
			}
		}
	}
	view.Generation = missionProjectionGeneration(view)
	for i := range view.Nodes {
		view.Nodes[i].Generation = missionNodeGeneration(view.Nodes[i])
	}
	return view, nil
}

func missionTopologyFingerprint(plan domain.PlanRevision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\x00%d\x00", plan.ID, plan.Number)
	for _, unit := range plan.WorkUnits {
		fmt.Fprintf(&b, "%s\x00", unit.ID)
		for _, dep := range unit.DependsOn {
			fmt.Fprintf(&b, "%s\x00", dep)
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func composeMissionProjection(record domain.Outcome, schedule ScheduleView, attention map[domain.WorkUnitID]domain.NeedsYouQuestion, latestRef func(domain.AttemptID) (domain.AttemptSessionRef, bool, error)) (MissionProjection, error) {
	view := MissionProjection{Version: MissionProjectionVersion, OutcomeID: record.ID, MissionID: record.SpaceID, ContractRevisionNumber: record.CurrentRevisionNumber, PlanRevisionID: schedule.Plan.ID, PlanRevisionNumber: schedule.Plan.Number, TopologyGeneration: schedule.Plan.Number, NextRunnableID: schedule.NextRunnableID, CustodyHeldBy: schedule.CustodyHeldBy, NoRunnableReason: string(schedule.NoRunnableReason)}
	view.TopologyFingerprint = missionTopologyFingerprint(schedule.Plan)
	for _, entry := range schedule.WorkUnits {
		n := MissionNode{WorkUnitID: string(entry.WorkUnit.ID), PlanRevisionID: string(schedule.Plan.ID), Title: entry.WorkUnit.Title, Role: string(entry.WorkUnit.Role), Inputs: append([]domain.WorkUnitInput(nil), entry.WorkUnit.Inputs...), Links: []MissionLink{}, DependsOn: append([]domain.WorkUnitID(nil), entry.WorkUnit.DependsOn...), ScheduleState: string(entry.State), BlockingDependencies: append([]domain.WorkUnitID(nil), entry.BlockingDependencies...), BlockedReason: string(entry.BlockedReason), BlockedDetail: entry.BlockedDetail, CriterionIDs: append([]domain.CriterionID(nil), entry.WorkUnit.CriterionIDs...), CriterionReady: entry.CriterionReady, Responsibility: "unconfirmed", UpdatedAt: schedule.Plan.CreatedAt}
		if binding, err := entry.WorkUnit.ExecutionBinding(); err == nil && (binding.ModelSelection == domain.ExecutionBindingModelProviderDefault || binding.ModelSelection == domain.ExecutionBindingModelExplicit) {
			n.ExecutionBinding = &MissionExecutionBinding{Provider: string(binding.Provider), ModelSelection: string(binding.ModelSelection), Model: binding.Model}
		}
		for _, dep := range entry.WorkUnit.DependsOn {
			view.Edges = append(view.Edges, MissionEdge{From: dep, To: entry.WorkUnit.ID})
		}
		if len(entry.Attempts) > 0 {
			latest := entry.Attempts[len(entry.Attempts)-1]
			for _, candidate := range entry.Attempts {
				if candidate.Number > latest.Number {
					latest = candidate
				}
			}
			n.CurrentAttempt = &MissionAttempt{ID: string(latest.ID), Number: latest.Number, Status: string(latest.Status), CreatedAt: latest.CreatedAt, UpdatedAt: latest.UpdatedAt}
			var ref domain.AttemptSessionRef
			ok := false
			var err error
			if latestRef != nil {
				ref, ok, err = latestRef(latest.ID)
			}
			if err != nil {
				return MissionProjection{}, err
			}
			if ok && ref.AttemptID == latest.ID {
				n.CurrentAttempt.Session = &MissionSession{RefID: string(ref.ID), Seq: ref.Seq, SessionID: ref.SessionID, Harness: string(ref.Harness), Mode: string(ref.Mode), Status: "unknown", BoundAt: ref.BoundAt}
			}
			if latest.UpdatedAt.After(n.UpdatedAt) {
				n.UpdatedAt = latest.UpdatedAt
			}
		}
		if q, ok := attention[entry.WorkUnit.ID]; ok {
			kind := ""
			if q.Kind == domain.NeedsYouApproval {
				kind = "needs_approval"
			}
			if q.Kind == domain.NeedsYouChoice {
				kind = "needs_choice"
			}
			if q.Kind == domain.NeedsYouInput {
				kind = "needs_input"
			}
			if kind != "" {
				n.Attention = &MissionAttention{Kind: kind, Summary: q.Reason, QuestionID: q.ID, Generation: q.Generation}
			}
			if q.UpdatedAt.After(n.UpdatedAt) {
				n.UpdatedAt = q.UpdatedAt
			}
		}
		if entry.WorkUnit.ID == schedule.NextRunnableID && (entry.State == WorkUnitScheduleRunnable || entry.State == WorkUnitScheduleRetryable) {
			n.NextAction = "start"
		}
		if n.UpdatedAt.After(view.UpdatedAt) {
			view.UpdatedAt = n.UpdatedAt
		}
		view.Nodes = append(view.Nodes, n)
	}
	view.Generation = missionProjectionGeneration(view)
	for i := range view.Nodes {
		view.Nodes[i].Generation = missionNodeGeneration(view.Nodes[i])
	}
	return view, nil
}

func measuredMissionChanges(receipt domain.AttemptReceipt) *MissionChangeSummary {
	if !receipt.Frozen() || !receipt.RetentionState.Complete() {
		return nil
	}
	var additions, deletions int64
	for _, file := range receipt.Files {
		if file.Additions == nil || file.Deletions == nil {
			return nil
		}
		additions += *file.Additions
		deletions += *file.Deletions
	}
	return &MissionChangeSummary{Additions: additions, Deletions: deletions, FilesChanged: int64(len(receipt.Files)), SourceAttemptID: string(receipt.AttemptID), ArtifactVersion: receipt.ArtifactVersion, MeasurementState: "measured"}
}

func missionNodeGeneration(node MissionNode) int64 { return digestGeneration(node) }
func missionProjectionGeneration(view MissionProjection) int64 {
	copy := view
	copy.Generation = 0
	copy.UpdatedAt = time.Time{}
	for i := range copy.Nodes {
		copy.Nodes[i].Generation = 0
		copy.Nodes[i].UpdatedAt = time.Time{}
	}
	return digestGeneration(copy)
}
func digestGeneration(value any) int64 {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)
	return int64(binary.BigEndian.Uint64(sum[:8]) & ((1 << 63) - 1))
}
