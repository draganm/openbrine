// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/draganm/openbrine/internal/grpcwrap"
	plugin "github.com/draganm/openbrine/internal/plugin6"
	simple "github.com/draganm/openbrine/internal/provider-simple-v6"
	"github.com/draganm/openbrine/internal/tfplugin6"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin6.ProviderServer {
			return grpcwrap.Provider6(simple.Provider())
		},
	})
}
