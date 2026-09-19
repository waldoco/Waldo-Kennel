package ports

import (
	"bytes"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"testing"
)

func TestIntakeClarificationCursorCodecKeyedAndCanonical(t *testing.T) {
	a, _ := NewIntakeClarificationCursorCodec(bytes.Repeat([]byte{1}, 32))
	b, _ := NewIntakeClarificationCursorCodec(bytes.Repeat([]byte{2}, 32))
	cur, e := a.New("i", IntakeClarificationCursorBoundary{}, IntakeClarificationCursorBoundary{Ordinal: 2, RoundID: "r2"}, 7)
	if e != nil {
		t.Fatal(e)
	}
	after, high, aw, e := a.Parse(cur, "i")
	if e != nil || after.Ordinal != 0 || high.RoundID != domain.IntakeClarificationRoundID("r2") || aw != 7 {
		t.Fatalf("parse=%+v %+v %d %v", after, high, aw, e)
	}
	if _, _, _, e := b.Parse(cur, "i"); e == nil {
		t.Fatal("different key accepted")
	}
}
func TestIntakeClarificationCursorCodecRejectsMissingKey(t *testing.T) {
	if _, e := NewIntakeClarificationCursorCodec(nil); e == nil {
		t.Fatal("missing key accepted")
	}
}
