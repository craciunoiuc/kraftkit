// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	"kraftkit.sh/config"
	"kraftkit.sh/log"
	mplatform "kraftkit.sh/machine/platform"
	"kraftkit.sh/packmanager"
	"kraftkit.sh/unikraft/app"

	machineapi "kraftkit.sh/api/machine/v1alpha1"
	_ "kraftkit.sh/manifest"
	// _ "kraftkit.sh/oci"
)

type GithubAction struct {
	// Input arguments for the action
	// Global flags
	Loglevel string `long:"loglevel" env:"INPUT_LOGLEVEL" usage:"" default:"info"`

	// Project flags
	Workdir   string `long:"workdir" env:"INPUT_WORKDIR" usage:"Path to working directory (default is cwd)"`
	Kraftfile string `long:"kraftfile" env:"INPUT_KRAFTFILE" usage:"Path to Kraftfile or contents of Kraftfile"`

	// Build flags
	Arch   string `long:"arch" env:"INPUT_ARCH" usage:""`
	Plat   string `long:"plat" env:"INPUT_PLAT" usage:""`
	Target string `long:"target" env:"INPUT_TARGET" usage:""`

	// Running flags
	Execute bool `long:"run" env:"INPUT_EXECUTE" usage:""`

	// Packaging flags
	Args              []string `long:"args" env:"INPUT_ARGS" usage:""`
	Format            string   `long:"format" env:"INPUT_FORMAT" usage:""`
	InitRd            string   `long:"initrd" env:"INPUT_INITRD" usage:""`
	Memory            string   `long:"memory" env:"INPUT_MEMORY" usage:""`
	Name              string   `long:"name" env:"INPUT_NAME" usage:""`
	Output            string   `long:"output" env:"INPUT_OUTPUT" usage:""`
	Kconfig           bool     `long:"kconfig" env:"INPUT_KCONFIG" usage:""`
	Rootfs            string   `long:"rootfs" env:"INPUT_ROOTFS" usage:""`
	Push              bool     `long:"push" env:"INPUT_PUSH" usage:""`
	machineController machineapi.MachineService

	// Internal attributes
	project app.Application
}

func (opts *GithubAction) Pre(cmd *cobra.Command, args []string) (err error) {
	if (len(opts.Arch) > 0 || len(opts.Plat) > 0) && len(opts.Target) > 0 {
		return fmt.Errorf("target and platform/architecture are mutually exclusive")
	}

	ctx := cmd.Context()

	switch opts.Loglevel {
	case "debug":
		log.G(ctx).SetLevel(logrus.DebugLevel)
	case "trace":
		log.G(ctx).SetLevel(logrus.TraceLevel)
	}

	pm, err := packmanager.NewUmbrellaManager(ctx)
	if err != nil {
		return err
	}

	cmd.SetContext(packmanager.WithPackageManager(ctx, pm))

	if len(opts.Workdir) == 0 {
		opts.Workdir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	popts := []app.ProjectOption{
		app.WithProjectWorkdir(opts.Workdir),
	}

	// Check if the provided Kraftfile is set, and whether it's either a path or
	// an inline file.
	if len(opts.Kraftfile) > 0 {
		if _, err := os.Stat(opts.Kraftfile); err == nil {
			popts = append(popts, app.WithProjectKraftfile(opts.Kraftfile))
		} else {
			// Dump the contents to a file
			fi, err := os.CreateTemp("", "*.Kraftfile")
			if err != nil {
				return fmt.Errorf("could not create temporary file for Kraftfile: %w", err)
			}

			defer fi.Close()

			n, err := fi.Write([]byte(opts.Kraftfile))
			if err != nil {
				return fmt.Errorf("could not write to temporary Kraftfile: %w", err)
			}

			if n != len(opts.Kraftfile) {
				return fmt.Errorf("could not write entire Kraftfile to %s", fi.Name())
			}

			popts = append(popts, app.WithProjectKraftfile(fi.Name()))
		}
	} else {
		popts = append(popts, app.WithProjectDefaultKraftfiles())
	}

	// Initialize at least the configuration options for a project
	opts.project, err = app.NewProjectFromOptions(ctx, popts...)
	if err != nil && errors.Is(err, app.ErrNoKraftfile) {
		return fmt.Errorf("cannot build project directory without a Kraftfile")
	} else if err != nil {
		return fmt.Errorf("could not initialize project directory: %w", err)
	}

	return nil
}

func (opts *GithubAction) Run(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if err := opts.pull(ctx); err != nil {
		return fmt.Errorf("could not pull project components: %w", err)
	}

	if err := opts.build(ctx); err != nil {
		return fmt.Errorf("could not build unikernel: %w", err)
	}

	if opts.Execute {
		var err error

		if opts.Name == "" {
			return fmt.Errorf("no name specified for the unikernel")
		}

		machineStrategy, ok := mplatform.Strategies()[mplatform.PlatformsByName()[opts.Plat]]
		if !ok {
			return fmt.Errorf("unsupported platform driver: %s (contributions welcome!)", opts.Plat)
		}

		opts.machineController, err = machineStrategy.NewMachineV1alpha1(ctx)
		if err != nil {
			return err
		}

		if err := opts.execute(ctx); err != nil {
			return fmt.Errorf("could not run unikernel: %w", err)
		}
	}

	if opts.Output != "" {
		if err := opts.pack(ctx); err != nil {
			return fmt.Errorf("could not package unikernel: %w", err)
		}

		if opts.Push {
			if err := opts.push(ctx); err != nil {
				return fmt.Errorf("could not push unikernel: %w", err)
			}
		}
	}

	return nil
}

func main() {
	cmd, err := cmdfactory.New(&GithubAction{}, cobra.Command{})
	if err != nil {
		panic(err)
	}

	ctx := signals.SetupSignalContext()

	cfg, err := config.NewDefaultKraftKitConfig()
	if err != nil {
		panic(err)
	}

	cfgm, err := config.NewConfigManager(cfg)
	if err != nil {
		panic(err)
	}

	// Set up the config manager in the context if it is available
	ctx = config.WithConfigManager(ctx, cfgm)

	cmd, args, err := cmd.Find(os.Args[1:])
	if err != nil {
		panic(err)
	}

	if err := cmdfactory.AttributeFlags(cmd, cfg, args...); err != nil {
		panic(err)
	}

	// Set up a default logger based on the internal TextFormatter
	logger := logrus.New()

	formatter := new(log.TextFormatter)
	formatter.FullTimestamp = true
	formatter.DisableTimestamp = true
	logger.Formatter = formatter

	// Set up the logger in the context if it is available
	ctx = log.WithLogger(ctx, logger)

	cmdfactory.Main(ctx, cmd)
}
