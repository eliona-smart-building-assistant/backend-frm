package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type MetricsCollector struct {
	statFn                  func() *pgxpool.Stat
	acquireCount            *prometheus.Desc
	acquireDuration         *prometheus.Desc
	acquiredConns           *prometheus.Desc
	canceledAcquireCount    *prometheus.Desc
	constructingConns       *prometheus.Desc
	emptyAcquireCount       *prometheus.Desc
	emptyAcquireWaitTime    *prometheus.Desc
	idleConns               *prometheus.Desc
	maxConns                *prometheus.Desc
	totalConns              *prometheus.Desc
	newConnsCount           *prometheus.Desc
	maxLifetimeDestroyCount *prometheus.Desc
	maxIdleDestroyCount     *prometheus.Desc
}

func (p *Pool) NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		statFn: p.Pool().Stat,
		acquireCount: prometheus.NewDesc(
			"pgxpool_acquire_count",
			"Cumulative count of successful acquires from the pool.",
			nil, nil),
		acquireDuration: prometheus.NewDesc(
			"pgxpool_acquire_duration_ns",
			"Total duration of all successful acquires from the pool in nanoseconds.",
			nil, nil),
		acquiredConns: prometheus.NewDesc(
			"pgxpool_acquired_conns",
			"Number of currently acquired connections in the pool.",
			nil, nil),
		canceledAcquireCount: prometheus.NewDesc(
			"pgxpool_canceled_acquire_count",
			"Cumulative count of acquires from the pool that were canceled by a context.",
			nil, nil),
		constructingConns: prometheus.NewDesc(
			"pgxpool_constructing_conns",
			"Number of conns with construction in progress in the pool.",
			nil, nil),
		emptyAcquireCount: prometheus.NewDesc(
			"pgxpool_empty_acquire",
			"Cumulative count of successful acquires from the pool that waited for a resource to be released or constructed because the pool was empty.",
			nil, nil),
		emptyAcquireWaitTime: prometheus.NewDesc(
			"pgxpool_empty_acquire_wait_time_ns",
			"Cumulative time in nanoseconds waited for successful acquires from the pool for a resource to be released or constructed because the pool was empty.",
			nil, nil),
		idleConns: prometheus.NewDesc(
			"pgxpool_idle_conns",
			"Number of currently idle conns in the pool.",
			nil, nil),
		maxConns: prometheus.NewDesc(
			"pgxpool_max_conns",
			"Maximum size of the pool.",
			nil, nil),
		totalConns: prometheus.NewDesc(
			"pgxpool_total_conns",
			"Total number of resources currently in the pool. The value is the sum of ConstructingConns, AcquiredConns, and IdleConns.",
			nil, nil),
		newConnsCount: prometheus.NewDesc(
			"pgxpool_new_conns_count",
			"Cumulative count of new connections opened.",
			nil, nil),
		maxLifetimeDestroyCount: prometheus.NewDesc(
			"pgxpool_max_lifetime_destroy_count",
			"Cumulative count of connections destroyed because they exceeded MaxConnLifetime.",
			nil, nil),
		maxIdleDestroyCount: prometheus.NewDesc(
			"pgxpool_max_idle_destroy_count",
			"Cumulative count of connections destroyed because they exceeded MaxConnIdleTime.",
			nil, nil),
	}
}

func (m *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *MetricsCollector) Collect(ch chan<- prometheus.Metric) {
	stat := m.statFn()

	ch <- prometheus.MustNewConstMetric(
		m.acquireCount,
		prometheus.CounterValue,
		float64(stat.AcquireCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.acquireDuration,
		prometheus.CounterValue,
		float64(stat.AcquireDuration()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.acquiredConns,
		prometheus.GaugeValue,
		float64(stat.AcquiredConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.canceledAcquireCount,
		prometheus.CounterValue,
		float64(stat.CanceledAcquireCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.constructingConns,
		prometheus.GaugeValue,
		float64(stat.ConstructingConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.emptyAcquireCount,
		prometheus.CounterValue,
		float64(stat.EmptyAcquireCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.emptyAcquireWaitTime,
		prometheus.CounterValue,
		float64(stat.EmptyAcquireWaitTime()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.idleConns,
		prometheus.GaugeValue,
		float64(stat.IdleConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.maxConns,
		prometheus.GaugeValue,
		float64(stat.MaxConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.totalConns,
		prometheus.GaugeValue,
		float64(stat.TotalConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.newConnsCount,
		prometheus.CounterValue,
		float64(stat.NewConnsCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.maxLifetimeDestroyCount,
		prometheus.CounterValue,
		float64(stat.MaxLifetimeDestroyCount()),
	)
	ch <- prometheus.MustNewConstMetric(
		m.maxIdleDestroyCount,
		prometheus.CounterValue,
		float64(stat.MaxLifetimeDestroyCount()),
	)
}
