// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"context"

	"kraftkit.sh/pack"
	"kraftkit.sh/packmanager"
	"kraftkit.sh/unikraft"
	"kraftkit.sh/unikraft/app"
)

// pack
func (opts *GithubAction) pack(ctx context.Context) error {
	popts := []app.ProjectOption{
		app.WithProjectWorkdir(opts.Workdir),
	}

	if len(opts.Kraftfile) > 0 {
		popts = append(popts, app.WithProjectKraftfile(opts.Kraftfile))
	} else {
		popts = append(popts, app.WithProjectDefaultKraftfiles())
	}

	// Interpret the project directory
	project, err := app.NewProjectFromOptions(ctx, popts...)
	if err != nil {
		return err
	}

	// Generate a package for every matching requested target
	for _, targ := range project.Targets() {
		// See: https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable
		targ := targ

		switch true {
		case
			// If no arguments are supplied
			len(opts.Target) == 0 &&
				len(opts.Arch) == 0 &&
				len(opts.Plat) == 0,

			// If the --target flag is supplied and the target name match
			len(opts.Target) > 0 &&
				targ.Name() == opts.Target,

			// If only the --arch flag is supplied and the target's arch matches
			len(opts.Arch) > 0 &&
				len(opts.Plat) == 0 &&
				targ.Architecture().Name() == opts.Arch,

			// If only the --plat flag is supplied and the target's platform matches
			len(opts.Plat) > 0 &&
				len(opts.Arch) == 0 &&
				targ.Platform().Name() == opts.Plat,

			// If both the --arch and --plat flag are supplied and match the target
			len(opts.Plat) > 0 &&
				len(opts.Arch) > 0 &&
				targ.Architecture().Name() == opts.Arch &&
				targ.Platform().Name() == opts.Plat:

			var format pack.PackageFormat
			if opts.Format != "auto" {
				format = pack.PackageFormat(opts.Format)
			} else if targ.Format().String() != "" {
				format = targ.Format()
			}

			pm := packmanager.G(ctx)

			// Switch the package manager the desired format for this target
			if format != "auto" {
				pm, err = pm.From(format)
				if err != nil {
					return err
				}
			}

			popts := []packmanager.PackOption{
				packmanager.PackInitrd(opts.InitRd),
				packmanager.PackKConfig(opts.Kconfig),
				packmanager.PackName(opts.Name),
				packmanager.PackOutput(opts.Output),
			}

			if ukversion, ok := targ.KConfig().Get(unikraft.UK_FULLVERSION); ok {
				popts = append(popts,
					packmanager.PackWithKernelVersion(ukversion.Value),
				)
			}

			if _, err := pm.Pack(ctx, targ, popts...); err != nil {
				return err
			}

			return nil
		}
	}

	return nil
}
