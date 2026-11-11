package launcher

import (
	"context"
	"errors"
	"testing"
)

func TestLaunchWorkflowRepairsOnceAndUsesCapturedTarget(t *testing.T) {
	i := Instance{ID: 3, Revision: 2, Name: "submitted"}
	request := (LaunchRequest{Instance: i, State: State{Instances: []Instance{i}}}).Snapshot()
	installs, starts, auths := 0, 0, 0
	steps := launchSteps{
		installed: func(Instance) bool { return true },
		authorize: func(context.Context, LaunchRequest, func(Progress)) (LaunchResult, error) {
			auths++
			return LaunchResult{CredentialsChanged: true}, nil
		},
		install: func(_ context.Context, target Instance, _ string, _ func(Progress)) error {
			installs++
			if target != i {
				t.Fatal("target changed")
			}
			return nil
		},
		start: func(_ context.Context, r LaunchRequest, _ LaunchResult, _ func(Progress)) (Running, error) {
			starts++
			if r.Instance != i {
				t.Fatal("target changed")
			}
			if starts == 1 {
				return Running{}, ErrRepairRequired
			}
			return Running{PID: 42}, nil
		},
	}
	result, err := executeLaunch(t.Context(), request, nil, steps)
	if err != nil || installs != 1 || starts != 2 || auths != 1 || result.Running.PID != 42 || !result.CredentialsChanged {
		t.Fatalf("workflow: %+v %v %d %d %d", result, err, installs, starts, auths)
	}
	request.InstallOnly = true
	installs, starts, auths = 0, 0, 0
	if _, err = executeLaunch(t.Context(), request, nil, steps); err != nil || installs != 1 || starts != 0 || auths != 0 {
		t.Fatalf("install-only: %v", err)
	}
}
func TestLaunchWorkflowStopsOnAuthFailureAndCancellation(t *testing.T) {
	i := Instance{ID: 1}
	r := LaunchRequest{Instance: i, State: State{Instances: []Instance{i}}}
	failed := errors.New("account rejected")
	steps := launchSteps{
		authorize: func(context.Context, LaunchRequest, func(Progress)) (LaunchResult, error) {
			return LaunchResult{}, failed
		},
		installed: func(Instance) bool { t.Fatal("installation checked after auth failure"); return true },
	}
	result, err := executeLaunch(t.Context(), r, nil, steps)
	if !errors.Is(err, failed) || !result.AuthFailed {
		t.Fatalf("auth failure: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = executeLaunch(ctx, r, nil, steps)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	r.Instance.Revision = 9
	if _, err = executeLaunch(t.Context(), r, nil, steps); err == nil {
		t.Fatal("accepted mismatched target")
	}
}
