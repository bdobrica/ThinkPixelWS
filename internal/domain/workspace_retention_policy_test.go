package domain

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkspaceRetentionPolicy(t *testing.T) {
	t.Parallel()
	tenantID, _ := uuid.NewV7()
	workspaceID, _ := uuid.NewV7()
	idleTTL := 30 * 24 * time.Hour
	maximumGenerations := uint64(20)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))

	policy, err := (NewWorkspaceRetentionPolicy{
		TenantID: tenantID, WorkspaceID: workspaceID, IdleTTL: &idleTTL,
		MaximumGenerations: &maximumGenerations, LegalHold: true, LegalHoldReference: "case-123",
	}).WorkspaceRetentionPolicy(now)
	if err != nil {
		t.Fatalf("create retention policy: %v", err)
	}
	if policy.StateVersion != 1 || policy.IdleTTL == nil || *policy.IdleTTL != idleTTL {
		t.Fatalf("unexpected policy: %#v", policy)
	}
	if policy.CreatedAt.Location() != time.UTC || !policy.CreatedAt.Equal(now) || !policy.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps were not normalized to UTC: %#v", policy)
	}

	idleTTL = time.Hour
	maximumGenerations = 1
	if *policy.IdleTTL == idleTTL || *policy.MaximumGenerations == maximumGenerations {
		t.Fatal("policy retained caller-owned pointers")
	}
}

func TestWorkspaceRetentionPolicyRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tenantID, _ := uuid.NewV7()
	workspaceID, _ := uuid.NewV7()
	positive := time.Hour
	maximum := uint64(2)
	valid := NewWorkspaceRetentionPolicy{TenantID: tenantID, WorkspaceID: workspaceID, IdleTTL: &positive, MaximumGenerations: &maximum}

	tests := map[string]func(*NewWorkspaceRetentionPolicy){
		"non-v7 tenant":     func(input *NewWorkspaceRetentionPolicy) { input.TenantID = uuid.New() },
		"non-v7 Workspace":  func(input *NewWorkspaceRetentionPolicy) { input.WorkspaceID = uuid.New() },
		"zero duration":     func(input *NewWorkspaceRetentionPolicy) { value := time.Duration(0); input.DeleteAfter = &value },
		"negative duration": func(input *NewWorkspaceRetentionPolicy) { value := -time.Hour; input.ProfileRetention = &value },
		"fractional second": func(input *NewWorkspaceRetentionPolicy) {
			value := time.Second + time.Millisecond
			input.SnapshotRetention = &value
		},
		"zero maximum": func(input *NewWorkspaceRetentionPolicy) { value := uint64(0); input.MaximumGenerations = &value },
		"large maximum": func(input *NewWorkspaceRetentionPolicy) {
			value := uint64(math.MaxInt64) + 1
			input.MaximumGenerations = &value
		},
		"hold without reference": func(input *NewWorkspaceRetentionPolicy) { input.LegalHold = true },
		"reference without hold": func(input *NewWorkspaceRetentionPolicy) { input.LegalHoldReference = "case-123" },
		"invalid hold reference": func(input *NewWorkspaceRetentionPolicy) {
			input.LegalHold = true
			input.LegalHoldReference = strings.Repeat("x", maxLegalHoldReferenceLength+1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := input.WorkspaceRetentionPolicy(time.Now()); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWorkspaceRetentionPolicyValidateRejectsInvalidState(t *testing.T) {
	t.Parallel()
	tenantID, _ := uuid.NewV7()
	workspaceID, _ := uuid.NewV7()
	now := time.Now().UTC()
	policy := WorkspaceRetentionPolicy{TenantID: tenantID, WorkspaceID: workspaceID, StateVersion: 1, CreatedAt: now, UpdatedAt: now}
	policy.StateVersion = 0
	if err := policy.Validate(); err == nil {
		t.Fatal("expected zero state version to be rejected")
	}
	policy.StateVersion = 1
	policy.UpdatedAt = now.Add(-time.Second)
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid timestamps to be rejected")
	}
}
