// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"bytes"
	"strings"
	"testing"
)

func TestRender_ProducesSelfContainedHTML(t *testing.T) {
	data := &GraphData{
		Modules: []Module{{ID: "", Label: "(root)"}},
		Nodes: []Node{
			{ID: "test_vpc.main", Label: "test_vpc.main", Kind: "managed", Type: "test_vpc", Name: "main"},
			{ID: "test_subnet.s", Label: "test_subnet.s", Kind: "managed", Type: "test_subnet", Name: "s"},
		},
		Edges: []Edge{{From: "test_vpc.main", To: "test_subnet.s", ToAttr: "vpc_id"}},
	}
	var buf bytes.Buffer
	if err := Render(&buf, data, "Test"); err != nil {
		t.Fatalf("Render: %s", err)
	}
	out := buf.String()

	for _, want := range []string{
		"<title>Test</title>",
		`id="cy"`,
		`id="graph-data"`,
		`"test_vpc.main"`,
		// Vendored libraries should be inlined.
		"cytoscape",
		"dagre",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}

	// </script must not leak through verbatim (would close the JSON block).
	if strings.Contains(out, `"</script>`) {
		t.Errorf("rendered output contains a literal </script> sequence inside JSON")
	}
}
