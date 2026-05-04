// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"github.com/draganm/openbrine/internal/encryption/keyprovider/aws_kms"
	"github.com/draganm/openbrine/internal/encryption/keyprovider/azure_vault"
	externalKeyProvider "github.com/draganm/openbrine/internal/encryption/keyprovider/external"
	"github.com/draganm/openbrine/internal/encryption/keyprovider/gcp_kms"
	"github.com/draganm/openbrine/internal/encryption/keyprovider/openbao"
	"github.com/draganm/openbrine/internal/encryption/keyprovider/pbkdf2"
	"github.com/draganm/openbrine/internal/encryption/method/aesgcm"
	externalMethod "github.com/draganm/openbrine/internal/encryption/method/external"
	"github.com/draganm/openbrine/internal/encryption/method/unencrypted"
	"github.com/draganm/openbrine/internal/encryption/registry/lockingencryptionregistry"
)

var DefaultRegistry = lockingencryptionregistry.New()

func init() {
	if err := DefaultRegistry.RegisterKeyProvider(pbkdf2.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(aws_kms.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(gcp_kms.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(azure_vault.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(openbao.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterKeyProvider(externalKeyProvider.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(aesgcm.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(externalMethod.New()); err != nil {
		panic(err)
	}
	if err := DefaultRegistry.RegisterMethod(unencrypted.New()); err != nil {
		panic(err)
	}
}
