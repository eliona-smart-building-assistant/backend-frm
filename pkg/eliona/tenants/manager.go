package tenants

import (
	"fmt"
	"slices"
	"sync"

	"github.com/eliona-smart-building-assistant/backend-frm/pkg/kafka"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/log"
)

type TenantManagerConfig struct {
	PartitionCount   int
	OnTenantOwned    func(tenantId string)
	OnTenantDisowned func(tenantId string)
}

type TenantManager struct {
	mu              sync.Mutex
	logger          log.Logger
	topic           string
	cfg             TenantManagerConfig
	partCalc        kafka.PartitionCalculator
	ownedPartitions []int
	tenants         map[string]struct{}

	// ownedTenants tracks tenants for which OnTenantOwned was already
	// called and OnTenantDisowned was not yet called. Callbacks fire only
	// on transitions of this set, making them exactly paired and
	// insensitive to duplicate AddTenant/RemoveTenant calls and repeated
	// partition events.
	ownedTenants map[string]struct{}
}

func NewTenantManager(logger log.Logger, topic string, cfg TenantManagerConfig) (*TenantManager, error) {
	if cfg.PartitionCount <= 1 {
		return nil, fmt.Errorf("partition count should be positive, got %d", cfg.PartitionCount)
	}

	if cfg.OnTenantOwned == nil {
		return nil, fmt.Errorf("OnTenantOwned callback is not set")
	}

	if cfg.OnTenantDisowned == nil {
		return nil, fmt.Errorf("OnTenantDisowned callback is not set")
	}

	return &TenantManager{
		logger:       logger,
		topic:        topic,
		cfg:          cfg,
		partCalc:     kafka.NewPartitionCalculator(topic, cfg.PartitionCount),
		tenants:      make(map[string]struct{}),
		ownedTenants: make(map[string]struct{}),
	}, nil
}

func (m *TenantManager) OnPartitionEventBulk(event kafka.PartitionEvent, topic string, partitions []int) {
	if topic != m.topic {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var affectedTenants []string
	for i := range partitions {
		affectedTenants = append(affectedTenants, m.findTenantsForPartition(partitions[i])...)
	}

	switch event {
	case kafka.PartitionAssigned:
		for _, p := range partitions {
			if !slices.Contains(m.ownedPartitions, p) {
				m.ownedPartitions = append(m.ownedPartitions, p)
			}
		}

		m.logger.Info().Str("topic", m.topic).Ints("partitions", partitions).Msg("partitions assigned")

		for _, tenant := range affectedTenants {
			if m.setTenantOwned(tenant) {
				m.logger.Debug().Str("tenant_id", tenant).Msg("tenant owned")
				m.cfg.OnTenantOwned(tenant)
			}
		}

	case kafka.Partitionrevoked, kafka.PartitionLost:
		for _, p := range partitions {
			m.ownedPartitions = slices.DeleteFunc(m.ownedPartitions, func(n int) bool {
				return n == p
			})
		}

		m.logger.Info().Str("topic", m.topic).Ints("partitions", partitions).Msg("partitions revoked or lost")

		for _, tenant := range affectedTenants {
			if m.setTenantDisowned(tenant) {
				m.logger.Debug().Str("tenant_id", tenant).Msg("tenant disowned")
				m.cfg.OnTenantDisowned(tenant)
			}
		}
	}
}

func (m *TenantManager) AddTenant(tenantId string) {
	m.logger.Info().Str("tenant_id", tenantId).Msg("tenant added")

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tenants[tenantId] = struct{}{}

	if !slices.Contains(m.ownedPartitions, m.partCalc.PartitionForKey([]byte(tenantId))) {
		// Partition tenant belong to is not assigned to us
		return
	}

	if m.setTenantOwned(tenantId) {
		m.cfg.OnTenantOwned(tenantId)
	}
}

func (m *TenantManager) RemoveTenant(tenantId string) {
	m.logger.Info().Str("tenant_id", tenantId).Msg("tenant removed")

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tenants, tenantId)

	// The tenant set alone is not enough to decide whether the callback is
	// due: a tenant can be removed while its partition is owned by another
	// member or before any assignment. Only the ownership transition set
	// knows if OnTenantOwned was ever delivered for this tenant.
	if m.setTenantDisowned(tenantId) {
		m.cfg.OnTenantDisowned(tenantId)
	}
}

// setTenantOwned marks the tenant as owned. It reports whether this is a
// transition, i.e. whether the OnTenantOwned callback is due.
func (m *TenantManager) setTenantOwned(tenantId string) bool {
	if _, ok := m.ownedTenants[tenantId]; ok {
		return false
	}

	m.ownedTenants[tenantId] = struct{}{}
	return true
}

// setTenantDisowned marks the tenant as no longer owned. It reports whether
// this is a transition, i.e. whether the OnTenantDisowned callback is due.
func (m *TenantManager) setTenantDisowned(tenantId string) bool {
	if _, ok := m.ownedTenants[tenantId]; !ok {
		return false
	}

	delete(m.ownedTenants, tenantId)
	return true
}

func (m *TenantManager) findTenantsForPartition(p int) []string {
	var tenantdIds []string

	for tenant := range m.tenants {
		if m.partCalc.PartitionForKey([]byte(tenant)) == p {
			tenantdIds = append(tenantdIds, tenant)
		}
	}

	return tenantdIds
}
