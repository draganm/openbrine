"use strict";

(function () {
  const ROOT_MODULE_ID = "__root__";

  const data = JSON.parse(document.getElementById("graph-data").textContent);

  const elements = buildElements(data);

  cytoscape.use(cytoscapeDagre);

  const cy = cytoscape({
    container: document.getElementById("cy"),
    elements,
    wheelSensitivity: 0.2,
    minZoom: 0.1,
    maxZoom: 4,
    style: cytoscapeStyles(),
    layout: dagreLayout(),
  });

  cy.on("tap", "node", (e) => {
    const n = e.target;
    if (n.data("kindCategory") === "module") return;
    showNodePanel(n.data());
  });

  cy.on("tap", "edge", (e) => showEdgePanel(e.target.data()));

  cy.on("tap", (e) => {
    if (e.target === cy) closePanel();
  });

  document.getElementById("panel-close").addEventListener("click", closePanel);

  document.getElementById("search").addEventListener("input", (e) => {
    const q = e.target.value.trim().toLowerCase();
    cy.batch(() => {
      cy.elements().removeClass("dim").removeClass("hit");
      if (!q) return;
      const hits = cy.nodes().filter(
        (n) =>
          n.data("kindCategory") !== "module" &&
          ((n.data("label") || "").toLowerCase().includes(q) ||
            (n.data("id") || "").toLowerCase().includes(q))
      );
      if (hits.length === 0) return;
      cy.elements().addClass("dim");
      hits.removeClass("dim").addClass("hit");
      hits.connectedEdges().removeClass("dim");
      hits.neighborhood("node").removeClass("dim");
    });
  });

  document.getElementById("hide-data").addEventListener("change", (e) => {
    cy.batch(() => {
      cy.nodes('[kind = "data"]').toggleClass("hidden", e.target.checked);
      cy.edges()
        .filter((edge) => {
          const s = edge.source();
          const t = edge.target();
          return s.data("kind") === "data" || t.data("kind") === "data";
        })
        .toggleClass("hidden", e.target.checked);
    });
  });

  document.getElementById("collapse-modules").addEventListener("change", (e) => {
    if (e.target.checked) collapseAllModules();
    else expandAllModules();
    cy.layout(dagreLayout()).run();
  });

  function buildElements(d) {
    const els = [];
    const moduleIds = new Set();
    for (const m of d.modules || []) {
      const id = m.id === "" ? ROOT_MODULE_ID : m.id;
      moduleIds.add(id);
      const parent =
        m.parent === undefined || m.parent === null
          ? null
          : m.parent === ""
          ? ROOT_MODULE_ID
          : m.parent;
      if (id === ROOT_MODULE_ID) {
        // Don't render the root module as a visible compound; let its
        // children sit at the top level. We still keep the ID known so
        // parent = ROOT_MODULE_ID maps to nothing.
        continue;
      }
      els.push({
        group: "nodes",
        data: {
          id,
          label: m.label,
          kindCategory: "module",
          source: m.source || "",
          ...(parent && parent !== ROOT_MODULE_ID ? { parent } : {}),
        },
        classes: "module",
      });
    }

    for (const n of d.nodes || []) {
      const parent = n.module === "" ? null : n.module;
      els.push({
        group: "nodes",
        data: {
          id: n.id,
          label: shortLabel(n),
          kind: n.kind,
          kindCategory: "node",
          fullLabel: n.label,
          type: n.type || "",
          name: n.name,
          attributes: n.attributes || [],
          sourceFile: n.sourceFile || "",
          sourceLine: n.sourceLine || 0,
          hasState: !!n.hasState,
          ...(parent ? { parent } : {}),
        },
        classes: n.kind,
      });
    }

    let i = 0;
    for (const e of d.edges || []) {
      els.push({
        group: "edges",
        data: {
          id: "e" + i++,
          source: e.from,
          target: e.to,
          fromAttr: e.fromAttr || "",
          toAttr: e.toAttr || "",
          transform: e.transform || "",
          expr: e.expr || "",
          sourceFile: e.sourceFile || "",
          sourceLine: e.sourceLine || 0,
          edgeLabel: e.transform || "",
        },
        classes: e.transform ? "transformed" : "",
      });
    }
    return els;
  }

  function shortLabel(n) {
    if (n.kind === "managed") return `${n.type}.${n.name}`;
    if (n.kind === "data") return `data.${n.type}.${n.name}`;
    if (n.kind === "output") return `output.${n.name}`;
    if (n.kind === "variable") return `var.${n.name}`;
    return n.label || n.name;
  }

  function dagreLayout() {
    return {
      name: "dagre",
      rankDir: "TB",
      nodeSep: 30,
      rankSep: 50,
      edgeSep: 12,
      ranker: "network-simplex",
      animate: false,
      fit: true,
      padding: 24,
    };
  }

  function cytoscapeStyles() {
    return [
      {
        selector: "node",
        style: {
          "background-color": "#dde7ee",
          "border-color": "#7e8a96",
          "border-width": 1,
          shape: "round-rectangle",
          label: "data(label)",
          "font-family":
            "ui-monospace, SF Mono, Menlo, monospace",
          "font-size": 11,
          color: "#1f2328",
          "text-valign": "center",
          "text-halign": "center",
          "text-wrap": "wrap",
          "text-max-width": "180px",
          padding: "8px",
          width: "label",
          height: "label",
        },
      },
      {
        selector: 'node.module',
        style: {
          "background-color": "#f6f8fa",
          "background-opacity": 0.7,
          "border-color": "#8c959f",
          "border-width": 1,
          "border-style": "dashed",
          shape: "round-rectangle",
          label: "data(label)",
          "font-size": 12,
          "font-weight": 600,
          color: "#57606a",
          "text-valign": "top",
          "text-halign": "center",
          "text-margin-y": -4,
          padding: 18,
        },
      },
      {
        selector: 'node.managed',
        style: { "background-color": "#dbeafe", "border-color": "#2563eb" },
      },
      {
        selector: 'node.data',
        style: {
          "background-color": "#fef3c7",
          "border-color": "#b45309",
          "border-style": "dashed",
        },
      },
      {
        selector: 'node.output',
        style: {
          "background-color": "#dcfce7",
          "border-color": "#15803d",
          shape: "round-tag",
        },
      },
      {
        selector: 'node.variable',
        style: {
          "background-color": "#f3e8ff",
          "border-color": "#7c3aed",
          shape: "round-hexagon",
        },
      },
      {
        selector: 'node[?hasState]',
        style: { "border-width": 2 },
      },
      {
        selector: "edge",
        style: {
          width: 1.5,
          "line-color": "#8c959f",
          "target-arrow-color": "#8c959f",
          "target-arrow-shape": "triangle",
          "curve-style": "bezier",
          "arrow-scale": 0.9,
          label: "data(edgeLabel)",
          "font-size": 9,
          "font-family":
            "ui-monospace, SF Mono, Menlo, monospace",
          color: "#57606a",
          "text-background-color": "#fafbfc",
          "text-background-opacity": 1,
          "text-background-padding": 2,
          "text-background-shape": "round-rectangle",
        },
      },
      {
        selector: "edge.transformed",
        style: {
          "line-color": "#b45309",
          "target-arrow-color": "#b45309",
          "line-style": "dashed",
        },
      },
      {
        selector: ".dim",
        style: { opacity: 0.18 },
      },
      {
        selector: "node.hit",
        style: {
          "border-width": 3,
          "border-color": "#dc2626",
        },
      },
      {
        selector: ".hidden",
        style: { display: "none" },
      },
    ];
  }

  function showNodePanel(d) {
    const body = document.getElementById("panel-body");
    const parts = [];
    parts.push(`<h2>${escape(d.fullLabel || d.label)}</h2>`);
    parts.push(`<div class="subtitle">${escape(d.kind)}${
      d.type ? " · " + escape(d.type) : ""
    }</div>`);
    if (d.sourceFile) {
      parts.push(
        `<div class="src">${escape(d.sourceFile)}:${d.sourceLine}</div>`
      );
    }
    if (d.kind === "managed" || d.kind === "data") {
      if (!d.hasState) {
        parts.push(
          `<div class="empty">No state for this resource. Run <code>tofu apply</code> to populate attributes.</div>`
        );
      } else if (d.attributes == null) {
        parts.push(`<div class="empty">No attributes recorded.</div>`);
      } else {
        const sensSet = new Set(d.sensitive || []);
        parts.push(`<div class="tree">`);
        parts.push(renderRoot(d.attributes, sensSet));
        parts.push(`</div>`);
      }
    }
    body.innerHTML = parts.join("");
    bindTreeToggles(body);
    openPanel();
  }

  // renderRoot expands the top-level object/array into a flat list so
  // there's no chevron that would collapse the entire panel. Non-object
  // roots fall through to the regular renderer.
  function renderRoot(value, sensSet) {
    if (value && typeof value === "object" && !Array.isArray(value)) {
      const keys = Object.keys(value).sort();
      if (keys.length === 0) return `<span class="leaf-val">{}</span>`;
      return (
        `<ul class="children root">` +
        keys
          .map(
            (k) =>
              `<li><span class="key">${escape(k)}</span>: ${renderTree(
                value[k],
                k,
                sensSet,
                1
              )}</li>`
          )
          .join("") +
        `</ul>`
      );
    }
    if (Array.isArray(value)) {
      if (value.length === 0) return `<span class="leaf-val">[]</span>`;
      return (
        `<ul class="children root">` +
        value
          .map(
            (v, i) =>
              `<li><span class="key">${i}</span>: ${renderTree(
                v,
                `[${i}]`,
                sensSet,
                1
              )}</li>`
          )
          .join("") +
        `</ul>`
      );
    }
    return renderTree(value, "", sensSet, 0);
  }

  // renderTree returns HTML for a nested JSON value. Objects and arrays
  // are wrapped in a foldable that starts collapsed; click the chevron
  // to expand.
  function renderTree(value, path, sensSet, depth) {
    if (sensSet.has(path) && path !== "") {
      return `<span class="leaf-val sensitive">(sensitive)</span>`;
    }
    if (value === null) return `<span class="leaf-val null">null</span>`;
    const t = typeof value;
    if (t === "string")
      return `<span class="leaf-val string">${escape(JSON.stringify(value))}</span>`;
    if (t === "number" || t === "boolean")
      return `<span class="leaf-val ${t}">${escape(String(value))}</span>`;

    if (Array.isArray(value)) {
      if (value.length === 0) return `<span class="leaf-val">[]</span>`;
      const open = false;
      const summary = `[${value.length} ${
        value.length === 1 ? "item" : "items"
      }]`;
      const items = value
        .map((v, i) => {
          const childPath = `${path}[${i}]`;
          return `<li><span class="key">${i}</span>: ${renderTree(
            v,
            childPath,
            sensSet,
            depth + 1
          )}</li>`;
        })
        .join("");
      return foldable("array", summary, items, open);
    }

    if (t === "object") {
      const keys = Object.keys(value).sort();
      if (keys.length === 0) return `<span class="leaf-val">{}</span>`;
      const open = false;
      const summary = `{${keys.length} ${
        keys.length === 1 ? "key" : "keys"
      }}`;
      const items = keys
        .map((k) => {
          const childPath = path === "" ? k : `${path}.${k}`;
          return `<li><span class="key">${escape(k)}</span>: ${renderTree(
            value[k],
            childPath,
            sensSet,
            depth + 1
          )}</li>`;
        })
        .join("");
      return foldable("object", summary, items, open);
    }
    return `<span class="leaf-val">${escape(String(value))}</span>`;
  }

  function foldable(kind, summary, itemsHTML, openByDefault) {
    const cls = "fold " + (openByDefault ? "open" : "");
    return `<span class="${cls}"><span class="toggle" role="button" tabindex="0">${
      openByDefault ? "▾" : "▸"
    }</span><span class="summary ${kind}">${escape(
      summary
    )}</span><ul class="children">${itemsHTML}</ul></span>`;
  }

  function bindTreeToggles(root) {
    root.querySelectorAll(".fold > .toggle").forEach((t) => {
      t.addEventListener("click", (e) => {
        const fold = t.parentElement;
        const open = fold.classList.toggle("open");
        t.textContent = open ? "▾" : "▸";
        e.stopPropagation();
      });
    });
  }

  function showEdgePanel(d) {
    const body = document.getElementById("panel-body");
    const parts = [];
    parts.push(`<h2>${escape(d.source)} → ${escape(d.target)}</h2>`);
    parts.push(
      `<div class="subtitle">${
        d.transform ? "transform: " + escape(d.transform) : "direct reference"
      }</div>`
    );
    if (d.toAttr) {
      parts.push(meta("used in", d.toAttr));
    }
    if (d.fromAttr) {
      parts.push(meta("references", d.fromAttr));
    }
    if (d.sourceFile) {
      parts.push(
        `<div class="src">${escape(d.sourceFile)}:${d.sourceLine}</div>`
      );
    }
    if (d.expr) {
      parts.push(`<pre>${escape(d.expr)}</pre>`);
    }
    body.innerHTML = parts.join("");
    openPanel();
  }

  function meta(k, v) {
    return `<div class="meta-row"><div class="k">${escape(
      k
    )}</div><div class="v">${escape(v)}</div></div>`;
  }

  function openPanel() {
    document.getElementById("panel").classList.remove("hidden");
  }
  function closePanel() {
    document.getElementById("panel").classList.add("hidden");
  }

  function collapseAllModules() {
    cy.nodes("node.module").forEach((m) => {
      cy.collection(m.descendants("node")).style("display", "none");
    });
  }

  function expandAllModules() {
    cy.nodes().style("display", "element");
  }

  function escape(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, (c) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      }[c])
    );
  }

  function formatValue(v) {
    if (v == null) return "null";
    if (typeof v === "string") return JSON.stringify(v);
    if (typeof v === "number" || typeof v === "boolean") return String(v);
    return JSON.stringify(v);
  }
})();
