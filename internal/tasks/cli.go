package tasks

import "context"

func BuildPlanPreview(inbox string) ([]string, []ValidationError, error) {
	taskList, err := ScanInbox(inbox)
	if err != nil {
		return nil, nil, err
	}

	validationErrs := ValidateTasks(taskList)
	if len(validationErrs) > 0 {
		return nil, validationErrs, nil
	}

	plan, err := BuildExecutionPlan(taskList, map[string]bool{})
	if err != nil {
		return nil, nil, err
	}

	ids := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		ids = append(ids, step.Task.ID)
	}
	return ids, nil, nil
}

func RunInboxScheduler(ctx context.Context, inbox string, workspace string) SchedulerResult {
	return NewScheduler(inbox, workspace).Run(ctx)
}
