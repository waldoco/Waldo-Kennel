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
	Attention                         *MissionAttention
	NextAction                        string
	Responsibility                    string
	UpdatedAt                         time.Time
	Generation                        int64
}
type MissionEdge struct{ From, To domain.WorkUnitID }
type MissionProjection struct {
	Version                       int
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
	if _, ok, err := s.store.GetOutcomeProjectID(ctx, outcomeID); err != nil {
		return MissionProjection{}, err
	} else if !ok {
		return MissionProjection{}, fmt.Errorf("mission projection responsibility space has no Project lineage")
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
	return composeMissionProjection(record, schedule, attention, func(id domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
		return s.store.LatestAttemptSessionRef(ctx, id)
	})
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
		n := MissionNode{WorkUnitID: string(entry.WorkUnit.ID), PlanRevisionID: string(schedule.Plan.ID), Title: entry.WorkUnit.Title, Role: string(entry.WorkUnit.Role), Inputs: append([]domain.WorkUnitInput(nil), entry.WorkUnit.Inputs...), DependsOn: append([]domain.WorkUnitID(nil), entry.WorkUnit.DependsOn...), ScheduleState: string(entry.State), BlockingDependencies: append([]domain.WorkUnitID(nil), entry.BlockingDependencies...), BlockedReason: string(entry.BlockedReason), BlockedDetail: entry.BlockedDetail, CriterionIDs: append([]domain.CriterionID(nil), entry.WorkUnit.CriterionIDs...), CriterionReady: entry.CriterionReady, Responsibility: "unconfirmed", UpdatedAt: schedule.Plan.CreatedAt}
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
