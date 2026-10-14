package activationrepo

import (
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func bound(t *testing.T, r *Repository) string {
	t.Helper()
	owner, e := r.BoundOwner(bg)
	must(t, e)
	return owner
}

func TestOwnerBindingStartsEmptyAndIsSharedAcrossConnections(t *testing.T) {
	a, b := repos(t)
	if got := bound(t, a); got != "" {
		t.Fatalf("fresh binding = %q", got)
	}
	must(t, a.BindOwner(bg, "owner-a"))
	must(t, a.BindOwner(bg, "owner-a")) // idempotent
	if got := bound(t, b); got != "owner-a" {
		t.Fatalf("other connection sees %q", got)
	}
	want(t, a.BindOwner(bg, ""), domain.ErrDeviceBridgeInvalid)
}

func TestOwnerBindingNeverOrphansAPendingOrPairedIdentity(t *testing.T) {
	a, b := repos(t)
	must(t, a.BindOwner(bg, "owner-a"))
	reserve(t, a, "owner-a")
	want(t, b.BindOwner(bg, "owner-b"), ErrReservationActive)
	if got := bound(t, a); got != "owner-a" {
		t.Fatalf("pending owner rebound to %q", got)
	}

	c, _ := repos(t)
	must(t, c.BindOwner(bg, "owner-a"))
	d := seed(t, c, "owner-a")
	want(t, c.BindOwner(bg, "owner-b"), ErrReservationActive)
	must(t, c.Revoke(bg, "owner-a", d.DeviceID))
	must(t, c.BindOwner(bg, "owner-b"))
	if got := bound(t, c); got != "owner-b" {
		t.Fatalf("revoked owner not replaced: %q", got)
	}
	// The revoked owner's evidence is retained under its own key.
	if v := load(t, c, "owner-a"); v.Device.DeviceID != d.DeviceID {
		t.Fatalf("revoked evidence lost: %+v", v)
	}
}

func TestOwnerBindingMayMoveBeforeAnyReservation(t *testing.T) {
	a, _ := repos(t)
	must(t, a.BindOwner(bg, "owner-a"))
	must(t, a.BindOwner(bg, "owner-b"))
	if got := bound(t, a); got != "owner-b" {
		t.Fatalf("binding = %q", got)
	}
}
