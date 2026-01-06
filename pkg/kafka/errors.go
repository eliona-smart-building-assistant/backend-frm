package kafka

import (
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

type ConsumerError struct {
	err         error
	desc        string
	canContinue bool
	isInfo      bool
}

func (c ConsumerError) Error() string {
	return c.err.Error()
}

func (c ConsumerError) CanContinue() bool {
	return c.canContinue
}

func (c ConsumerError) IsInfo() bool {
	return c.isInfo
}

func (c ConsumerError) Unwrap() error {
	return c.err
}

func wrapKgoConsumerError(err error) ConsumerError {
	var (
		kgoErr             *kerr.Error
		kgoErrDataLoss     *kgo.ErrDataLoss
		kgoErrGroupSession *kgo.ErrGroupSession
	)

	if errors.As(err, &kgoErr) {
		return ConsumerError{
			err:         kgoErr,
			desc:        fmt.Sprintf("%d - %s: %s", kgoErr.Code, kgoErr.Message, kgoErr.Description),
			canContinue: false,
			isInfo:      false,
		}
	}

	if errors.As(err, &kgoErrDataLoss) {
		return ConsumerError{
			err:         kgoErrDataLoss,
			canContinue: true,
			isInfo:      true,
		}
	}

	if errors.As(err, &kgoErrGroupSession) {
		return ConsumerError{
			err:         kgoErrGroupSession,
			canContinue: false,
			isInfo:      false,
		}
	}

	return ConsumerError{
		err:         err,
		canContinue: false,
		isInfo:      false,
	}
}
