package kafka

import (
	"github.com/twmb/franz-go/plugin/kprom"
)

func (c *Client) NewMetricsCollector() *kprom.Metrics {
	return c.metricsCollector
}

func (c *Client) newMetricsCollector() *kprom.Metrics {
	collector := kprom.NewMetrics(
		c.clientId,
	)

	c.metricsCollector = collector
	return collector
}
