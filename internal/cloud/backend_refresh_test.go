// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloud

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/draganm/openbrine/internal/backend"
	"github.com/draganm/openbrine/internal/command/arguments"
	"github.com/draganm/openbrine/internal/command/clistate"
	"github.com/draganm/openbrine/internal/command/views"
	"github.com/draganm/openbrine/internal/initwd"
	"github.com/draganm/openbrine/internal/plans"
	"github.com/draganm/openbrine/internal/states/statemgr"
	"github.com/draganm/openbrine/internal/terminal"
)

func testOperationRefresh(t *testing.T, configDir string) (*backend.Operation, *views.View, func(*testing.T) *terminal.TestOutput) {
	t.Helper()

	return testOperationRefreshWithTimeout(t, configDir, 0)
}

func testOperationRefreshWithTimeout(t *testing.T, configDir string, timeout time.Duration) (*backend.Operation, *views.View, func(*testing.T) *terminal.TestOutput) {
	t.Helper()

	_, configLoader := initwd.MustLoadConfigForTests(t, configDir, "tests")

	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	backendView := views.NewBackendHuman(views.NewView(streams))
	stateLockerView := backendView.StateLocker()
	operationView := views.NewOperation(arguments.ViewHuman, false, view)

	return &backend.Operation{
		ConfigDir:    configDir,
		ConfigLoader: configLoader,
		PlanRefresh:  true,
		StateLocker:  clistate.NewLocker(timeout, stateLockerView),
		Type:         backend.OperationTypeRefresh,
		View:         operationView,
	}, view, done
}

func TestCloud_refreshBasicActuallyRunsApplyRefresh(t *testing.T) {
	b, bCleanup := testBackendWithName(t)
	defer bCleanup()

	op, view, done := testOperationRefresh(t, "./testdata/refresh")
	b.View = views.NewBackendRemote(view)

	op.PlanMode = plans.RefreshOnlyMode
	op.Workspace = testBackendSingleWorkspaceName

	run, err := b.Operation(context.Background(), op)
	if err != nil {
		t.Fatalf("error starting operation: %v", err)
	}

	<-run.Done()
	voutput := done(t)
	if run.Result != backend.OperationSuccess {
		t.Fatalf("operation failed: %s", voutput.Stderr())
	}

	output := voutput.Stdout()
	if !strings.Contains(output, "Proceeding with 'tofu apply -refresh-only -auto-approve'") {
		t.Fatalf("expected TFC header in output: %s", output)
	}

	stateMgr, _ := b.StateMgr(t.Context(), testBackendSingleWorkspaceName)
	// An error suggests that the state was not unlocked after apply
	if _, err := stateMgr.Lock(t.Context(), statemgr.NewLockInfo()); err != nil {
		t.Fatalf("unexpected error locking state after apply: %s", err.Error())
	}
}
