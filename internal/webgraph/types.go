// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

// Package webgraph builds an interactive HTML/SVG graph of the system
// described by an OpenTofu configuration, with attribute values overlaid
// from state.
//
// Unlike the plan/apply graph used internally by tofu, this graph is
// derived directly from the static HCL configuration: nodes correspond
// to user-authored objects (resources, data sources, module calls,
// outputs, variables) and edges correspond to references between their
// attributes. The shape of the graph reflects what a human reading the
// HCL would see, not the order of operations during a plan or apply.
package webgraph

// GraphData is the JSON payload embedded in the rendered HTML. The web
// app reads this and asks Cytoscape.js to lay it out.
type GraphData struct {
	Modules []Module `json:"modules"`
	Nodes   []Node   `json:"nodes"`
	Edges   []Edge   `json:"edges"`
}

// Module represents a static module in the configuration. Modules become
// compound (parent) nodes in the rendered graph so resources within a
// module are visually grouped.
type Module struct {
	ID     string `json:"id"`               // dot-form module path; "" for root
	Label  string `json:"label"`            // last segment, or "(root)" for root
	Parent string `json:"parent,omitempty"` // parent module ID
	Source string `json:"source,omitempty"` // module call source addr
}

// Node is a single addressable element in the graph: a resource, data
// source, output, or input variable.
type Node struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Module string `json:"module"`
	Kind   string `json:"kind"`           // managed | data | output | variable
	Type   string `json:"type,omitempty"` // e.g. aws_vpc; empty for output/variable
	Name   string `json:"name"`           // local name within module

	// Attributes is the decoded JSON tree of the resource instance's
	// current state, preserved as nested objects/arrays so the front-end
	// can render it as a foldable tree. Nil for non-resource nodes or
	// when no state is available.
	Attributes any `json:"attributes,omitempty"`

	// Sensitive is a list of dot/bracket paths into Attributes that the
	// state marks as sensitive. The front-end renders the values at
	// these paths as "(sensitive)" rather than their literal value.
	Sensitive []string `json:"sensitive,omitempty"`

	SourceFile string `json:"sourceFile,omitempty"`
	SourceLine int    `json:"sourceLine,omitempty"`
	// HasState is true iff a corresponding instance was found in state.
	// For managed/data nodes only.
	HasState bool `json:"hasState,omitempty"`
}

// Edge is a directed dependency between two nodes, labelled with the
// transformation (if any) wrapping the reference.
type Edge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	FromAttr   string `json:"fromAttr,omitempty"` // attribute path on From node (the referenced attr)
	ToAttr     string `json:"toAttr,omitempty"`   // attribute name on To node (where the ref is used)
	Transform  string `json:"transform,omitempty"`
	Expr       string `json:"expr,omitempty"`
	SourceFile string `json:"sourceFile,omitempty"`
	SourceLine int    `json:"sourceLine,omitempty"`
}
