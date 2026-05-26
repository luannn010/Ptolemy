package policy

import "testing"

func TestApprovals_ApproveRequiresPark(t *testing.T) {
	a := NewApprovals()
	if a.Approve("never-parked") {
		t.Fatalf("Approve must return false for an unparked id")
	}
}

func TestApprovals_ParkApproveConsume(t *testing.T) {
	a := NewApprovals()
	a.Park("id-1")
	if !a.Approve("id-1") {
		t.Fatalf("Approve should succeed for a parked id")
	}
	if !a.ConsumeApproved("id-1") {
		t.Fatalf("ConsumeApproved should succeed once after Approve")
	}
}

func TestApprovals_ConsumeIsSingleUse(t *testing.T) {
	a := NewApprovals()
	a.Park("id-2")
	a.Approve("id-2")
	if !a.ConsumeApproved("id-2") {
		t.Fatalf("first ConsumeApproved must succeed")
	}
	if a.ConsumeApproved("id-2") {
		t.Fatalf("second ConsumeApproved must fail; token is single-use")
	}
}

func TestApprovals_ConsumeWithoutApproveFails(t *testing.T) {
	a := NewApprovals()
	a.Park("id-3")
	if a.ConsumeApproved("id-3") {
		t.Fatalf("ConsumeApproved must fail when Approve has not run")
	}
}

func TestApprovals_DistinctIdsIndependent(t *testing.T) {
	a := NewApprovals()
	a.Park("a")
	a.Park("b")
	a.Approve("a")
	if a.ConsumeApproved("b") {
		t.Fatalf("approving a must not approve b")
	}
	if !a.ConsumeApproved("a") {
		t.Fatalf("a should still be consumable")
	}
}
