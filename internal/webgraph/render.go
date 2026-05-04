// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
)

//go:embed embed
var assetsFS embed.FS

// Render writes a self-contained HTML document representing the given
// graph data to w. The output includes embedded CSS, the application
// JavaScript, and the third-party Cytoscape.js / Dagre / cytoscape-dagre
// libraries inlined verbatim.
//
// title is shown in the page header and the <title> element. An empty
// title falls back to "OpenTofu graph".
func Render(w io.Writer, data *GraphData, title string) error {
	if title == "" {
		title = "OpenTofu graph"
	}
	tmplBytes, err := assetsFS.ReadFile("embed/index.html.tmpl")
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	tmpl, err := template.New("index").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	styles, err := assetsFS.ReadFile("embed/styles.css")
	if err != nil {
		return fmt.Errorf("read styles: %w", err)
	}
	app, err := assetsFS.ReadFile("embed/app.js")
	if err != nil {
		return fmt.Errorf("read app.js: %w", err)
	}
	cy, err := assetsFS.ReadFile("embed/vendor/cytoscape.min.js")
	if err != nil {
		return fmt.Errorf("read cytoscape: %w", err)
	}
	dagre, err := assetsFS.ReadFile("embed/vendor/dagre.min.js")
	if err != nil {
		return fmt.Errorf("read dagre: %w", err)
	}
	cyDagre, err := assetsFS.ReadFile("embed/vendor/cytoscape-dagre.js")
	if err != nil {
		return fmt.Errorf("read cytoscape-dagre: %w", err)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal graph data: %w", err)
	}
	// Escape "</script" so the JSON payload can sit safely inside a
	// <script type="application/json"> block.
	safeJSON := strings.ReplaceAll(string(jsonBytes), "</", "<\\/")

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Title          string
		Styles         string
		App            string
		Cytoscape      string
		Dagre          string
		CytoscapeDagre string
		GraphJSON      string
	}{
		Title:          title,
		Styles:         string(styles),
		App:            string(app),
		Cytoscape:      string(cy),
		Dagre:          string(dagre),
		CytoscapeDagre: string(cyDagre),
		GraphJSON:      safeJSON,
	})
	if err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	_, err = w.Write(buf.Bytes())
	return err
}
