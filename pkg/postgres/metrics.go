package postgres

import (
	"bufio"
	"context"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	queryCollector          *QueryMetricsColletor
}

// NewMetricsConnector return Prometheus collector that provides all metric groups
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
		queryCollector: p.queryCollector,
	}
}

func (m *MetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
	if m.queryCollector != nil {
		m.queryCollector.Describe(ch)
	}
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
		float64(stat.MaxIdleDestroyCount()),
	)

	if m.queryCollector != nil {
		m.queryCollector.Collect(ch)
	}
}

// MARK - QueryCollector

// ContextKey represents a context key.
type ContextKey struct {
	name string
}

// String returns the context key as a string.
func (k *ContextKey) String() string {
	return k.name
}

// TraceQueryKey represents the context key of the data.
var TraceQueryKey = &ContextKey{
	name: reflect.TypeFor[TraceQueryData]().PkgPath(),
}

// TraceQueryData represents a query data
type TraceQueryData struct {
	StartedAt time.Time
	SQL       string
	Operation string
	Args      []any
}

// TraceBatchKey represents the context key of the data.
var TraceBatchKey = &ContextKey{
	name: reflect.TypeFor[TraceBatchData]().PkgPath(),
}

// TraceBatchData represents a batch data
type TraceBatchData struct {
	StartedAt time.Time
	Batch     *pgx.Batch
}

type QueryMetricsColletor struct {
	queriesTotal  *prometheus.CounterVec
	errorsTotal   *prometheus.CounterVec
	queryDuration *prometheus.HistogramVec
}

func (p *Pool) newQueryMetricsCollector(nspc string) *QueryMetricsColletor {
	labels := []string{"db_operation"}

	return &QueryMetricsColletor{
		queriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: nspc,
				Subsystem: "query",
				Name:      "total",
			},
			labels,
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: nspc,
				Subsystem: "query",
				Name:      "errors_total",
			},
			labels,
		),
		queryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: nspc,
				Subsystem: "duration",
				Name:      "duration_s",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 10},
			},
			labels,
		),
	}
}

func (q *QueryMetricsColletor) Describe(ch chan<- *prometheus.Desc) {
	q.errorsTotal.Describe(ch)
	q.queriesTotal.Describe(ch)
	q.queryDuration.Describe(ch)
}

func (q *QueryMetricsColletor) Collect(ch chan<- prometheus.Metric) {
	q.errorsTotal.Collect(ch)
	q.queriesTotal.Collect(ch)
	q.queryDuration.Collect(ch)
}

func (q *QueryMetricsColletor) TraceQueryStart(ctx context.Context, conn *pgx.Conn, args pgx.TraceQueryStartData) context.Context {
	operation := sqlOperation(args.SQL)
	lables := prometheus.Labels{
		"db_operation": operation,
	}

	q.queriesTotal.With(lables).Inc()

	return context.WithValue(
		ctx, TraceQueryKey,
		&TraceQueryData{
			StartedAt: time.Now().UTC(),
			Operation: operation,
			SQL:       args.SQL,
			Args:      args.Args,
		})
}

func (q *QueryMetricsColletor) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, args pgx.TraceQueryEndData) {
	data, ok := ctx.Value(TraceQueryKey).(*TraceQueryData)
	if !ok {
		return
	}

	labels := prometheus.Labels{
		"db_operation": data.Operation,
	}

	if args.Err != nil {
		q.errorsTotal.With(labels).Inc()
	}

	q.queryDuration.With(labels).Observe(time.Since(data.StartedAt).Seconds())
}

var (
	customNamePattern = regexp.MustCompile(`^--\s+name:\s+(\w+)`)
	keywordPattern    = regexp.MustCompile(`(?i)^\s*(SELECT|INSERT|UPDATE|DELETE|COPY|CALL|EXECUTE|BEGIN|COMMIT|ROLLBACK|CREATE|DROP|ALTER|TRUNCATE|EXPLAIN)\b`)
)

func sqlOperation(sql string) string {
	if name := customNamePattern.FindStringSubmatch(sql); len(name) == 2 {
		return name[1]
	}

	if operation := keywordPattern.FindStringSubmatch(plainQuery(sql)); len(operation) == 2 {
		return strings.ToUpper(operation[1])
	}

	return "database.query"
}

func plainQuery(query string) string {
	scanner := bufio.NewScanner(strings.NewReader(query))
	var b strings.Builder
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(text, "--"); idx == 0 {
			continue
		} else if idx > 0 {
			text = strings.TrimSpace(text[:idx])
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(text)
	}
	return b.String()
}
