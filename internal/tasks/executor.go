package tasks

import (
	"context"
	"fmt"
)

type TaskContractExecutor func(ctx context.Context, workspace string, task Task, contract string) ([]byte, error)

var taskContractExecutor TaskContractExecutor

func SetTaskContractExecutor(executor TaskContractExecutor) func() {
	previous := taskContractExecutor
	taskContractExecutor = executor
	return func() {
		taskContractExecutor = previous
	}
}

func executeTaskContract(ctx context.Context, workspace string, task Task, contract string) ([]byte, error) {
	if taskContractExecutor == nil {
		return nil, fmt.Errorf("task contract executor is not configured")
	}
	return taskContractExecutor(ctx, workspace, task, contract)
}
