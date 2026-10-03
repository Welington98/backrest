package tasks

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/garethgeorge/backrest/gen/go/v1"
)

// CopyTask copies the snapshots of a plan from the plan's repo into the repo
// named by the plan's copy policy. It is scheduled after successful backups.
type CopyTask struct {
	BaseTask
	destRepoID string
	at         time.Time
	didRun     bool
}

func NewOneoffCopyTask(repo *v1.Repo, planID string, destRepoID string, at time.Time) Task {
	return &CopyTask{
		BaseTask: BaseTask{
			TaskType:   "copy",
			TaskName:   fmt.Sprintf("copy plan %q from repo %q to repo %q", planID, repo.Id, destRepoID),
			TaskRepo:   repo,
			TaskPlanID: planID,
		},
		destRepoID: destRepoID,
		at:         at,
	}
}

func (t *CopyTask) Next(now time.Time, runner TaskRunner) (ScheduledTask, error) {
	if t.didRun {
		return NeverScheduledTask, nil
	}
	t.didRun = true
	return ScheduledTask{
		RunAt: t.at,
		Op: &v1.Operation{
			Op: &v1.Operation_OperationCopy{
				OperationCopy: &v1.OperationCopy{DestRepo: t.destRepoID},
			},
		},
	}, nil
}

func (t *CopyTask) Run(ctx context.Context, st ScheduledTask, runner TaskRunner) error {
	op := st.Op

	notifyError := func(err error) error {
		return NotifyError(ctx, runner, t.Name(), err, v1.Hook_CONDITION_COPY_ERROR)
	}

	destRepo, err := runner.GetRepoOrchestrator(t.destRepoID)
	if err != nil {
		return notifyError(fmt.Errorf("couldn't get destination repo %q: %w", t.destRepoID, err))
	}

	if err := runner.ExecuteHooks(ctx, []v1.Hook_Condition{
		v1.Hook_CONDITION_COPY_START,
	}, HookVars{Task: t.Name()}); err != nil {
		return notifyError(fmt.Errorf("copy start hook: %w", err))
	}

	if err := destRepo.UnlockIfAutoEnabled(ctx); err != nil {
		return notifyError(fmt.Errorf("auto unlock repo %q: %w", t.destRepoID, err))
	}

	opCopy := &v1.Operation_OperationCopy{
		OperationCopy: &v1.OperationCopy{DestRepo: t.destRepoID},
	}
	op.Op = opCopy

	liveID, writer, err := runner.LogrefWriter()
	if err != nil {
		return fmt.Errorf("create logref writer: %w", err)
	}
	defer writer.Close()
	opCopy.OperationCopy.OutputLogref = liveID

	if err := runner.UpdateOperation(op); err != nil {
		return fmt.Errorf("update operation: %w", err)
	}

	if err := destRepo.CopyFrom(ctx, t.Repo(), t.PlanID(), writer); err != nil {
		runner.ExecuteHooks(ctx, []v1.Hook_Condition{
			v1.Hook_CONDITION_COPY_ERROR,
			v1.Hook_CONDITION_ANY_ERROR,
		}, HookVars{Task: t.Name(), Error: err.Error()})
		return fmt.Errorf("copy: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close logref writer: %w", err)
	}

	if err := runner.ExecuteHooks(ctx, []v1.Hook_Condition{
		v1.Hook_CONDITION_COPY_SUCCESS,
	}, HookVars{Task: t.Name()}); err != nil {
		return fmt.Errorf("execute copy end hooks: %w", err)
	}
	return nil
}
