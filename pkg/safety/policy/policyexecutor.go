package policy

import (
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/safety"
	"github.com/eliona-smart-building-assistant/backend-frm/pkg/safety/common"
)

// Executor handles execution and execution results according to a policy. May contain pre-execution and
// post-execution behaviors. Each Executor makes its own determination about whether an execution result is a
// success or failure.
type Executor[R any] interface {
	// PreExecute is called before execution to return an alternative result or error, such as if execution is not allowed or
	// needed.
	PreExecute(exec ExecutionInternal[R]) *common.PolicyResult[R]

	// Apply performs an execution by calling PreExecute and returning any result, else calling the innerFn PostExecute.
	//
	// If an Executor delays or blocks during execution, it must check that the execution was not canceled in the
	// meantime, else return the ExecutionInternal.Result if it was.
	Apply(innerFn func(safety.Execution[R]) *common.PolicyResult[R]) func(safety.Execution[R]) *common.PolicyResult[R]

	// PostExecute performs synchronous post-execution handling for an execution result.
	PostExecute(exec ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R]

	// IsFailure returns whether the result is a failure according to the corresponding policy.
	IsFailure(result R, err error) bool

	// OnSuccess performs post-execution handling for a result that is considered a success according to IsFailure.
	OnSuccess(exec ExecutionInternal[R], result *common.PolicyResult[R])

	// OnFailure performs post-execution handling for a result that is considered a failure according to IsFailure, possibly
	// creating a new result, else returning the original result.
	OnFailure(exec ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R]
}

// BaseExecutor provides base implementation of Executor.
type BaseExecutor[R any] struct {
	Executor[R]
	*BaseFailurePolicy[R]
}

var _ Executor[any] = &BaseExecutor[any]{}

func (e *BaseExecutor[R]) PreExecute(_ ExecutionInternal[R]) *common.PolicyResult[R] {
	return nil
}

func (e *BaseExecutor[R]) Apply(innerFn func(safety.Execution[R]) *common.PolicyResult[R]) func(safety.Execution[R]) *common.PolicyResult[R] {
	return func(exec safety.Execution[R]) *common.PolicyResult[R] {
		execInternal := exec.(ExecutionInternal[R])
		result := e.Executor.PreExecute(execInternal)
		if result != nil {
			return result
		}

		result = innerFn(exec)
		return e.Executor.PostExecute(execInternal, result)
	}
}

func (e *BaseExecutor[R]) PostExecute(exec ExecutionInternal[R], er *common.PolicyResult[R]) *common.PolicyResult[R] {
	if e.Executor.IsFailure(er.Result, er.Error) {
		er = e.Executor.OnFailure(exec, er.WithFailure())
	} else {
		er = er.WithDone(true, true)
		e.Executor.OnSuccess(exec, er)
	}
	return er
}

func (e *BaseExecutor[R]) IsFailure(result R, err error) bool {
	if e.BaseFailurePolicy != nil {
		return e.BaseFailurePolicy.IsFailure(result, err)
	}
	return err != nil
}

func (e *BaseExecutor[R]) OnSuccess(exec ExecutionInternal[R], result *common.PolicyResult[R]) {
	if e.BaseFailurePolicy != nil && e.onSuccess != nil {
		e.onSuccess(safety.ExecutionEvent[R]{
			ExecutionAttempt: exec.CopyWithResult(result),
		})
	}
}

func (e *BaseExecutor[R]) OnFailure(exec ExecutionInternal[R], result *common.PolicyResult[R]) *common.PolicyResult[R] {
	if e.BaseFailurePolicy != nil && e.onFailure != nil {
		e.onFailure(safety.ExecutionEvent[R]{
			ExecutionAttempt: exec.CopyWithResult(result),
		})
	}
	return result
}
