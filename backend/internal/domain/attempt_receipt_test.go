package domain

import "testing"

func TestArtifactFileChangeMeasurementsAreCompleteNonNegativeAndTextOnly(t *testing.T) {
	zero, one, negative := int64(0), int64(1), int64(-1)
	base := ArtifactFile{ID: "a", AttemptID: "att", RelativePath: "a.txt", ChangeKind: ArtifactModified, ContentDigest: "digest"}
	cases := []struct {
		name string
		file ArtifactFile
	}{
		{"missing deletion", func() ArtifactFile { f := base; f.Additions = &one; return f }()},
		{"negative", func() ArtifactFile { f := base; f.Additions = &negative; f.Deletions = &zero; return f }()},
		{"binary", func() ArtifactFile { f := base; f.IsBinary = true; f.Additions = &one; f.Deletions = &zero; return f }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.file.Validate(); err == nil {
				t.Fatal("invalid measurement accepted")
			}
		})
	}
	valid := base
	valid.Additions = &one
	valid.Deletions = &zero
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid measurement rejected: %v", err)
	}
}
