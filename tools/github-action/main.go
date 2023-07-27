// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kraftkit.sh/cmdfactory"
	"kraftkit.sh/config"
	"kraftkit.sh/log"
	"kraftkit.sh/packmanager"
	"kraftkit.sh/unikraft/app"

	machineapi "kraftkit.sh/api/machine/v1alpha1"
	_ "kraftkit.sh/manifest"
	// _ "kraftkit.sh/oci"
)

type GithubAction struct {
	// Input arguments for the action
	// Global flags
	LogLevel string `long:"log-level" env:"INPUT_LOG_LEVEL" usage:"" default:"info"`

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

	// switch opts.LogLevel {
	// case
	// }

	ctx := cmd.Context()
	pm, err := packmanager.NewUmbrellaManager(ctx)
	if err != nil {
		return err
	}

	cmd.SetContext(packmanager.WithPackageManager(ctx, pm))

	fmt.Println(opts.Workdir)
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
		if err := opts.execute(ctx); err != nil {
			return fmt.Errorf("could not run unikernel: %w", err)
		}
	}

	if len(opts.Output) > 0 {
		if err := opts.pack(ctx); err != nil {
			return fmt.Errorf("could not package unikernel: %w", err)
		}

		if err := opts.push(ctx); err != nil {
			return fmt.Errorf("could not push unikernel: %w", err)
		}
	}

	return nil
}

type lightweightCliOptions struct {
	Logger         *logrus.Logger
	ConfigManager  *config.ConfigManager[config.KraftKit]
	PackageManager packmanager.PackageManager
}

type lightweightCliOption func(*lightweightCliOptions) error

// withLightweightDefaultLogger sets up the built in logger based on provided
// config found from the ConfigManager.
func withLightweightDefaultLogger() lightweightCliOption {
	return func(copts *lightweightCliOptions) error {
		if copts.Logger != nil {
			return nil
		}

		// Configure the logger based on parameters set by in KraftKit's
		// configuration
		if copts.ConfigManager == nil {
			copts.Logger = log.L
			return nil
		}

		// Set up a default logger based on the internal TextFormatter
		logger := logrus.New()

		switch log.LoggerTypeFromString(copts.ConfigManager.Config.Log.Type) {
		case log.QUIET:
			formatter := new(logrus.TextFormatter)
			logger.Formatter = formatter

		case log.BASIC:
			formatter := new(log.TextFormatter)
			formatter.FullTimestamp = true
			formatter.DisableTimestamp = true

			if copts.ConfigManager.Config.Log.Timestamps {
				formatter.DisableTimestamp = false
			} else {
				formatter.TimestampFormat = ">"
			}

			logger.Formatter = formatter

		case log.FANCY:
			formatter := new(log.TextFormatter)
			formatter.FullTimestamp = true
			formatter.DisableTimestamp = true

			if copts.ConfigManager.Config.Log.Timestamps {
				formatter.DisableTimestamp = false
			} else {
				formatter.TimestampFormat = ">"
			}

			logger.Formatter = formatter

		case log.JSON:
			formatter := new(logrus.JSONFormatter)
			formatter.DisableTimestamp = true

			if copts.ConfigManager.Config.Log.Timestamps {
				formatter.DisableTimestamp = false
			}

			logger.Formatter = formatter
		}

		level, ok := log.Levels()[copts.ConfigManager.Config.Log.Level]
		if !ok {
			logger.Level = logrus.InfoLevel
		} else {
			logger.Level = level
		}

		// Save the logger
		copts.Logger = logger

		return nil
	}
}

func withLightweightDefaultConfigManager() lightweightCliOption {
	return func(copts *lightweightCliOptions) error {
		cfg, err := config.NewDefaultKraftKitConfig()
		if err != nil {
			return err
		}
		cfgm, err := config.NewConfigManager(cfg)
		if err != nil {
			return err
		}

		copts.ConfigManager = cfgm

		return nil
	}
}

func main() {
	cmd, err := cmdfactory.New(&GithubAction{}, cobra.Command{})
	if err != nil {
		panic(err)
	}

	ctx := signals.SetupSignalContext()
	copts := &lightweightCliOptions{}

	runtime.LockOSThread()

	for _, o := range []lightweightCliOption{
		withLightweightDefaultConfigManager(),
		withLightweightDefaultLogger(),
	} {
		if err := o(copts); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// Set up the config manager in the context if it is available
	if copts.ConfigManager != nil {
		ctx = config.WithConfigManager(ctx, copts.ConfigManager)
	}

	// Set up the logger in the context if it is available
	if copts.Logger != nil {
		ctx = log.WithLogger(ctx, copts.Logger)
	}

	cmdfactory.Main(ctx, cmd)
}
