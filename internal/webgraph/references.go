// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/draganm/openbrine/internal/addrs"
)

// attrRef is one reference found in a parent expression, augmented with the
// minimal context needed to render an edge: which top-level attribute it
// came from, the smallest enclosing transform expression (if any), and
// the verbatim source text of the parent expression.
type attrRef struct {
	ref       *addrs.Reference
	toAttr    string
	transform string
	exprText  string
	rng       hcl.Range
}

// extractAttrRefsFromBody walks every attribute in body and returns a flat
// list of references along with their attribute name, transform label, and
// expression text. Bodies that aren't hclsyntax (e.g. JSON) fall back to a
// reference-only walk with no transform information.
func extractAttrRefsFromBody(body hcl.Body, srcs *sourceCache) []attrRef {
	if body == nil {
		return nil
	}

	syn, ok := body.(*hclsyntax.Body)
	if !ok {
		return extractAttrRefsFallback(body, srcs)
	}

	var out []attrRef
	for _, attr := range syn.Attributes {
		out = append(out, extractAttrRefsFromExpr(attr.Name, attr.Expr, srcs)...)
	}
	for _, block := range syn.Blocks {
		// Resource configuration may contain nested blocks (e.g. "tags" rendered
		// as a block by some providers). Recurse, but use the outer block's
		// type as the attribute name so the user can see where in the resource
		// the reference lives.
		for _, attr := range block.Body.Attributes {
			out = append(out, extractAttrRefsFromExpr(block.Type+"."+attr.Name, attr.Expr, srcs)...)
		}
	}
	return out
}

func extractAttrRefsFromExpr(attrName string, rootExpr hcl.Expression, srcs *sourceCache) []attrRef {
	syntaxExpr, ok := rootExpr.(hclsyntax.Expression)
	if !ok {
		return nil
	}

	v := &refVisitor{
		attrName: attrName,
		srcs:     srcs,
	}
	hclsyntax.Walk(syntaxExpr, v)
	return v.out
}

// extractAttrRefsFromExprPublic is the entry point used for non-block
// expressions like Output.Expr. It returns refs labelled with the given
// attrName.
func extractAttrRefsFromExprPublic(attrName string, expr hcl.Expression, srcs *sourceCache) []attrRef {
	if expr == nil {
		return nil
	}
	return extractAttrRefsFromExpr(attrName, expr, srcs)
}

type refVisitor struct {
	attrName string
	srcs     *sourceCache
	stack    []hclsyntax.Node
	out      []attrRef
}

func (v *refVisitor) Enter(node hclsyntax.Node) hcl.Diagnostics {
	v.stack = append(v.stack, node)
	if st, ok := node.(*hclsyntax.ScopeTraversalExpr); ok {
		ref, diags := addrs.ParseRef(st.Traversal)
		if ref == nil || diags.HasErrors() {
			return nil
		}
		// Skip references to local values, count/each, path/terraform/tofu, self.
		// Edges to those have no useful target in the rendered graph.
		switch ref.Subject.(type) {
		case addrs.LocalValue, addrs.CountAttr, addrs.ForEachAttr,
			addrs.PathAttr, addrs.TerraformAttr:
			return nil
		}
		transform, parentExpr := v.classifyTransform(st)
		exprText := ""
		if parentExpr != nil {
			exprText = v.srcs.slice(parentExpr.Range())
		}
		v.out = append(v.out, attrRef{
			ref:       ref,
			toAttr:    v.attrName,
			transform: transform,
			exprText:  exprText,
			rng:       st.Range(),
		})
	}
	return nil
}

func (v *refVisitor) Exit(node hclsyntax.Node) hcl.Diagnostics {
	if len(v.stack) > 0 {
		v.stack = v.stack[:len(v.stack)-1]
	}
	return nil
}

// classifyTransform walks back up the stack from the current ScopeTraversalExpr
// and returns the label for the smallest enclosing transform-bearing
// expression, plus the corresponding hclsyntax.Expression for source text
// extraction. If no transform applies (the reference appears bare, or only
// inside object/tuple constructors that pass it through unchanged), it
// returns "" and the ScopeTraversalExpr itself.
func (v *refVisitor) classifyTransform(self *hclsyntax.ScopeTraversalExpr) (string, hclsyntax.Expression) {
	for i := len(v.stack) - 2; i >= 0; i-- {
		switch p := v.stack[i].(type) {
		case *hclsyntax.FunctionCallExpr:
			return p.Name + "()", p
		case *hclsyntax.TemplateExpr:
			// A LiteralValueExpr-only template is just a string literal; the
			// ScopeTraversalExpr-only template is a bare reference. The
			// interesting case is when the template mixes literals with
			// interpolations.
			if len(p.Parts) > 1 {
				return "\"…\"", p
			}
			// Single-part templates are transparent; keep walking.
		case *hclsyntax.TemplateWrapExpr:
			// A bare ${ref} interpolation; transparent.
		case *hclsyntax.BinaryOpExpr:
			return binaryOpSymbol(p), p
		case *hclsyntax.UnaryOpExpr:
			return unaryOpSymbol(p), p
		case *hclsyntax.ConditionalExpr:
			return "? :", p
		case *hclsyntax.ForExpr:
			return "for", p
		case *hclsyntax.SplatExpr:
			return "[*]", p
		case *hclsyntax.IndexExpr:
			return "[…]", p
		case *hclsyntax.RelativeTraversalExpr:
			// Wraps a sub-expression in a further traversal; treat as transparent.
		case *hclsyntax.ObjectConsExpr, *hclsyntax.TupleConsExpr,
			*hclsyntax.ObjectConsKeyExpr:
			// Containers that pass references through unchanged. Continue
			// walking up to find a more interesting wrapper.
		case *hclsyntax.ParenthesesExpr:
			// Transparent.
		}
	}
	return "", self
}

func binaryOpSymbol(e *hclsyntax.BinaryOpExpr) string {
	switch e.Op {
	case hclsyntax.OpLogicalOr:
		return "||"
	case hclsyntax.OpLogicalAnd:
		return "&&"
	case hclsyntax.OpEqual:
		return "=="
	case hclsyntax.OpNotEqual:
		return "!="
	case hclsyntax.OpGreaterThan:
		return ">"
	case hclsyntax.OpGreaterThanOrEqual:
		return ">="
	case hclsyntax.OpLessThan:
		return "<"
	case hclsyntax.OpLessThanOrEqual:
		return "<="
	case hclsyntax.OpAdd:
		return "+"
	case hclsyntax.OpSubtract:
		return "-"
	case hclsyntax.OpMultiply:
		return "*"
	case hclsyntax.OpDivide:
		return "/"
	case hclsyntax.OpModulo:
		return "%"
	}
	return "<op>"
}

func unaryOpSymbol(e *hclsyntax.UnaryOpExpr) string {
	switch e.Op {
	case hclsyntax.OpLogicalNot:
		return "!"
	case hclsyntax.OpNegate:
		return "-"
	}
	return "<op>"
}

// extractAttrRefsFallback handles non-hclsyntax bodies (e.g. JSON HCL) by
// using JustAttributes() and lang-style traversal extraction. Transforms
// are unavailable in this path.
func extractAttrRefsFallback(body hcl.Body, srcs *sourceCache) []attrRef {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		// JustAttributes refuses bodies that contain blocks. We can't easily
		// recover; just return what we found if anything.
		if len(attrs) == 0 {
			return nil
		}
	}
	var out []attrRef
	for name, attr := range attrs {
		traversals := attr.Expr.Variables()
		for _, t := range traversals {
			ref, refDiags := addrs.ParseRef(t)
			if ref == nil || refDiags.HasErrors() {
				continue
			}
			out = append(out, attrRef{
				ref:    ref,
				toAttr: name,
				rng:    attr.Expr.Range(),
			})
		}
	}
	return out
}

// sourceCache lazily reads source files so we can produce verbatim
// expression text for edge tooltips. Files are read at most once each.
type sourceCache struct {
	files map[string][]byte
}

func newSourceCache() *sourceCache {
	return &sourceCache{files: map[string][]byte{}}
}

// slice returns the bytes of the given range as a string. Out-of-range
// requests return an empty string rather than panicking.
func (c *sourceCache) slice(rng hcl.Range) string {
	if rng.Filename == "" || rng.Start.Byte < 0 || rng.End.Byte <= rng.Start.Byte {
		return ""
	}
	data, err := c.read(rng.Filename)
	if err != nil {
		return ""
	}
	if rng.End.Byte > len(data) {
		return ""
	}
	return string(data[rng.Start.Byte:rng.End.Byte])
}

func (c *sourceCache) read(name string) ([]byte, error) {
	if data, ok := c.files[name]; ok {
		return data, nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	c.files[name] = data
	return data, nil
}
