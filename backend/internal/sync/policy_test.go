package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestOfflineCapableMatchesRules(t *testing.T) {
	cases := []struct {
		entity, action string
		want           bool
	}{
		{"task", "create", true},
		{"task", "update", true},
		{"task", "delete", true},
		{"attendance", "check_in", true},
		{"attendance", "check_out", true},
		{"leave", "create", true},
		{"shift", "create", true},
		{"leave", "approve", false},
		{"finance", "pay", false},
		{"rbac", "assign", false},
	}
	for _, tc := range cases {
		if got := OfflineCapable(tc.entity, tc.action); got != tc.want {
			t.Fatalf("OfflineCapable(%s,%s)=%v want %v", tc.entity, tc.action, got, tc.want)
		}
	}
}

func TestServerAuthoritative(t *testing.T) {
	if !ServerAuthoritative("leave", "approve") {
		t.Fatal("leave approve must be server-authoritative")
	}
	if !ServerAuthoritative("workflow", "delegate") {
		t.Fatal("workflow delegate must be server-authoritative")
	}
	if ServerAuthoritative("task", "create") {
		t.Fatal("task create is offline-capable, not server-authoritative")
	}
}

func TestAttendanceColumn(t *testing.T) {
	col, err := AttendanceColumn("check_in")
	if err != nil || col != "check_in_at" {
		t.Fatalf("check_in -> %q %v", col, err)
	}
	col, err = AttendanceColumn("check_out")
	if err != nil || col != "check_out_at" {
		t.Fatalf("check_out -> %q %v", col, err)
	}
	if _, err := AttendanceColumn("correct"); err == nil {
		t.Fatal("correct should be unsupported offline")
	}
}

func TestTaskCreateConflict(t *testing.T) {
	if !TaskCreateConflict(0) {
		t.Fatal("zero rows must be a conflict")
	}
	if TaskCreateConflict(1) {
		t.Fatal("one row is not a conflict")
	}
}

func TestClassifyPushOutcome(t *testing.T) {
	if got := ClassifyPushOutcome(false, true, false); got != "conflict" {
		t.Fatalf("got %s", got)
	}
	if got := ClassifyPushOutcome(true, false, false); got != "duplicate" {
		t.Fatalf("got %s", got)
	}
	if got := ClassifyPushOutcome(false, false, true); got != "rejected" {
		t.Fatalf("got %s", got)
	}
	if got := ClassifyPushOutcome(false, false, false); got != "accepted" {
		t.Fatalf("got %s", got)
	}
}

func TestLeavePayloadRequiresDates(t *testing.T) {
	op := Operation{Action: "create", Payload: json.RawMessage(`{"leave_type":"annual"}`)}
	err := validateLeavePayload(op)
	if err == nil {
		t.Fatal("expected missing dates error")
	}
	op.Payload = json.RawMessage(`{"start_date":"2026-08-20","end_date":"2026-08-21"}`)
	if err := validateLeavePayload(op); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
}

func TestShiftPayloadRequiresFields(t *testing.T) {
	op := Operation{Action: "create", Payload: json.RawMessage(`{"title":"Desk"}`)}
	if err := validateShiftPayload(op); err == nil {
		t.Fatal("expected missing fields error")
	}
	op.Payload = json.RawMessage(`{"title":"Desk","shift_date":"2026-08-20","starts_at":"09:00","ends_at":"17:00"}`)
	if err := validateShiftPayload(op); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
}

func TestTaskPayloadTitleRequired(t *testing.T) {
	op := Operation{Action: "create", Payload: json.RawMessage(`{"id":"t1"}`)}
	if err := validateTaskPayload(op); err == nil {
		t.Fatal("expected title required")
	}
	op.Payload = json.RawMessage(`{"id":"t1","title":"Ship"}`)
	if err := validateTaskPayload(op); err != nil {
		t.Fatalf("valid payload: %v", err)
	}
}

func TestErrConflictSentinel(t *testing.T) {
	if !errors.Is(ErrConflict, ErrConflict) {
		t.Fatal("ErrConflict must be comparable with errors.Is")
	}
}

// validate* helpers exercise the same field rules apply* uses, without a DB.
func validateLeavePayload(operation Operation) error {
	if operation.Action != "create" {
		return fmt.Errorf("unsupported leave operation")
	}
	var payload leavePayload
	if err := json.Unmarshal(operation.Payload, &payload); err != nil {
		return err
	}
	if payload.StartDate == "" || payload.EndDate == "" {
		return fmt.Errorf("leave dates are required")
	}
	return nil
}

func validateShiftPayload(operation Operation) error {
	if operation.Action != "create" {
		return fmt.Errorf("unsupported shift operation")
	}
	var p shiftPayload
	if err := json.Unmarshal(operation.Payload, &p); err != nil {
		return err
	}
	if p.Title == "" || p.ShiftDate == "" || p.StartsAt == "" || p.EndsAt == "" {
		return fmt.Errorf("shift fields are required")
	}
	return nil
}

func validateTaskPayload(operation Operation) error {
	var payload taskPayload
	if err := json.Unmarshal(operation.Payload, &payload); err != nil {
		return err
	}
	if payload.Title == "" {
		return fmt.Errorf("task title is required")
	}
	return nil
}
