package kafka

import "github.com/twmb/franz-go/pkg/kgo"

type PartitionCalculator struct {
	p     kgo.TopicPartitioner
	count int
}

func NewPartitionCalculator(topic string, partitions int) PartitionCalculator {
	return PartitionCalculator{
		p:     kgo.StickyKeyPartitioner(nil).ForTopic(topic),
		count: partitions,
	}
}

func (pc PartitionCalculator) PartitionForKey(key []byte) int {
	r := kgo.Record{
		Key: key,
	}

	return pc.p.Partition(&r, pc.count)
}
