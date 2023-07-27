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
	"kraftkit.sh/unikraft/app"
)

// pack
func (opts *GithubAction) push(ctx context.Context) error {
	popts := []app.ProjectOption{
		app.WithProjectWorkdir(opts.Workdir),
	}

	if len(opts.Kraftfile) > 0 {
		popts = append(popts, app.WithProjectKraftfile(opts.Kraftfile))
	} else {
		popts = append(popts, app.WithProjectDefaultKraftfiles())
	}

	// Read the kraft yaml specification and get the target name
	project, err := app.NewProjectFromOptions(ctx, popts...)
	if err != nil {
		return err
	}

	// Get the target name
	ref := project.Name()

	var pmananger packmanager.PackageManager
	if opts.Format != "auto" {
		pmananger = packmanager.PackageManagers()[pack.PackageFormat(opts.Format)]
		if pmananger == nil {
			return errors.New("invalid package format specified")
		}
	} else {
		pmananger = packmanager.G(ctx)
	}

	if pm, compatible, err := pmananger.IsCompatible(ctx, ref); err == nil && compatible {
		packages, err := pm.Catalog(ctx,
			packmanager.WithCache(true),
			packmanager.WithName(ref),
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
