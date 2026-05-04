// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/draganm/openbrine/internal/addrs"
	"github.com/draganm/openbrine/internal/configs"
	"github.com/draganm/openbrine/internal/states"
)

// Build produces a GraphData by walking the static configuration tree
// and overlaying attribute values from state. State must be non-nil:
// the rendered graph derives node identity from HCL but always shows
// real attribute values when available.
func Build(cfg *configs.Config, state *states.State) (*GraphData, error) {
	if cfg == nil {
		return &GraphData{}, nil
	}
	b := &builder{
		state: state,
		srcs:  newSourceCache(),
		seen:  map[string]struct{}{},
	}
	b.walkConfig(cfg)
	b.finalizeEdges()
	b.sortStable()
	return &GraphData{
		Modules: b.modules,
		Nodes:   b.nodes,
		Edges:   b.edges,
	}, nil
}

type builder struct {
	state        *states.State
	srcs         *sourceCache
	modules      []Module
	nodes        []Node
	edges        []Edge
	pendingEdges []Edge
	seen         map[string]struct{}
}

func (b *builder) walkConfig(cfg *configs.Config) {
	modID := modulePathID(cfg.Path)
	parentID := ""
	if cfg.Parent != nil {
		parentID = modulePathID(cfg.Parent.Path)
	}
	label := "(root)"
	if !cfg.Path.IsRoot() {
		label = cfg.Path[len(cfg.Path)-1]
	}
	source := ""
	if cfg.SourceAddr != nil {
		source = cfg.SourceAddr.String()
	}
	b.modules = append(b.modules, Module{
		ID:     modID,
		Label:  label,
		Parent: parentID,
		Source: source,
	})

	mod := cfg.Module
	if mod == nil {
		// Defensive — recurse anyway in case Children were populated.
		for _, child := range cfg.Children {
			b.walkConfig(child)
		}
		return
	}

	b.emitVariables(modID, mod.Variables)
	b.emitOutputs(cfg, modID, mod.Outputs)
	b.emitResources(cfg, modID, mod.ManagedResources, "managed")
	b.emitResources(cfg, modID, mod.DataResources, "data")
	b.emitModuleCalls(cfg, modID, mod.ModuleCalls)

	childKeys := make([]string, 0, len(cfg.Children))
	for k := range cfg.Children {
		childKeys = append(childKeys, k)
	}
	sort.Strings(childKeys)
	for _, k := range childKeys {
		b.walkConfig(cfg.Children[k])
	}
}

func (b *builder) emitVariables(modID string, vars map[string]*configs.Variable) {
	keys := sortedKeys(vars)
	for _, name := range keys {
		v := vars[name]
		id := varNodeID(modID, name)
		b.nodes = append(b.nodes, Node{
			ID:         id,
			Label:      "var." + name,
			Module:     modID,
			Kind:       "variable",
			Name:       name,
			SourceFile: v.DeclRange.Filename,
			SourceLine: v.DeclRange.Start.Line,
		})
		b.seen[id] = struct{}{}
	}
}

func (b *builder) emitOutputs(cfg *configs.Config, modID string, outs map[string]*configs.Output) {
	keys := sortedKeys(outs)
	for _, name := range keys {
		o := outs[name]
		id := outputNodeID(modID, name)
		b.nodes = append(b.nodes, Node{
			ID:         id,
			Label:      "output." + name,
			Module:     modID,
			Kind:       "output",
			Name:       name,
			SourceFile: o.DeclRange.Filename,
			SourceLine: o.DeclRange.Start.Line,
		})
		b.seen[id] = struct{}{}

		for _, ar := range extractAttrRefsFromExprPublic("value", o.Expr, b.srcs) {
			b.recordEdge(cfg, ar, id)
		}
	}
}

func (b *builder) emitResources(cfg *configs.Config, modID string, rs map[string]*configs.Resource, kind string) {
	keys := sortedKeys(rs)
	for _, k := range keys {
		r := rs[k]
		id := resourceNodeID(modID, r.Mode, r.Type, r.Name)
		label := r.Type + "." + r.Name
		if r.Mode == addrs.DataResourceMode {
			label = "data." + r.Type + "." + r.Name
		}
		attrs, sens, hasState := b.attributesForResource(cfg.Path, r)
		b.nodes = append(b.nodes, Node{
			ID:         id,
			Label:      label,
			Module:     modID,
			Kind:       kind,
			Type:       r.Type,
			Name:       r.Name,
			Attributes: attrs,
			Sensitive:  sens,
			SourceFile: r.DeclRange.Filename,
			SourceLine: r.DeclRange.Start.Line,
			HasState:   hasState,
		})
		b.seen[id] = struct{}{}

		for _, ar := range extractAttrRefsFromBody(r.Config, b.srcs) {
			b.recordEdge(cfg, ar, id)
		}
	}
}

func (b *builder) emitModuleCalls(cfg *configs.Config, modID string, calls map[string]*configs.ModuleCall) {
	keys := sortedKeys(calls)
	for _, name := range keys {
		mc := calls[name]
		// Module call inputs reference resources/outputs in the parent module
		// and feed values into the child module's variable nodes. Resolve the
		// child module via cfg.Children so we land on the right Variable IDs.
		child, ok := cfg.Children[name]
		if !ok {
			continue
		}
		childModID := modulePathID(child.Path)
		refs := extractAttrRefsFromBody(mc.Config, b.srcs)
		for _, ar := range refs {
			targetID := varNodeID(childModID, ar.toAttr)
			edge := b.makeEdge(cfg, ar, targetID)
			if edge != nil {
				b.pendingEdges = append(b.pendingEdges, *edge)
			}
		}
	}
}

// recordEdge resolves the reference target to a node ID and appends an
// edge if the target resolves to one of our emitted node kinds.
func (b *builder) recordEdge(cfg *configs.Config, ar attrRef, toID string) {
	edge := b.makeEdge(cfg, ar, toID)
	if edge != nil {
		b.pendingEdges = append(b.pendingEdges, *edge)
	}
}

func (b *builder) makeEdge(cfg *configs.Config, ar attrRef, toID string) *Edge {
	fromID, fromAttr, ok := b.resolveSubject(cfg, ar.ref)
	if !ok {
		return nil
	}
	return &Edge{
		From:       fromID,
		To:         toID,
		FromAttr:   fromAttr,
		ToAttr:     ar.toAttr,
		Transform:  ar.transform,
		Expr:       ar.exprText,
		SourceFile: ar.rng.Filename,
		SourceLine: ar.rng.Start.Line,
	}
}

// finalizeEdges drops any edge whose endpoints don't match nodes we
// actually emitted. This filters out spurious edges produced when
// addrs.ParseRef misinterprets for-expression iteration variables (and
// other non-referenceable traversals) as resource references.
func (b *builder) finalizeEdges() {
	for _, e := range b.pendingEdges {
		if _, fromOK := b.seen[e.From]; !fromOK {
			continue
		}
		if _, toOK := b.seen[e.To]; !toOK {
			continue
		}
		b.edges = append(b.edges, e)
	}
	b.pendingEdges = nil
}

// resolveSubject maps an addrs.Reference (interpreted in the context of
// cfg's module path) to one of our node IDs. The returned attribute path
// represents the portion of the reference's traversal that drills into
// the target's value (e.g. "id" or "tags[0]"); empty if the reference
// targets the object as a whole.
func (b *builder) resolveSubject(cfg *configs.Config, ref *addrs.Reference) (string, string, bool) {
	modID := modulePathID(cfg.Path)
	switch s := ref.Subject.(type) {
	case addrs.Resource:
		return resourceNodeID(modID, s.Mode, s.Type, s.Name), traversalAttrPath(ref.Remaining), true
	case addrs.ResourceInstance:
		return resourceNodeID(modID, s.Resource.Mode, s.Resource.Type, s.Resource.Name), traversalAttrPath(ref.Remaining), true
	case addrs.InputVariable:
		return varNodeID(modID, s.Name), "", true
	case addrs.OutputValue:
		return outputNodeID(modID, s.Name), "", true
	case addrs.ModuleCallInstanceOutput:
		// `module.foo.bar`: target is the output named `bar` inside the
		// child module.
		childPath := append(append(addrs.Module{}, cfg.Path...), s.Call.Call.Name)
		return outputNodeID(modulePathID(childPath), s.Name), "", true
	}
	return "", "", false
}

// attributesForResource looks up the no-key instance for r in state and
// returns its decoded attribute tree along with the list of sensitive
// dot/bracket paths. hasState=false (with nil values) when the resource
// has not yet been applied or the state has no record for it.
func (b *builder) attributesForResource(modPath addrs.Module, r *configs.Resource) (any, []string, bool) {
	if b.state == nil {
		return nil, nil, false
	}
	abs := addrs.AbsResource{
		Module: modPath.UnkeyedInstanceShim(),
		Resource: addrs.Resource{
			Mode: r.Mode,
			Type: r.Type,
			Name: r.Name,
		},
	}
	rs := b.state.Resource(abs)
	if rs == nil {
		return nil, nil, false
	}
	// Prefer the no-key instance. If only keyed instances exist, take the
	// first (sorted) so we still show something useful.
	var inst *states.ResourceInstance
	if i, ok := rs.Instances[addrs.NoKey]; ok {
		inst = i
	} else {
		var keys []string
		byKey := map[string]*states.ResourceInstance{}
		for k, i := range rs.Instances {
			ks := k.String()
			keys = append(keys, ks)
			byKey[ks] = i
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			inst = byKey[keys[0]]
		}
	}
	if inst == nil || inst.Current == nil {
		return nil, nil, false
	}
	sensPaths := make([]cty.Path, 0, len(inst.Current.AttrSensitivePaths))
	for _, pvm := range inst.Current.AttrSensitivePaths {
		sensPaths = append(sensPaths, pvm.Path)
	}
	return decodeAttrs(inst.Current.AttrsJSON), sensitivePaths(sensPaths), true
}

func (b *builder) sortStable() {
	sort.SliceStable(b.modules, func(i, j int) bool { return b.modules[i].ID < b.modules[j].ID })
	sort.SliceStable(b.nodes, func(i, j int) bool { return b.nodes[i].ID < b.nodes[j].ID })
	sort.SliceStable(b.edges, func(i, j int) bool {
		if b.edges[i].From != b.edges[j].From {
			return b.edges[i].From < b.edges[j].From
		}
		if b.edges[i].To != b.edges[j].To {
			return b.edges[i].To < b.edges[j].To
		}
		return b.edges[i].ToAttr < b.edges[j].ToAttr
	})
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// modulePathID returns the dot-form of a static module path. The root
// module has ID "" so we can detect it explicitly when wiring up the
// Cytoscape compound-node parent field.
func modulePathID(p addrs.Module) string {
	return p.String()
}

func resourceNodeID(modID string, mode addrs.ResourceMode, typ, name string) string {
	prefix := ""
	if modID != "" {
		prefix = modID + "."
	}
	if mode == addrs.DataResourceMode {
		return prefix + "data." + typ + "." + name
	}
	return prefix + typ + "." + name
}

func varNodeID(modID, name string) string {
	if modID == "" {
		return "var." + name
	}
	return modID + ".var." + name
}

func outputNodeID(modID, name string) string {
	if modID == "" {
		return "output." + name
	}
	return modID + ".output." + name
}

// traversalAttrPath renders the trailing portion of an addrs.Reference
// (after the subject) as a string like "id" or "tags.Name".
func traversalAttrPath(rem hcl.Traversal) string {
	var b strings.Builder
	for _, t := range rem {
		switch tt := t.(type) {
		case hcl.TraverseAttr:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(tt.Name)
		case hcl.TraverseIndex:
			switch tt.Key.Type() {
			case cty.Number:
				bf := tt.Key.AsBigFloat()
				idx, _ := bf.Int64()
				fmt.Fprintf(&b, "[%d]", idx)
			case cty.String:
				fmt.Fprintf(&b, "[%q]", tt.Key.AsString())
			}
		}
	}
	return b.String()
}
