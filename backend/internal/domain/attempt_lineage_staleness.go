package domain

// LineageStaleFact records one superseded admitted input: an Attempt was
// admitted to consume a dependency WorkUnit's result at one artifact
// version, and that WorkUnit's current retained result has since moved to a
// different version (rework produced a newer Attempt). The old lineage is
// never rewritten - the fact derives from sealed input manifests and
// immutable receipts, so it survives restart byte for byte.
type LineageStaleFact struct {
	// AttemptID and WorkUnitID name the stale Attempt.
	AttemptID  AttemptID
	WorkUnitID WorkUnitID
	// DependencyUnitID is the unit whose current result moved.
	DependencyUnitID WorkUnitID
	// AdmittedAttemptID is the dependency Attempt the stale Attempt was
	// admitted to consume, at AdmittedVersion. CurrentVersion is what the
	// dependency unit retains now.
	AdmittedAttemptID AttemptID
	AdmittedVersion   string
	CurrentVersion    string
}

// LineageStaleness is the deterministic result of the staleness walk over
// one Plan revision's WorkUnit DAG. It is derived, never stored: the same
// durable facts always yield the same report, and because a WorkUnit's
// current version only moves forward to newer Attempts, a stale verdict is
// permanent unless the bytes genuinely come back (identical content is
// content-addressed identical, which is currency, not resurrection).
type LineageStaleness struct {
	// StaleAttempts maps each stale Attempt to the superseded-input facts
	// that condemn it.
	StaleAttempts map[AttemptID][]LineageStaleFact
	// StaleWorkUnits maps each stale WorkUnit to its own plus its inherited
	// dependency facts, in topological order.
	StaleWorkUnits map[WorkUnitID][]LineageStaleFact
}

// AttemptStale reports whether one Attempt's admitted inputs are superseded.
func (s LineageStaleness) AttemptStale(id AttemptID) bool {
	return len(s.StaleAttempts[id]) > 0
}

// WorkUnitStale reports whether a WorkUnit's proving lineage is stale,
// directly or through a dependency.
func (s LineageStaleness) WorkUnitStale(id WorkUnitID) bool {
	return len(s.StaleWorkUnits[id]) > 0
}

// WorkUnitStaleFacts returns the facts explaining one WorkUnit's staleness,
// nil when the unit is current.
func (s LineageStaleness) WorkUnitStaleFacts(id WorkUnitID) []LineageStaleFact {
	return s.StaleWorkUnits[id]
}

// DeriveLineageStaleness walks one Plan revision. attempts are the Outcome's
// attempts (the walk scopes them to the revision itself). inputRefs carries
// the admitted input references sealed into each Attempt's input manifest;
// an Attempt without a sealed input half (legacy, or no predecessors)
// carries no supersession risk. receipts carries the durable receipt per
// Attempt that has one; only a Retained receipt defines a WorkUnit's
// current result.
func DeriveLineageStaleness(plan PlanRevision, attempts []Attempt, inputRefs map[AttemptID][]AttemptManifestInputRef, receipts map[AttemptID]AttemptReceipt) (LineageStaleness, error) {
	out := LineageStaleness{StaleAttempts: map[AttemptID][]LineageStaleFact{}, StaleWorkUnits: map[WorkUnitID][]LineageStaleFact{}}
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		return out, err
	}
	inPlan := map[WorkUnitID]bool{}
	for _, unit := range plan.WorkUnits {
		inPlan[unit.ID] = true
	}

	// The current result of a WorkUnit is its newest Attempt with a Retained
	// receipt. Incomplete, unsupported or failed retentions move nothing.
	current := map[WorkUnitID]Attempt{}
	for _, attempt := range attempts {
		if attempt.PlanRevisionID != plan.ID || attempt.OutcomeID != plan.OutcomeID || attempt.ContractRevisionNumber != plan.ContractRevisionNumber {
			continue
		}
		if !inPlan[attempt.WorkUnitID] {
			continue
		}
		receipt, ok := receipts[attempt.ID]
		if !ok || receipt.RetentionState != RetentionRetained {
			continue
		}
		if prev, has := current[attempt.WorkUnitID]; !has || attempt.Number > prev.Number {
			current[attempt.WorkUnitID] = attempt
		}
	}

	attemptsByUnit := map[WorkUnitID][]Attempt{}
	for _, attempt := range attempts {
		if attempt.PlanRevisionID != plan.ID || attempt.OutcomeID != plan.OutcomeID || attempt.ContractRevisionNumber != plan.ContractRevisionNumber {
			continue
		}
		if !inPlan[attempt.WorkUnitID] {
			continue
		}
		attemptsByUnit[attempt.WorkUnitID] = append(attemptsByUnit[attempt.WorkUnitID], attempt)
	}

	// The walk runs in topological order so a dependency's Attempts are
	// classified before anything admitted to consume them. An Attempt is
	// stale when an admitted input's work unit has since retained a different
	// artifact version, or when the exact dependency Attempt it was admitted
	// to consume is itself stale: a result assembled from a superseded
	// lineage stays superseded even while its own bytes remain current.
	for _, unit := range ordered {
		for _, attempt := range attemptsByUnit[unit.ID] {
			for _, ref := range inputRefs[attempt.ID] {
				if cur, ok := current[ref.WorkUnitID]; ok {
					if currentVersion := receipts[cur.ID].ArtifactVersion; currentVersion != ref.ArtifactVersion {
						out.StaleAttempts[attempt.ID] = append(out.StaleAttempts[attempt.ID], LineageStaleFact{
							AttemptID: attempt.ID, WorkUnitID: attempt.WorkUnitID,
							DependencyUnitID: ref.WorkUnitID, AdmittedAttemptID: ref.AttemptID,
							AdmittedVersion: ref.ArtifactVersion, CurrentVersion: currentVersion,
						})
						continue
					}
				}
				if inherited := out.StaleAttempts[ref.AttemptID]; len(inherited) > 0 {
					for _, fact := range inherited {
						out.StaleAttempts[attempt.ID] = append(out.StaleAttempts[attempt.ID], LineageStaleFact{
							AttemptID: attempt.ID, WorkUnitID: attempt.WorkUnitID,
							DependencyUnitID: fact.DependencyUnitID, AdmittedAttemptID: fact.AdmittedAttemptID,
							AdmittedVersion: fact.AdmittedVersion, CurrentVersion: fact.CurrentVersion,
						})
					}
				}
			}
		}
	}

	// A WorkUnit is stale when its current proving Attempt is stale, or when
	// any dependency unit is stale. Topological order makes the inheritance
	// one pass, and the facts accumulate so the projection can name the
	// original supersession, not just the nearest hop.
	for _, unit := range ordered {
		var facts []LineageStaleFact
		if cur, ok := current[unit.ID]; ok {
			facts = append(facts, out.StaleAttempts[cur.ID]...)
		}
		for _, dependency := range unit.DependsOn {
			facts = append(facts, out.StaleWorkUnits[dependency]...)
		}
		if len(facts) > 0 {
			out.StaleWorkUnits[unit.ID] = facts
		}
	}
	return out, nil
}
