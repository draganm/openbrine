// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"

	"github.com/draganm/openbrine/internal/addrs"
	"github.com/draganm/openbrine/internal/configs"
	"github.com/draganm/openbrine/internal/states"
)

func loadTestConfig(t *testing.T, root string) *configs.Config {
	t.Helper()
	parser := configs.NewParser(nil)
	mod, diags := parser.LoadConfigDir(root, configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Fatalf("LoadConfigDir(%s): %s", root, diags.Error())
	}
	versionI := 0
	cfg, diags := configs.BuildConfig(context.Background(), mod, configs.ModuleWalkerFunc(
		func(_ context.Context, req *configs.ModuleRequest) (*configs.Module, *version.Version, hcl.Diagnostics) {
			sourcePath := filepath.Join(root, req.SourceAddr.String())
			cm, cmDiags := parser.LoadConfigDir(sourcePath, req.Call)
			v, _ := version.NewVersion("1.0.0")
			versionI++
			return cm, v, cmDiags
		},
	))
	if diags.HasErrors() {
		t.Fatalf("BuildConfig(%s): %s", root, diags.Error())
	}
	return cfg
}

// findEdge returns the first edge whose ToAttr contains attrSubstr and
// whose To matches toID, or nil if none found.
func findEdge(t *testing.T, data *GraphData, fromSubstr, toSubstr, attrSubstr string) *Edge {
	t.Helper()
	for i := range data.Edges {
		e := &data.Edges[i]
		if !strings.Contains(e.From, fromSubstr) || !strings.Contains(e.To, toSubstr) {
			continue
		}
		if attrSubstr != "" && !strings.Contains(e.ToAttr, attrSubstr) {
			continue
		}
		return e
	}
	return nil
}

func findNode(t *testing.T, data *GraphData, idSubstr string) *Node {
	t.Helper()
	for i := range data.Nodes {
		n := &data.Nodes[i]
		if strings.Contains(n.ID, idSubstr) {
			return n
		}
	}
	return nil
}

func TestBuild_SimpleResources(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")
	state := states.NewState()
	data, err := Build(cfg, state)
	if err != nil {
		t.Fatalf("Build: %s", err)
	}

	// Direct reference should produce an edge with no transform.
	e := findEdge(t, data, "test_vpc.main", "test_subnet.direct", "vpc_id")
	if e == nil {
		t.Fatalf("missing edge test_vpc.main → test_subnet.direct[vpc_id]\nedges:\n%s", debugEdges(data))
	}
	if e.Transform != "" {
		t.Errorf("direct ref should have empty transform, got %q", e.Transform)
	}
}

func TestBuild_TransformDetection(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")
	data, _ := Build(cfg, states.NewState())

	e := findEdge(t, data, "test_vpc.main", "test_subnet.transformed", "cidr_block")
	if e == nil {
		t.Fatalf("missing cidrsubnet edge\nedges:\n%s", debugEdges(data))
	}
	if e.Transform != "cidrsubnet()" {
		t.Errorf("expected transform 'cidrsubnet()', got %q", e.Transform)
	}
	if !strings.Contains(e.Expr, "cidrsubnet(") {
		t.Errorf("expected Expr to include cidrsubnet call, got %q", e.Expr)
	}

	// Template interpolation should be classified as "..."
	e2 := findEdge(t, data, "test_vpc.main", "test_subnet.transformed", "name")
	if e2 == nil {
		t.Fatalf("missing template-interpolated edge")
	}
	if !strings.Contains(e2.Transform, "…") && !strings.Contains(e2.Transform, "...") {
		t.Errorf("expected template transform marker, got %q", e2.Transform)
	}
}

func TestBuild_DataAndOutput(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")
	data, _ := Build(cfg, states.NewState())

	dn := findNode(t, data, "data.test_lookup.info")
	if dn == nil {
		t.Fatalf("missing data source node")
	}
	if dn.Kind != "data" {
		t.Errorf("expected kind=data, got %q", dn.Kind)
	}

	on := findNode(t, data, "output.subnet_id")
	if on == nil {
		t.Fatalf("missing output node")
	}
	if on.Kind != "output" {
		t.Errorf("expected kind=output, got %q", on.Kind)
	}
	// Output should have an incoming edge from test_subnet.direct
	e := findEdge(t, data, "test_subnet.direct", "output.subnet_id", "")
	if e == nil {
		t.Fatalf("expected edge from test_subnet.direct → output.subnet_id\nedges:\n%s", debugEdges(data))
	}
}

func TestBuild_ModuleBoundaries(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/module")
	data, _ := Build(cfg, states.NewState())

	// Two modules: root and "net"
	if len(data.Modules) < 2 {
		t.Fatalf("expected at least 2 modules, got %d: %#v", len(data.Modules), data.Modules)
	}
	hasNet := false
	for _, m := range data.Modules {
		if m.ID == "module.net" {
			hasNet = true
		}
	}
	if !hasNet {
		t.Errorf("expected a module entry with ID=module.net, got %#v", data.Modules)
	}

	// Variable nodes inside child module
	if findNode(t, data, "module.net.var.vpc_id") == nil {
		t.Errorf("missing child variable node module.net.var.vpc_id")
	}

	// Edge: root resource → child variable (via module call)
	e := findEdge(t, data, "test_vpc.main", "module.net.var.vpc_id", "")
	if e == nil {
		t.Fatalf("expected edge from test_vpc.main → module.net.var.vpc_id\nedges:\n%s", debugEdges(data))
	}

	// Edge: child output → root output (module.net.subnet_id consumed at root)
	e = findEdge(t, data, "module.net.output.subnet_id", "output.subnet_id", "")
	if e == nil {
		t.Fatalf("expected edge from module.net.output.subnet_id → output.subnet_id\nedges:\n%s", debugEdges(data))
	}
}

func TestBuild_AttributesFromState(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")

	state := states.NewState()
	state.RootModule().SetResourceInstanceCurrent(
		addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "test_vpc", Name: "main"}.Instance(addrs.NoKey),
		&states.ResourceInstanceObjectSrc{
			Status:    states.ObjectReady,
			AttrsJSON: []byte(`{"id":"vpc-123","cidr_block":"10.0.0.0/16","tags":{"Name":"main"}}`),
		},
		addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("test"),
		},
		addrs.NoKey,
	)

	data, _ := Build(cfg, state)
	n := findNode(t, data, "test_vpc.main")
	if n == nil {
		t.Fatalf("missing test_vpc.main node")
	}
	if !n.HasState {
		t.Errorf("expected HasState=true")
	}
	obj, ok := n.Attributes.(map[string]any)
	if !ok {
		t.Fatalf("expected Attributes to be a map, got %T", n.Attributes)
	}
	if obj["id"] != "vpc-123" {
		t.Errorf("expected id=vpc-123, got %v", obj["id"])
	}
	tags, ok := obj["tags"].(map[string]any)
	if !ok {
		t.Fatalf("expected tags to be nested object, got %T", obj["tags"])
	}
	if tags["Name"] != "main" {
		t.Errorf("expected tags.Name=main, got %v", tags["Name"])
	}
}

func TestBuild_DropsForExpressionIterationVarEdges(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")
	data, _ := Build(cfg, states.NewState())
	for _, e := range data.Edges {
		// A for-expression like `for k, v in ... : k => v.id` must not
		// emit edges from/to a synthetic node named "k" or "v".
		for _, badPrefix := range []string{"k.", "v.", ".k.", ".v."} {
			if strings.Contains(e.From, badPrefix) || strings.Contains(e.To, badPrefix) {
				t.Errorf("edge references a for-expression iteration variable: %+v", e)
			}
		}
		if strings.HasSuffix(e.From, ".k") || strings.HasSuffix(e.From, ".v") {
			t.Errorf("edge From ends in iteration variable: %+v", e)
		}
		if strings.HasSuffix(e.To, ".k") || strings.HasSuffix(e.To, ".v") {
			t.Errorf("edge To ends in iteration variable: %+v", e)
		}
	}
	// All edges' endpoints must correspond to nodes we actually emitted.
	nodes := map[string]struct{}{}
	for _, n := range data.Nodes {
		nodes[n.ID] = struct{}{}
	}
	for _, e := range data.Edges {
		if _, ok := nodes[e.From]; !ok {
			t.Errorf("edge From=%q has no corresponding node", e.From)
		}
		if _, ok := nodes[e.To]; !ok {
			t.Errorf("edge To=%q has no corresponding node", e.To)
		}
	}
}

func TestBuild_MissingStateForResource(t *testing.T) {
	cfg := loadTestConfig(t, "testdata/simple")
	data, _ := Build(cfg, states.NewState())
	n := findNode(t, data, "test_vpc.main")
	if n == nil {
		t.Fatalf("missing node")
	}
	if n.HasState {
		t.Errorf("expected HasState=false for un-applied resource")
	}
	if n.Attributes != nil {
		t.Errorf("expected nil attributes, got %#v", n.Attributes)
	}
}

func debugEdges(data *GraphData) string {
	var b strings.Builder
	for _, e := range data.Edges {
		b.WriteString("  ")
		b.WriteString(e.From)
		b.WriteString(" -> ")
		b.WriteString(e.To)
		if e.ToAttr != "" {
			b.WriteString(" [attr=")
			b.WriteString(e.ToAttr)
			b.WriteString("]")
		}
		if e.Transform != "" {
			b.WriteString(" [transform=")
			b.WriteString(e.Transform)
			b.WriteString("]")
		}
		b.WriteByte('\n')
	}
	return b.String()
}
