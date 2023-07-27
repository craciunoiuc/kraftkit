// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"context"
	"errors"

	"kraftkit.sh/pack"
	"kraftkit.sh/packmanager"
)

// push
func (opts *GithubAction) push(ctx context.Context) error {
	var pmananger packmanager.PackageManager
	if opts.Format != "auto" {
		pmananger = packmanager.PackageManagers()[pack.PackageFormat(opts.Format)]
		if pmananger == nil {
			return errors.New("invalid package format specified")
		}
	} else {
		pmananger = packmanager.G(ctx)
	}

	if pm, compatible, err := pmananger.IsCompatible(ctx, opts.Output); err == nil && compatible {
		packages, err := pm.Catalog(ctx,
			packmanager.WithCache(true),
			packmanager.WithName(opts.Output),
		)
		if err != nil {
			return err
		}

		if len(packages) == 0 {
			return errors.New("no packages found")
		} else if len(packages) > 1 {
			return errors.New("multiple packages found")
		}

		packages[0].Push(ctx)
	}
	return nil
}
