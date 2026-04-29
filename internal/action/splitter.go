package action

import "fmt"

type TaskSplitter interface {
	Split(raw string) (*TaskBatch, error)
}

type PlaceholderTaskSplitter struct{}

func (PlaceholderTaskSplitter) Split(raw string) (*TaskBatch, error) {
	return nil, fmt.Errorf("task batch splitting is not implemented")
}
