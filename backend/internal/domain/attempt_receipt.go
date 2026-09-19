package domain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WorkspaceKind is the custody shape a retained snapshot came from.
//
// It is recorded rather than assumed because the two are not equivalent: a
// Git worktree has isolation and revisions, a staged folder has neither. A
// plain folder must never be described as a worktree, and revision fields must
// stay empty for it rather than carrying a fabricated value.
type WorkspaceKind string

const (
	// WorkspaceGitWorktree is a Git worktree with real isolation and revisions.
	WorkspaceGitWorktree WorkspaceKind = "git_worktree"
	// WorkspaceStagedFolder is a single-writer staged directory. Supplied
	// document Outcomes use this; it has no revisions and no worktree
	// isolation, and claiming otherwise would be a lie about custody.
	WorkspaceStagedFolder WorkspaceKind = "staged_folder"
)

// Valid reports whether k is a known custody shape.
func (k WorkspaceKind) Valid() bool {
	return k == WorkspaceGitWorktree || k == WorkspaceStagedFolder
}

// RetentionState says how completely a snapshot represents what was produced.
//
// Partial retention is reported as partial. Presenting an incomplete snapshot
// as the artifact is the failure this vocabulary exists to prevent, because a
// downstream unit would then consume something nobody accounted for.
type RetentionState string

const (
	// RetentionRetained means every changed path is represented.
	RetentionRetained RetentionState = "retained"
	// RetentionIncomplete means the snapshot ran into a bound and does not
	// represent everything. It never satisfies a downstream handoff.
	RetentionIncomplete RetentionState = "incomplete"
	// RetentionUnsupported means the workspace held something this snapshot
	// cannot represent, and the case was refused rather than silently dropped.
	RetentionUnsupported RetentionState = "unsupported"
	// RetentionFailed means the workspace could not be read at all.
	RetentionFailed RetentionState = "failed"
)

// Valid reports whether s is a known retention state.
func (s RetentionState) Valid() bool {
	switch s {
	case RetentionRetained, RetentionIncomplete, RetentionUnsupported, RetentionFailed:
		return true
	}
	return false
}

// Complete reports whether this retention may satisfy a downstream handoff.
// Only a fully retained snapshot may: anything else means the successor would
// start from an unaccounted workspace.
func (s RetentionState) Complete() bool { return s == RetentionRetained }

// ArtifactChangeKind is what happened to one path.
type ArtifactChangeKind string

// What happened to one path in a retained snapshot.
const (
	// ArtifactAdded is a path the attempt created and tracked.
	ArtifactAdded ArtifactChangeKind = "added"
	// ArtifactModified is a tracked path the attempt changed.
	ArtifactModified ArtifactChangeKind = "modified"
	// ArtifactDeleted is output too. A downstream unit that re-creates a file
	// its upstream removed has not received the upstream's work.
	ArtifactDeleted ArtifactChangeKind = "deleted"
	// ArtifactUntracked is a path present in the workspace but never tracked.
	// It is retained rather than dropped: untracked output is still output.
	ArtifactUntracked ArtifactChangeKind = "untracked"
)

// Valid reports whether k is a known change kind.
func (k ArtifactChangeKind) Valid() bool {
	switch k {
	case ArtifactAdded, ArtifactModified, ArtifactDeleted, ArtifactUntracked:
		return true
	}
	return false
}

// ArtifactFile is one changed path in a retained snapshot.
type ArtifactFile struct {
	ID           string
	AttemptID    AttemptID
	RelativePath string
	ChangeKind   ArtifactChangeKind
	// ContentDigest is empty for a deletion and for content the snapshot
	// declined to read. Empty means unknown, never "empty file".
	ContentDigest string
	SizeBytes     *int64
	FileMode      *int64
	IsBinary      bool
	// Additions and Deletions are retained only when Kennel measured text-line
	// changes against the Attempt's frozen base. Nil means unmeasured, never zero.
	Additions *int64
	Deletions *int64
	// UnsupportedReason records a path that could not be represented, so an
	// unsupported case stays visible per file instead of collapsing the whole
	// receipt.
	UnsupportedReason string
}

// Validate checks one manifest entry, including that its path stays inside the
// workspace the snapshot owns.
func (f ArtifactFile) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("artifact file id is required")
	}
	if f.AttemptID.IsZero() {
		return fmt.Errorf("artifact file attempt id is required")
	}
	if strings.TrimSpace(f.RelativePath) == "" {
		return fmt.Errorf("artifact file relative path is required")
	}
	// A snapshot is confined to the workspace it owns. Treat both slash styles
	// as separators so a manifest cannot become an escape when it is exported
	// on another platform. A name such as "notes..md" is ordinary and safe.
	if err := validateArtifactPath(f.RelativePath); err != nil {
		return fmt.Errorf("artifact file path %q must stay inside the workspace", f.RelativePath)
	}
	if !f.ChangeKind.Valid() {
		return fmt.Errorf("artifact file change kind %q is invalid", f.ChangeKind)
	}
	if f.ChangeKind == ArtifactDeleted && f.ContentDigest != "" {
		return fmt.Errorf("a deleted artifact file cannot carry a content digest")
	}
	if f.ChangeKind != ArtifactDeleted && strings.TrimSpace(f.ContentDigest) == "" && f.UnsupportedReason == "" {
		return fmt.Errorf("artifact file %q is missing content identity", f.RelativePath)
	}
	if (f.Additions == nil) != (f.Deletions == nil) {
		return fmt.Errorf("artifact file %q change measurement is incomplete", f.RelativePath)
	}
	if f.Additions != nil && (*f.Additions < 0 || *f.Deletions < 0) {
		return fmt.Errorf("artifact file %q change measurement is negative", f.RelativePath)
	}
	if f.IsBinary && f.Additions != nil {
		return fmt.Errorf("binary artifact file %q cannot carry text-line measurements", f.RelativePath)
	}
	return nil
}

func validateArtifactPath(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("path is empty or contains NUL")
	}
	// Backslashes are normalized only for validation; retaining them in a
	// canonical manifest would make the same file ambiguous across platforms.
	name = strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "//") ||
		(len(name) >= 2 && name[1] == ':') {
		return fmt.Errorf("path is absolute or drive-qualified")
	}
	components := strings.Split(name, "/")
	for _, component := range components {
		if component == ".." {
			return fmt.Errorf("path traverses its root")
		}
		if component == "" || component == "." {
			return fmt.Errorf("path has an empty or dot component")
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path escapes its root")
	}
	return nil
}

// AttemptReceipt is the durable record of what one Attempt produced.
//
// It is the bridge between execution ending and anything downstream trusting
// the result: a provider's description of its own output is a claim, and a
// successor cannot consume "whatever is in some worktree". It also carries
// everything a delivery manifest needs, because provenance cannot be
// backfilled into a receipt that is already frozen.
type AttemptReceipt struct {
	AttemptID              AttemptID
	OutcomeID              OutcomeID
	PlanRevisionID         PlanRevisionID
	WorkUnitID             WorkUnitID
	ContractRevisionNumber int64

	// ArtifactVersion is the immutable identity of this retained set: a digest
	// over the file manifest. An accepted-result export binds to this exact
	// value so a later attempt cannot change what was reviewed.
	ArtifactVersion string

	WorkspaceKind WorkspaceKind
	WorkspacePath string

	RepositoryPath     string
	RepositoryIdentity string

	// BaseRevision and ResultRevision are meaningful only for a git worktree.
	BaseRevision   string
	ResultRevision string
	WorkspaceDirty bool

	RetentionState  RetentionState
	RetentionDetail string

	// TerminationReason is why execution ended, as observed. Never parsed from
	// provider prose.
	TerminationReason string

	ObservedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// FrozenAt is set once the receipt has been used as review evidence. A
	// frozen receipt is never replaced, so later work cannot silently overwrite
	// what the owner reviewed.
	FrozenAt *time.Time

	Files []ArtifactFile
}

// Frozen reports whether this receipt has been used as review evidence.
func (r AttemptReceipt) Frozen() bool { return r.FrozenAt != nil }

// CanonicallyEqual reports whether two receipts describe the same retained
// result for every fact downstream custody seals: producing lineage,
// workspace and revision metadata, retention state and detail, termination
// reason, observation time, and the exact file manifest - every non-storage
// field of every entry, including nil-vs-measured line metrics. Row identity
// and write timestamps are storage facts, not custody facts.
func (r AttemptReceipt) CanonicallyEqual(other AttemptReceipt) bool {
	if r.AttemptID != other.AttemptID || r.OutcomeID != other.OutcomeID ||
		r.PlanRevisionID != other.PlanRevisionID || r.WorkUnitID != other.WorkUnitID ||
		r.ContractRevisionNumber != other.ContractRevisionNumber ||
		r.ArtifactVersion != other.ArtifactVersion ||
		r.WorkspaceKind != other.WorkspaceKind || r.WorkspacePath != other.WorkspacePath ||
		r.RepositoryPath != other.RepositoryPath || r.RepositoryIdentity != other.RepositoryIdentity ||
		r.BaseRevision != other.BaseRevision || r.ResultRevision != other.ResultRevision ||
		r.WorkspaceDirty != other.WorkspaceDirty ||
		r.RetentionState != other.RetentionState || r.RetentionDetail != other.RetentionDetail ||
		r.TerminationReason != other.TerminationReason ||
		!r.ObservedAt.Equal(other.ObservedAt) {
		return false
	}
	if len(r.Files) != len(other.Files) {
		return false
	}
	mine := append([]ArtifactFile(nil), r.Files...)
	theirs := append([]ArtifactFile(nil), other.Files...)
	sort.Slice(mine, func(i, j int) bool { return canonicalFileSortKey(mine[i]) < canonicalFileSortKey(mine[j]) })
	sort.Slice(theirs, func(i, j int) bool { return canonicalFileSortKey(theirs[i]) < canonicalFileSortKey(theirs[j]) })
	for i := range mine {
		if !canonicalFileEqual(mine[i], theirs[i]) {
			return false
		}
	}
	return true
}

// canonicalFileEqual compares every non-storage ArtifactFile field. ID and
// AttemptID are storage identity; everything else is a retained custody
// fact, including measured line counts. A nil measurement means unmeasured,
// which is a different fact from a measured zero, so pointer fields compare
// nil-ness first and value second.
func canonicalFileEqual(a, b ArtifactFile) bool {
	return a.RelativePath == b.RelativePath &&
		a.ChangeKind == b.ChangeKind &&
		a.ContentDigest == b.ContentDigest &&
		int64PtrEqual(a.SizeBytes, b.SizeBytes) &&
		int64PtrEqual(a.FileMode, b.FileMode) &&
		a.IsBinary == b.IsBinary &&
		int64PtrEqual(a.Additions, b.Additions) &&
		int64PtrEqual(a.Deletions, b.Deletions) &&
		a.UnsupportedReason == b.UnsupportedReason
}

func int64PtrEqual(a, b *int64) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

// canonicalFileSortKey orders manifests deterministically so equal multisets
// sort into identical sequences before pairwise comparison. Equality itself
// is decided by canonicalFileEqual, never by this key.
func canonicalFileSortKey(f ArtifactFile) string {
	deref := func(p *int64) string {
		if p == nil {
			return "nil"
		}
		return "val:" + strconv.FormatInt(*p, 10)
	}
	return strings.Join([]string{
		f.RelativePath, string(f.ChangeKind), f.ContentDigest,
		deref(f.SizeBytes), deref(f.FileMode), strconv.FormatBool(f.IsBinary),
		deref(f.Additions), deref(f.Deletions), f.UnsupportedReason,
	}, "\x00")
}

// Validate checks that the receipt carries full producing lineage and does not
// misdescribe its custody shape.
func (r AttemptReceipt) Validate() error {
	if r.AttemptID.IsZero() {
		return fmt.Errorf("attempt receipt attempt id is required")
	}
	if r.OutcomeID.IsZero() || r.PlanRevisionID.IsZero() || r.WorkUnitID.IsZero() {
		return fmt.Errorf("attempt receipt requires full producing lineage")
	}
	if r.ContractRevisionNumber < 1 {
		return fmt.Errorf("attempt receipt contract revision number is required")
	}
	if strings.TrimSpace(r.ArtifactVersion) == "" {
		return fmt.Errorf("attempt receipt artifact version is required")
	}
	if !r.WorkspaceKind.Valid() {
		return fmt.Errorf("attempt receipt workspace kind %q is invalid", r.WorkspaceKind)
	}
	if !r.RetentionState.Valid() {
		return fmt.Errorf("attempt receipt retention state %q is invalid", r.RetentionState)
	}
	// A staged folder has no revisions. Carrying one would misdescribe custody.
	if r.WorkspaceKind == WorkspaceStagedFolder && (r.BaseRevision != "" || r.ResultRevision != "") {
		return fmt.Errorf("a staged folder has no revisions and must not report one")
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("attempt receipt observed timestamp is required")
	}
	for _, file := range r.Files {
		if err := file.Validate(); err != nil {
			return err
		}
		if r.RetentionState == RetentionRetained && file.UnsupportedReason != "" {
			return fmt.Errorf("retained artifact file %q is unsupported: %s", file.RelativePath, file.UnsupportedReason)
		}
	}
	if r.RetentionState == RetentionRetained && r.ArtifactVersion != string(ArtifactManifestDigest(r.Files)) {
		return fmt.Errorf("artifact version does not match its manifest")
	}
	return nil
}

// ArtifactManifestDigest is the canonical artifact version for a file set.
//
// It digests each path with its change kind, content identity, size, mode and
// binary/unsupported semantics, in a stable order. Length-prefixing every
// field avoids delimiter collisions and executable mode is part of the
// artifact identity. Two attempts producing identical output share a version,
// which is what lets a downstream handoff assert it received the exact
// upstream artifact.
func ArtifactManifestDigest(files []ArtifactFile) SHA256Digest {
	ordered := make([][]byte, 0, len(files))
	for _, file := range files {
		var encoded bytes.Buffer
		writeManifestField := func(value string) {
			_ = binary.Write(&encoded, binary.BigEndian, uint64(len(value)))
			_, _ = encoded.WriteString(value)
		}
		writeManifestField(file.RelativePath)
		writeManifestField(string(file.ChangeKind))
		writeManifestField(file.ContentDigest)
		if file.SizeBytes == nil {
			writeManifestField("size:unknown")
		} else {
			writeManifestField(fmt.Sprintf("size:%d", *file.SizeBytes))
		}
		if file.FileMode == nil {
			writeManifestField("mode:unknown")
		} else {
			writeManifestField(fmt.Sprintf("mode:%d", *file.FileMode))
		}
		if file.IsBinary {
			writeManifestField("binary:1")
		} else {
			writeManifestField("binary:0")
		}
		writeManifestField(file.UnsupportedReason)
		ordered = append(ordered, encoded.Bytes())
	}
	// Sorted so storage or traversal order cannot change the identity.
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i], ordered[j]) < 0 })
	var manifest bytes.Buffer
	for _, entry := range ordered {
		_ = binary.Write(&manifest, binary.BigEndian, uint64(len(entry)))
		_, _ = manifest.Write(entry)
	}
	return DigestSHA256(manifest.Bytes())
}
