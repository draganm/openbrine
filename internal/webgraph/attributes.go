// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package webgraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// decodeAttrs decodes the JSON-encoded attributes of a resource instance
// into a generic any tree (objects → map[string]any, arrays → []any,
// primitives → their natural Go types). Map keys aren't reordered; the
// JS tree renderer can sort on display if needed.
func decodeAttrs(rawJSON []byte) any {
	if len(rawJSON) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(rawJSON, &v); err != nil {
		return nil
	}
	return v
}

// sensitivePaths converts cty paths from the state's sensitive marks
// into the dot/bracket string form the JS front-end uses for path
// matching. Paths are sorted for stable output.
func sensitivePaths(paths []cty.Path) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		s := pathToString(p)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func pathToString(p cty.Path) string {
	var b strings.Builder
	for _, step := range p {
		switch s := step.(type) {
		case cty.GetAttrStep:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s.Name)
		case cty.IndexStep:
			switch s.Key.Type() {
			case cty.Number:
				bf := s.Key.AsBigFloat()
				idx, _ := bf.Int64()
				fmt.Fprintf(&b, "[%d]", idx)
			case cty.String:
				fmt.Fprintf(&b, "[%q]", s.Key.AsString())
			}
		}
	}
	return b.String()
}
