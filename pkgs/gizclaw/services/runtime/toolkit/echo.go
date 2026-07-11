package toolkit

import "context"

const EchoExecutorName = "toolkit.echo"

// EchoExecutor returns the call arguments as the result payload.
type EchoExecutor struct{}

func (EchoExecutor) Invoke(_ context.Context, call Call) (Result, error) {
	return Result{Data: call.Args}, nil
}
