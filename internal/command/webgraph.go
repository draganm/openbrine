// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"

	"github.com/draganm/openbrine/internal/backend"
	"github.com/draganm/openbrine/internal/command/arguments"
	"github.com/draganm/openbrine/internal/command/views"
	"github.com/draganm/openbrine/internal/states"
	"github.com/draganm/openbrine/internal/states/statemgr"
	"github.com/draganm/openbrine/internal/tfdiags"
	"github.com/draganm/openbrine/internal/webgraph"
)

// WebGraphCommand renders a self-contained interactive HTML graph of
// the system described by the current OpenTofu configuration, with
// attribute values overlaid from state.
type WebGraphCommand struct {
	Meta
}

func (c *WebGraphCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	out, parseDiags := parseWebGraphFlags(rawArgs)
	view := views.NewWebGraph(c.View)
	if parseDiags.HasErrors() {
		view.Diagnostics(parseDiags)
		return cli.RunResultHelp
	}

	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	if pp, err := c.loadPluginPath(); err == nil {
		c.pluginPath = pp
	}

	cfg, cfgDiags := c.loadConfig(ctx, configPath)
	if cfgDiags.HasErrors() {
		view.Diagnostics(cfgDiags)
		return 1
	}

	state, stateDiags := c.loadStateForWebGraph(ctx, view)
	if stateDiags.HasErrors() {
		view.Diagnostics(stateDiags)
		return 1
	}
	if state == nil || state.Empty() {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No state available",
			"The webgraph command needs an existing state to populate resource attributes. Run `tofu apply` first, or use a backend that already has state for this workspace.",
		)))
		return 1
	}

	data, err := webgraph.Build(cfg, state)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed building graph data",
			err.Error(),
		)))
		return 1
	}

	f, err := os.Create(out)
	if err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed creating output file",
			fmt.Sprintf("Could not create %s: %s", out, err),
		)))
		return 1
	}
	defer f.Close()

	title := "OpenTofu graph: " + configPath
	if err := webgraph.Render(f, data, title); err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed rendering HTML",
			err.Error(),
		)))
		return 1
	}

	view.Output(fmt.Sprintf("Wrote graph to %s (%d nodes, %d edges)", out, len(data.Nodes), len(data.Edges)))
	return 0
}

func (c *WebGraphCommand) loadStateForWebGraph(ctx context.Context, view views.WebGraph) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		return nil, diags
	}

	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		return nil, diags
	}

	c.ignoreRemoteVersionConflict(b)

	if _, ok := b.(backend.Local); !ok {
		view.ErrorUnsupportedLocalOp()
		return nil, diags
	}

	env, err := c.Workspace(ctx)
	if err != nil {
		diags = diags.Append(fmt.Errorf("error selecting workspace: %w", err))
		return nil, diags
	}

	stateStore, err := b.StateMgr(ctx, env)
	if err != nil {
		diags = diags.Append(fmt.Errorf("failed to load state manager: %w", err))
		return nil, diags
	}
	if err := stateStore.RefreshState(context.TODO()); err != nil {
		diags = diags.Append(fmt.Errorf("failed to load state: %w", err))
		return nil, diags
	}

	stateFile := statemgr.Export(stateStore)
	if stateFile == nil {
		return nil, diags
	}
	return stateFile.State, diags
}

func parseWebGraphFlags(rawArgs []string) (string, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	fs := flag.NewFlagSet("webgraph", flag.ContinueOnError)
	fs.SetOutput(&strings.Builder{})
	out := fs.String("out", "tofu-graph.html", "path to write the HTML output to")
	if err := fs.Parse(rawArgs); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid arguments",
			err.Error(),
		))
	}
	return *out, diags
}

func (c *WebGraphCommand) Help() string {
	return strings.TrimSpace(`
Usage: tofu [global options] webgraph [options]

  Generates a single self-contained HTML file visualizing the system
  defined by the OpenTofu configuration in the current working
  directory. Resources, data sources, outputs, variables, and module
  boundaries are rendered as a navigable graph; clicking a node shows
  its attributes from state, and clicking an edge shows the HCL
  expression that produced the dependency.

Options:

  -out=path   Path to write the HTML output to. Default tofu-graph.html.
`)
}

func (c *WebGraphCommand) Synopsis() string {
	return "Generate an interactive HTML graph of the OpenTofu project"
}
