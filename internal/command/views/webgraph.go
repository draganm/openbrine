// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"github.com/draganm/openbrine/internal/tfdiags"
)

// WebGraph is the view used by the "webgraph" subcommand. It only needs
// to surface diagnostics and a one-line success message; the bulk of the
// command's output is written to a file.
type WebGraph interface {
	Diagnostics(diags tfdiags.Diagnostics)
	ErrorUnsupportedLocalOp()
	Output(line string)

	Backend() Backend
}

func NewWebGraph(view *View) WebGraph {
	return &WebGraphHuman{view: view}
}

type WebGraphHuman struct {
	view *View
}

var _ WebGraph = (*WebGraphHuman)(nil)

func (v *WebGraphHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *WebGraphHuman) ErrorUnsupportedLocalOp() {
	v.Diagnostics(tfdiags.Diagnostics{diagUnsupportedLocalOp})
}

func (v *WebGraphHuman) Output(line string) {
	_, _ = v.view.streams.Println(line)
}

func (v *WebGraphHuman) Backend() Backend {
	return &BackendHuman{view: v.view}
}
