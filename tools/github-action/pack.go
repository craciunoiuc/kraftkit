// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"context"
	"strings"

	"kraftkit.sh/pack"
	"kraftkit.sh/packmanager"
	"kraftkit.sh/unikraft"
	"kraftkit.sh/unikraft/component"
)

// pack
func (opts *GithubAction) packAndPush(ctx context.Context) error {
	output := opts.Output
	var format pack.PackageFormat
	if strings.Contains(opts.Output, "://") {
		split := strings.SplitN(opts.Output, "://", 2)
		format = pack.PackageFormat(split[0])
		output = split[1]
	} else {
		format = opts.targets[0].Format()
	}

	var err error
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
		packmanager.PackOutput(output),
	}

	if ukversion, ok := opts.targets[0].KConfig().Get(unikraft.UK_FULLVERSION); ok {
		popts = append(popts,
			packmanager.PackWithKernelVersion(ukversion.Value),
		)
	}

	var components []component.Component
	for _, targ := range opts.targets {
		components = append(components, targ)
	}

	packs, err := pm.Pack(ctx, components, popts...)
	if err != nil {
		return err
	}

	if opts.Push {
		return packs[0].Push(ctx)
	}

	return nil
}
