package kafka

import (
	"errors"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
)

const defaultOperationTimeout = 10000

func (c *Client) CreateTopic(name string, parts int32, replicas int16, config map[string]*string, update bool) error {
	admin := kadm.NewClient(c.client)
	admin.SetTimeoutMillis(defaultOperationTimeout)

	_, err := admin.CreateTopic(c.client.Context(), parts, replicas, config, name)
	if errors.Is(err, kerr.TopicAlreadyExists) && update {
		return c.AlterTopicConfig(name, config)
	}

	return err
}

func (c *Client) AlterTopicConfig(name string, config map[string]*string) error {
	admin := kadm.NewClient(c.client)
	alters := make([]kadm.AlterConfig, 0)
	for k, v := range config {
		alter := kadm.AlterConfig{
			Op:    kadm.SetConfig,
			Name:  k,
			Value: v,
		}

		alters = append(alters, alter)
	}

	_, err := admin.AlterTopicConfigs(c.client.Context(), alters, name)

	return err
}
