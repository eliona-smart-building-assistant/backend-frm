package tenants

import (
	"slices"
	"sync"
	"testing"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/kafka"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"
)

// transitionRecorder records the order of OnTenantOwned/OnTenantDisowned
// callback invocations.
type transitionRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *transitionRecorder) owned(tenantId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "+ "+tenantId)
}

func (r *transitionRecorder) disowned(tenantId string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, "- "+tenantId)
}

func (r *transitionRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func newTestManager(t *testing.T, partitionCount int) (*TenantManager, *transitionRecorder) {
	t.Helper()

	rec := &transitionRecorder{}
	m, err := NewTenantManager(log.NoopLogger(), "test-topic", TenantManagerConfig{
		PartitionCount:   partitionCount,
		OnTenantOwned:    rec.owned,
		OnTenantDisowned: rec.disowned,
	})
	if err != nil {
		t.Fatalf("NewTenantManager: %v", err)
	}

	return m, rec
}

// tenantsOnPartition returns the tenants whose key maps to partition p for
// the given partition count, mirroring the manager's partition calculator.
func tenantsOnPartition(partitionCount int, p int, tenants ...string) []string {
	pc := kafka.NewPartitionCalculator("test-topic", partitionCount)

	var result []string
	for _, tenant := range tenants {
		if pc.PartitionForKey([]byte(tenant)) == p {
			result = append(result, tenant)
		}
	}

	return result
}

func TestNewTenantManagerValidation(t *testing.T) {
	newCfg := func() TenantManagerConfig {
		return TenantManagerConfig{
			PartitionCount:   1,
			OnTenantOwned:    func(string) {},
			OnTenantDisowned: func(string) {},
		}
	}

	if _, err := NewTenantManager(log.NoopLogger(), "t", newCfg()); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	cfg := newCfg()
	cfg.PartitionCount = 0
	if _, err := NewTenantManager(log.NoopLogger(), "t", cfg); err == nil {
		t.Error("partition count 0 accepted")
	}

	cfg = newCfg()
	cfg.PartitionCount = -3
	if _, err := NewTenantManager(log.NoopLogger(), "t", cfg); err == nil {
		t.Error("negative partition count accepted")
	}

	cfg = newCfg()
	cfg.OnTenantOwned = nil
	if _, err := NewTenantManager(log.NoopLogger(), "t", cfg); err == nil {
		t.Error("missing OnTenantOwned accepted")
	}

	cfg = newCfg()
	cfg.OnTenantDisowned = nil
	if _, err := NewTenantManager(log.NoopLogger(), "t", cfg); err == nil {
		t.Error("missing OnTenantDisowned accepted")
	}
}

func TestPartitionEventIgnoresOtherTopics(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "other-topic", []int{0, 1, 2, 3})

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected no callbacks for other topic, got %v", calls)
	}
}

func TestAddTenantBeforeAssignment(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Fatalf("no callbacks expected before assignment, got %v", calls)
	}

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	if calls := rec.snapshot(); !slices.Equal(calls, []string{"+ tenant-a"}) {
		t.Errorf("expected exactly one owned callback on assignment, got %v", calls)
	}
}

func TestAddTenantAfterAssignment(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	m.AddTenant("tenant-a")

	if calls := rec.snapshot(); !slices.Equal(calls, []string{"+ tenant-a"}) {
		t.Errorf("expected exactly one owned callback, got %v", calls)
	}
}

// Duplicate AddTenant calls during service initialization must result in a
// single OnTenantOwned callback.
func TestAddTenantDuplicateCalls(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	m.AddTenant("tenant-a")
	m.AddTenant("tenant-a")
	m.AddTenant("tenant-a")

	if calls := rec.snapshot(); !slices.Equal(calls, []string{"+ tenant-a"}) {
		t.Errorf("expected single owned callback for duplicate adds, got %v", calls)
	}
}

func TestRevokeReassignCycle(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})
	m.OnPartitionEventBulk(kafka.Partitionrevoked, "test-topic", []int{0, 1, 2, 3})
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	expected := []string{"+ tenant-a", "- tenant-a", "+ tenant-a"}
	if calls := rec.snapshot(); !slices.Equal(calls, expected) {
		t.Errorf("expected %v, got %v", expected, calls)
	}
}

func TestPartitionLostMatchesRevoked(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})
	m.OnPartitionEventBulk(kafka.PartitionLost, "test-topic", []int{0, 1, 2, 3})

	expected := []string{"+ tenant-a", "- tenant-a"}
	if calls := rec.snapshot(); !slices.Equal(calls, expected) {
		t.Errorf("expected %v, got %v", expected, calls)
	}
}

// A tenant whose partition is not among the assigned ones must not produce
// callbacks.
func TestUnaffectedPartitions(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	p := m.partCalc.PartitionForKey([]byte("tenant-a"))
	other := []int{0, 1, 2, 3}
	other = slices.DeleteFunc(other, func(n int) bool { return n == p })

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", other)

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected no callbacks for unrelated partitions, got %v", calls)
	}
}

// Duplicate assignment of an already-owned partition must not re-fire
// callbacks.
func TestDuplicateAssignment(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	if calls := rec.snapshot(); !slices.Equal(calls, []string{"+ tenant-a"}) {
		t.Errorf("expected single owned callback, got %v", calls)
	}
}

// Removing a tenant that was never owned must not fire OnTenantDisowned.
func TestRemoveNeverOwnedTenant(t *testing.T) {
	m, rec := newTestManager(t, 4)

	m.RemoveTenant("tenant-a")

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected no callbacks, got %v", calls)
	}
}

// Removing a tenant whose partition is owned by another member must not
// fire OnTenantDisowned.
func TestRemoveTenantOnUnownedPartition(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")

	p := m.partCalc.PartitionForKey([]byte("tenant-a"))
	other := []int{0, 1, 2, 3}
	other = slices.DeleteFunc(other, func(n int) bool { return n == p })

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", other)

	m.RemoveTenant("tenant-a")

	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected no callbacks, got %v", calls)
	}
}

func TestRemoveOwnedTenant(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	m.RemoveTenant("tenant-a")

	if calls := rec.snapshot(); !slices.Equal(calls, []string{"+ tenant-a", "- tenant-a"}) {
		t.Errorf("expected owned then disowned, got %v", calls)
	}
}

// Removing and re-adding a tenant while owning its partition must result in
// disowned + owned, not stay silent.
func TestRemoveReaddCycle(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	m.RemoveTenant("tenant-a")
	m.AddTenant("tenant-a")

	expected := []string{"+ tenant-a", "- tenant-a", "+ tenant-a"}
	if calls := rec.snapshot(); !slices.Equal(calls, expected) {
		t.Errorf("expected %v, got %v", expected, calls)
	}
}

// A tenant removed after revocation must not receive a second
// OnTenantDisowned.
func TestRemoveAfterRevokeNoDuplicateDisowned(t *testing.T) {
	m, rec := newTestManager(t, 4)
	m.AddTenant("tenant-a")
	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})
	m.OnPartitionEventBulk(kafka.Partitionrevoked, "test-topic", []int{0, 1, 2, 3})

	m.RemoveTenant("tenant-a")

	expected := []string{"+ tenant-a", "- tenant-a"}
	if calls := rec.snapshot(); !slices.Equal(calls, expected) {
		t.Errorf("expected %v, got %v", expected, calls)
	}
}

func TestManyTenantsManyPartitions(t *testing.T) {
	m, rec := newTestManager(t, 8)

	tenants := []string{
		"tenant-a", "tenant-b", "tenant-c", "tenant-d",
		"tenant-e", "tenant-f", "tenant-g", "tenant-h",
	}
	for _, tenant := range tenants {
		m.AddTenant(tenant)
	}

	m.OnPartitionEventBulk(kafka.PartitionAssigned, "test-topic", []int{0, 1, 2, 3})

	var expected []string
	for p := 0; p < 4; p++ {
		for _, tenant := range tenantsOnPartition(8, p, tenants...) {
			expected = append(expected, "+ "+tenant)
		}
	}
	slices.Sort(expected)

	calls := rec.snapshot()
	slices.Sort(calls)

	if !slices.Equal(calls, expected) {
		t.Errorf("expected %v, got %v", expected, calls)
	}
}
