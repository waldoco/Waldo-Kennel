CREATE TABLE IF NOT EXISTS admission_verdicts (
    id TEXT PRIMARY KEY,
    outcome_id TEXT NOT NULL REFERENCES outcomes(id),
    plan_revision_id TEXT REFERENCES plan_revisions(id),
    contract_revision_number INTEGER,
    status TEXT NOT NULL CHECK(status IN ('admitted','rejected','stale')),
    policy_version TEXT NOT NULL,
    evaluated_at TIMESTAMP NOT NULL,
    verdict_json TEXT NOT NULL CHECK(json_valid(verdict_json)),
    CHECK(json_extract(verdict_json,'$.id')=id),
    CHECK(json_extract(verdict_json,'$.outcomeId')=outcome_id),
    CHECK(json_extract(verdict_json,'$.status')=status),
    CHECK(json_extract(verdict_json,'$.policyVersion')=policy_version),
    CHECK(json_extract(verdict_json,'$.planRevisionId') IS plan_revision_id),
    CHECK(json_extract(verdict_json,'$.contractRevisionNumber') IS contract_revision_number)
);
CREATE UNIQUE INDEX IF NOT EXISTS one_admitted_verdict_per_plan ON admission_verdicts(plan_revision_id) WHERE status='admitted';
CREATE UNIQUE INDEX IF NOT EXISTS work_units_id_plan_unique ON work_units(id, plan_revision_id);
CREATE TABLE IF NOT EXISTS approved_executable_specs (
    plan_revision_id TEXT NOT NULL REFERENCES plan_revisions(id),
    work_unit_id TEXT NOT NULL,
    digest TEXT NOT NULL UNIQUE,
    spec_json TEXT NOT NULL CHECK(json_valid(spec_json)),
    CHECK(json_extract(spec_json,'$.planRevisionId')=plan_revision_id),
    CHECK(json_extract(spec_json,'$.workUnitId')=work_unit_id),
    CHECK(json_extract(spec_json,'$.digest')=digest),
    PRIMARY KEY(plan_revision_id, work_unit_id),
    FOREIGN KEY(work_unit_id, plan_revision_id) REFERENCES work_units(id, plan_revision_id)
);
CREATE TABLE IF NOT EXISTS workspace_bound_launch_packets (
    attempt_id TEXT PRIMARY KEY REFERENCES attempts(id),
    spec_digest TEXT NOT NULL REFERENCES approved_executable_specs(digest),
    session_id TEXT NOT NULL,
    digest TEXT NOT NULL UNIQUE,
    packet_json TEXT NOT NULL CHECK(json_valid(packet_json)),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK(json_extract(packet_json,'$.attemptId')=attempt_id),
    CHECK(json_extract(packet_json,'$.specDigest')=spec_digest),
    CHECK(json_extract(packet_json,'$.sessionId')=session_id),
    CHECK(json_extract(packet_json,'$.digest')=digest)
);
CREATE TRIGGER IF NOT EXISTS admission_verdicts_immutable_update BEFORE UPDATE ON admission_verdicts BEGIN SELECT RAISE(ABORT, 'admission verdicts are immutable'); END;
CREATE TRIGGER IF NOT EXISTS approved_executable_specs_immutable_update BEFORE UPDATE ON approved_executable_specs BEGIN SELECT RAISE(ABORT, 'approved executable specs are immutable'); END;
CREATE TRIGGER IF NOT EXISTS workspace_bound_launch_packets_immutable_update BEFORE UPDATE ON workspace_bound_launch_packets BEGIN SELECT RAISE(ABORT, 'workspace-bound launch packets are immutable'); END;
CREATE TRIGGER IF NOT EXISTS admission_verdicts_immutable_delete BEFORE DELETE ON admission_verdicts BEGIN SELECT RAISE(ABORT, 'admission verdicts are immutable'); END;
CREATE TRIGGER IF NOT EXISTS approved_executable_specs_immutable_delete BEFORE DELETE ON approved_executable_specs BEGIN SELECT RAISE(ABORT, 'approved executable specs are immutable'); END;
CREATE TRIGGER IF NOT EXISTS workspace_bound_launch_packets_immutable_delete BEFORE DELETE ON workspace_bound_launch_packets BEGIN SELECT RAISE(ABORT, 'workspace-bound launch packets are immutable'); END;
