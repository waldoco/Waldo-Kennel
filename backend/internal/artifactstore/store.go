// Package artifactstore retains the bytes behind an Attempt receipt.
//
// The receipt database is the authority for lineage and immutable manifest
// identity. This package owns only the content side of that boundary: bytes
// are staged under Kennel's application state, verified while they are still
// staged, and published under the manifest version. It deliberately accepts a
// daemon-owned workspace identity rather than a client supplied path.
package artifactstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

const (
	defaultMaxFiles = 5000
	defaultMaxBytes = 512 << 20
	maxReadBuffer   = 32 << 10
)

// Config bounds one capture and names the isolated application-state root.
// A zero bound uses a conservative default. The root must not be a workspace
// directory; callers normally pass <data-dir>/artifacts.
type Config struct {
	Root        string
	MaxFiles    int
	MaxBytes    int64
	MaxDuration time.Duration
}

// Input is assembled from the Attempt and its daemon-owned session/workspace
// record. No HTTP/controller input should be copied here without resolving it
// through those records first.
type Input struct {
	AttemptID              domain.AttemptID
	OutcomeID              domain.OutcomeID
	PlanRevisionID         domain.PlanRevisionID
	WorkUnitID             domain.WorkUnitID
	ContractRevisionNumber int64
	WorkspaceKind          domain.WorkspaceKind
	WorkspacePath          string
	RepositoryPath         string
	BaseRevision           string
	BaseRef                string
	TerminationReason      string
}

// Result contains the receipt and its published content directory. The
// directory is derived from AttemptID and ArtifactVersion and is safe to keep
// after the source workspace is removed.
type Result struct {
	Receipt      domain.AttemptReceipt
	ContentDir   string
	PublishedNew bool
}

// Store is a filesystem content retainer. It is intentionally independent of
// SQLite so the database receipt can be committed/retried separately after a
// filesystem crash.
type Store struct {
	root        string
	maxFiles    int
	maxBytes    int64
	maxDuration time.Duration
}

// New creates a bounded filesystem artifact store under an application-state root.
func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, errors.New("artifact store root is required")
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("artifact store root: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create artifact store root: %w", err)
	}
	return &Store{root: filepath.Clean(root), maxFiles: positiveOr(cfg.MaxFiles, defaultMaxFiles), maxBytes: positiveOr64(cfg.MaxBytes, defaultMaxBytes), maxDuration: cfg.MaxDuration}, nil
}

func positiveOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
func positiveOr64(v, fallback int64) int64 {
	if v > 0 {
		return v
	}
	return fallback
}

// Retain captures the files represented by the workspace at one quiescent
// point. Git captures both base..HEAD and working-tree status, so an agent
// commit is not lost merely because HEAD is no longer dirty.
func (s *Store) Retain(ctx context.Context, in Input) (Result, error) {
	if s.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.maxDuration)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if in.AttemptID.IsZero() || in.OutcomeID.IsZero() || in.PlanRevisionID.IsZero() || in.WorkUnitID.IsZero() || in.ContractRevisionNumber < 1 {
		return Result{}, errors.New("artifact retention requires complete Attempt lineage")
	}
	if !in.WorkspaceKind.Valid() {
		return Result{}, fmt.Errorf("unsupported workspace kind %q", in.WorkspaceKind)
	}
	root, err := physicalDirectory(in.WorkspacePath)
	if err != nil {
		return Result{}, fmt.Errorf("workspace custody: %w", err)
	}

	files, capture, err := s.collect(ctx, root, in)
	if err != nil {
		return Result{}, err
	}
	if len(files) > s.maxFiles {
		capture.degrade(domain.RetentionIncomplete, fmt.Sprintf("file bound exceeded: %d files (limit %d)", len(files), s.maxFiles))
		files = files[:s.maxFiles]
	}
	version := string(domain.ArtifactManifestDigest(files))
	receipt := domain.AttemptReceipt{
		AttemptID: in.AttemptID, OutcomeID: in.OutcomeID, PlanRevisionID: in.PlanRevisionID, WorkUnitID: in.WorkUnitID,
		ContractRevisionNumber: in.ContractRevisionNumber, ArtifactVersion: version,
		WorkspaceKind: in.WorkspaceKind, WorkspacePath: root, RepositoryPath: in.RepositoryPath,
		RepositoryIdentity: capture.repositoryIdentity, BaseRevision: in.BaseRevision, ResultRevision: capture.resultRevision,
		WorkspaceDirty: capture.dirty, RetentionState: capture.state, RetentionDetail: capture.detail,
		TerminationReason: in.TerminationReason, ObservedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Files: files,
	}
	if err := receipt.Validate(); err != nil {
		return Result{}, fmt.Errorf("retained receipt invalid: %w", err)
	}

	stage := filepath.Join(s.root, ".staging", uuid.NewString())
	if err := os.MkdirAll(stage, 0o750); err != nil {
		return Result{}, fmt.Errorf("create artifact staging: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }() // only removes this capture's private staging area
	if err := s.publishFiles(ctx, stage, files, capture.bytes, receipt.RetentionState == domain.RetentionRetained); err != nil {
		return Result{}, err
	}
	final := ""
	if receipt.RetentionState.Complete() {
		final = filepath.Join(s.root, string(in.AttemptID), version)
		if err := s.publishDirectory(stage, final, receipt); err != nil {
			return Result{}, err
		}
	}
	return Result{Receipt: receipt, ContentDir: final, PublishedNew: final != ""}, nil
}

// ObserveVersion measures what a workspace currently holds, without
// publishing anything.
//
// It exists so a caller can ask whether the bytes it retained are still the
// bytes present after something else touched the workspace — a deterministic
// check that rewrites a lockfile, say. Recomputing the manifest is the only
// honest way to answer that: a pass taken from pre-change content must not be
// attached to post-change output.
func (s *Store) ObserveVersion(ctx context.Context, in Input) (string, error) {
	if s.maxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.maxDuration)
		defer cancel()
	}
	if !in.WorkspaceKind.Valid() {
		return "", fmt.Errorf("unsupported workspace kind %q", in.WorkspaceKind)
	}
	root, err := physicalDirectory(in.WorkspacePath)
	if err != nil {
		return "", fmt.Errorf("workspace custody: %w", err)
	}
	files, capture, err := s.collect(ctx, root, in)
	if err != nil {
		return "", err
	}
	if !capture.state.Complete() {
		// An incomplete observation cannot prove the result is unchanged, and
		// claiming a version for it would be worse than saying so.
		return "", fmt.Errorf("workspace observation is %s: %s", capture.state, capture.detail)
	}
	return string(domain.ArtifactManifestDigest(files)), nil
}

type captureState struct {
	state              domain.RetentionState
	detail             string
	dirty              bool
	repositoryIdentity string
	resultRevision     string
	bytes              map[string][]byte
	total              int64
}

// degrade lowers the retention verdict and never raises it. A snapshot that
// hit a byte or file bound is missing content outright, so incomplete outranks
// unsupported; without an ordering the reported state would depend on which
// path the walk happened to reach last.
func (c *captureState) degrade(state domain.RetentionState, detail string) {
	if retentionSeverity(state) <= retentionSeverity(c.state) {
		return
	}
	c.state, c.detail = state, detail
}

func retentionSeverity(state domain.RetentionState) int {
	switch state {
	case domain.RetentionRetained:
		return 0
	case domain.RetentionUnsupported:
		return 1
	case domain.RetentionIncomplete:
		return 2
	case domain.RetentionFailed:
		return 3
	}
	return 3
}

func (s *Store) collect(ctx context.Context, root string, in Input) ([]domain.ArtifactFile, *captureState, error) {
	c := &captureState{state: domain.RetentionRetained, bytes: map[string][]byte{}}
	paths := map[string]domain.ArtifactChangeKind{}
	measurements := map[string][2]int64{}
	if in.WorkspaceKind == domain.WorkspaceGitWorktree {
		if strings.TrimSpace(in.BaseRevision) == "" {
			return nil, c, errors.New("git retention requires the frozen base revision")
		}
		identity, err := gitOutput(ctx, root, "rev-parse", "--git-dir")
		if err != nil {
			return nil, c, fmt.Errorf("identify repository: %w", err)
		}
		c.repositoryIdentity = strings.TrimSpace(identity)
		head, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return nil, c, fmt.Errorf("read result revision: %w", err)
		}
		c.resultRevision = strings.TrimSpace(head)
		base, err := gitOutput(ctx, root, "diff", "--name-status", "-z", in.BaseRevision, "HEAD")
		if err != nil {
			return nil, c, fmt.Errorf("read committed output since base: %w", err)
		}
		parseNameStatus(base, paths)
		status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "--untracked-files=all", "-z")
		if err != nil {
			return nil, c, fmt.Errorf("read working-tree output: %w", err)
		}
		parsePorcelain(status, paths)
		numstat, err := gitOutput(ctx, root, "diff", "--numstat", "-z", in.BaseRevision)
		if err != nil {
			return nil, c, fmt.Errorf("measure retained output: %w", err)
		}
		parseNumstat(numstat, measurements)
		c.dirty = len(paths) > 0
	} else {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(rel)
			if entry.IsDir() {
				return nil
			}
			paths[name] = domain.ArtifactUntracked
			return nil
		}); err != nil {
			return nil, c, fmt.Errorf("walk staged workspace: %w", err)
		}
		c.dirty = len(paths) > 0
	}
	keys := make([]string, 0, len(paths))
	for p := range paths {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	files := make([]domain.ArtifactFile, 0, len(keys))
	for _, name := range keys {
		if err := ctx.Err(); err != nil {
			return nil, c, err
		}
		kind := paths[name]
		f := domain.ArtifactFile{ID: "artifact-" + uuid.NewString(), AttemptID: in.AttemptID, RelativePath: name, ChangeKind: kind}
		if counts, ok := measurements[name]; ok {
			additions, deletions := counts[0], counts[1]
			f.Additions, f.Deletions = &additions, &deletions
		}
		if _, err := confinedPath(root, name); err != nil {
			return nil, c, fmt.Errorf("workspace path %q: %w", name, err)
		}
		if secretPath(name) {
			f.UnsupportedReason = "secret-like path is excluded from retained content"
			c.degrade(domain.RetentionUnsupported, "secret-like output was not copied")
			files = append(files, f)
			continue
		}
		full, err := confinedPath(root, name)
		if err != nil {
			return nil, c, err
		}
		info, err := os.Lstat(full)
		if errors.Is(err, os.ErrNotExist) || kind == domain.ArtifactDeleted {
			f.ChangeKind = domain.ArtifactDeleted
			files = append(files, f)
			continue
		}
		if err != nil {
			return nil, c, fmt.Errorf("stat %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			f.UnsupportedReason = "symlink and special files are not retained"
			c.degrade(domain.RetentionUnsupported, "workspace contains an unsupported filesystem entry")
			files = append(files, f)
			continue
		}
		remaining := s.maxBytes - c.total
		if remaining <= 0 || info.Size() > remaining {
			// The bound is real and the content is declined, but the path is
			// still part of what the Attempt produced. Record the decline on the
			// file so the receipt stays valid and names the path it is missing.
			// Appending a digest-less entry instead would fail receipt
			// validation and lose the whole snapshot over one oversized file.
			f.UnsupportedReason = fmt.Sprintf("content declined: file needs %d bytes and the retention bound has %d remaining", info.Size(), max64(remaining, 0))
			c.degrade(domain.RetentionIncomplete, fmt.Sprintf("byte bound exceeded at %s", name))
			files = append(files, f)
			continue
		}
		content, err := readStable(ctx, full, info, remaining)
		if err != nil {
			f.UnsupportedReason = err.Error()
			c.degrade(domain.RetentionUnsupported, "workspace changed during retention")
			files = append(files, f)
			continue
		}
		digest := sha256.Sum256(content)
		sz := int64(len(content))
		mode := int64(info.Mode().Perm())
		f.ContentDigest = hex.EncodeToString(digest[:])
		f.SizeBytes = &sz
		f.FileMode = &mode
		f.IsBinary = bytes.IndexByte(content, 0) >= 0
		if f.Additions == nil && kind == domain.ArtifactUntracked && !f.IsBinary {
			additions, deletions := int64(bytes.Count(content, []byte{'\n'})), int64(0)
			if len(content) > 0 && content[len(content)-1] != '\n' {
				additions++
			}
			f.Additions, f.Deletions = &additions, &deletions
		}
		c.bytes[name] = content
		c.total += sz
		files = append(files, f)
	}
	return files, c, nil
}

func physicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("workspace path must be absolute")
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("workspace path is not a directory")
	}
	physical, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", err
	}
	return filepath.Clean(physical), nil
}

func confinedPath(root, name string) (string, error) {
	if err := (domain.ArtifactFile{ID: "path", AttemptID: "path", RelativePath: name, ChangeKind: domain.ArtifactUntracked, ContentDigest: "path"}).Validate(); err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("workspace path escapes custody")
	}
	return full, nil
}

func readStable(ctx context.Context, path string, before os.FileInfo, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out bytes.Buffer
	buf := make([]byte, maxReadBuffer)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if int64(out.Len())+int64(n) > maxBytes {
				return nil, errors.New("file exceeds retention byte bound")
			}
			_, _ = out.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || after.Size() != before.Size() || after.ModTime() != before.ModTime() || after.Mode().Perm() != before.Mode().Perm() {
		return nil, errors.New("file changed during read")
	}
	return out.Bytes(), nil
}

func secretPath(name string) bool {
	lower := strings.ToLower(filepath.ToSlash(name))
	base := filepath.Base(lower)
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.Contains(base, "credentials") || strings.Contains(base, "secret")
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func parseNameStatus(data string, paths map[string]domain.ArtifactChangeKind) {
	parts := bytes.Split([]byte(data), []byte{0})
	for i := 0; i+1 < len(parts); {
		status := string(parts[i])
		name := string(parts[i+1])
		i += 2
		if status == "" || name == "" {
			continue
		}
		if strings.HasPrefix(status, "D") {
			paths[name] = domain.ArtifactDeleted
		} else if strings.HasPrefix(status, "A") {
			paths[name] = domain.ArtifactAdded
		} else {
			paths[name] = domain.ArtifactModified
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i < len(parts) {
				name = string(parts[i])
				i++
				paths[name] = domain.ArtifactModified
			}
		}
	}
}

func parseNumstat(data string, out map[string][2]int64) {
	parts := bytes.Split([]byte(data), []byte{0})
	for _, part := range parts {
		fields := bytes.SplitN(part, []byte{'\t'}, 3)
		if len(fields) != 3 {
			continue
		}
		var additions, deletions int64
		if _, err := fmt.Sscan(string(fields[0]), &additions); err != nil {
			continue
		}
		if _, err := fmt.Sscan(string(fields[1]), &deletions); err != nil {
			continue
		}
		if name := string(fields[2]); name != "" {
			out[name] = [2]int64{additions, deletions}
		}
	}
}

func parsePorcelain(data string, paths map[string]domain.ArtifactChangeKind) {
	parts := bytes.Split([]byte(data), []byte{0})
	for i := 0; i < len(parts); i++ {
		entry := string(parts[i])
		// Porcelain v1 -z is "XY<space><path>": exactly one space separates the
		// status from the path, and every byte after it belongs to the filename.
		// Trimming would silently retain " report.txt " under the wrong name.
		if len(entry) < 4 || entry[2] != ' ' {
			continue
		}
		status, name := entry[:2], entry[3:]
		kind := domain.ArtifactModified
		if status == "??" {
			kind = domain.ArtifactUntracked
		} else if strings.Contains(status, "D") {
			kind = domain.ArtifactDeleted
		} else if strings.Contains(status, "A") {
			kind = domain.ArtifactAdded
		}
		paths[name] = kind
		if strings.Contains(status, "R") || strings.Contains(status, "C") {
			if i+1 < len(parts) {
				i++
				paths[string(parts[i])] = domain.ArtifactModified
			}
		}
	}
}

func (s *Store) publishFiles(ctx context.Context, stage string, files []domain.ArtifactFile, content map[string][]byte, complete bool) error {
	if !complete {
		return nil
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.ChangeKind == domain.ArtifactDeleted || file.UnsupportedReason != "" {
			continue
		}
		name := filepath.FromSlash(file.RelativePath)
		dst := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if file.FileMode != nil {
			modeValue := *file.FileMode
			if modeValue < 0 || modeValue > int64(^uint32(0)) {
				return fmt.Errorf("artifact %s has an invalid file mode", file.RelativePath)
			}
			mode = os.FileMode(uint32(modeValue))
		}
		if err := writeFileSynced(dst, content[file.RelativePath], mode); err != nil {
			return err
		}
	}
	return syncTree(stage)
}

func (s *Store) publishDirectory(stage, final string, receipt domain.AttemptReceipt) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return err
	}
	if info, err := os.Stat(final); err == nil {
		if !info.IsDir() {
			return errors.New("published artifact path is not a directory")
		}
		return verifyPublished(final, receipt)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stage, final); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	return syncDir(filepath.Dir(final))
}

func verifyPublished(root string, receipt domain.AttemptReceipt) error {
	for _, file := range receipt.Files {
		if file.ChangeKind == domain.ArtifactDeleted || file.UnsupportedReason != "" {
			continue
		}
		path, err := confinedPath(root, file.RelativePath)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("published artifact %s: %w", file.RelativePath, err)
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != file.ContentDigest {
			return fmt.Errorf("published artifact %s digest mismatch", file.RelativePath)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || file.FileMode == nil || int64(info.Mode().Perm()) != *file.FileMode {
			return fmt.Errorf("published artifact %s mode mismatch", file.RelativePath)
		}
	}
	return nil
}

// writeFileSynced writes one blob and flushes it before returning. The mode is
// set explicitly because O_CREATE applies umask, and publication verifies the
// recorded mode.
func writeFileSynced(path string, content []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// syncTree flushes every directory in the staged tree, deepest first, after its
// files are already durable. Syncing only the top directory records that the
// tree exists, not what is inside it, so a crash could publish a manifest
// version whose nested content was never written.
func syncTree(root string) error {
	var dirs []string
	if err := filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

// Read verifies a published blob against the immutable manifest entry before
// handing it to a successor. A corrupt or missing blob is never downgraded to
// an empty file.
func (s *Store) Read(ctx context.Context, receipt domain.AttemptReceipt, file domain.ArtifactFile) ([]byte, os.FileMode, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if err := receipt.Validate(); err != nil {
		return nil, 0, err
	}
	if !receipt.RetentionState.Complete() {
		return nil, 0, errors.New("artifact receipt is not complete")
	}
	path := filepath.Join(s.root, string(receipt.AttemptID), receipt.ArtifactVersion, filepath.FromSlash(file.RelativePath))
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != file.ContentDigest {
		return nil, 0, errors.New("artifact blob digest mismatch")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("artifact blob is not regular")
	}
	if file.FileMode != nil && int64(info.Mode().Perm()) != *file.FileMode {
		return nil, 0, errors.New("artifact blob mode mismatch")
	}
	return content, info.Mode(), nil
}
