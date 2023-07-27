// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2022, Unikraft GmbH and The KraftKit Authors.
// Licensed under the BSD-3-Clause License (the "License").
// You may not use this file except in compliance with the License.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rancher/wrangler/pkg/signals"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	machineapi "kraftkit.sh/api/machine/v1alpha1"
	"kraftkit.sh/config"
	"kraftkit.sh/initrd"
	"kraftkit.sh/iostreams"
	"kraftkit.sh/log"
	"kraftkit.sh/unikraft/app"
	"kraftkit.sh/unikraft/target"
)

type runnerProject struct {
	workdir string
	args    []string
}

// String implements Runner.
func (runner *runnerProject) String() string {
	return "project"
}

// Runnable implements Runner.
func (runner *runnerProject) Runnable(ctx context.Context, opts *GithubAction, args ...string) (bool, error) {
	if opts.Workdir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return false, fmt.Errorf("getting current working directory: %w", err)
		}

		if len(args) == 0 {
			runner.workdir = cwd
		} else {
			runner.workdir = cwd
			runner.args = args
			if f, err := os.Stat(args[0]); err == nil && f.IsDir() {
				runner.workdir = args[0]
				runner.args = args[1:]
			}
		}
	} else {
		runner.workdir = opts.Workdir
		runner.args = args
	}

	if !app.IsWorkdirInitialized(runner.workdir) {
		return false, fmt.Errorf("path is not project: %s", runner.workdir)
	}

	return true, nil
}

// Prepare implements Runner.
func (runner *runnerProject) Prepare(ctx context.Context, opts *GithubAction, machine *machineapi.Machine, args ...string) error {
	popts := []app.ProjectOption{
		app.WithProjectWorkdir(runner.workdir),
	}

	if len(opts.Kraftfile) > 0 {
		popts = append(popts, app.WithProjectKraftfile(opts.Kraftfile))
	} else {
		popts = append(popts, app.WithProjectDefaultKraftfiles())
	}

	project, err := app.NewProjectFromOptions(ctx, popts...)
	if err != nil {
		return fmt.Errorf("could not instantiate project directory %s: %v", runner.workdir, err)
	}

	// Filter project targets by any provided CLI options
	targets := target.Filter(
		project.Targets(),
		opts.Arch,
		opts.Plat,
		opts.Target,
	)

	var t target.Target

	switch {
	case len(targets) == 0:
		return fmt.Errorf("could not detect any project targets based on plat=\"%s\" arch=\"%s\"", opts.Plat, opts.Arch)

	case len(targets) == 1:
		t = targets[0]

	case config.G[config.KraftKit](ctx).NoPrompt && len(targets) > 1:
		return fmt.Errorf("could not determine what to run based on provided CLI arguments")

	default:
		t, err = target.Select(targets)
		if err != nil {
			return fmt.Errorf("could not select target: %v", err)
		}
	}

	// Provide a meaningful name
	targetName := t.Name()
	if targetName == project.Name() || targetName == "" {
		targetName = t.Platform().Name() + "/" + t.Architecture().Name()
	}

	machine.Spec.Kernel = "project://" + project.Name() + ":" + targetName
	machine.Spec.Architecture = t.Architecture().Name()
	machine.Spec.Platform = t.Platform().Name()
	machine.Spec.ApplicationArgs = runner.args

	machine.Status.KernelPath = t.Kernel()

	if len(opts.InitRd) > 0 {
		machine.Status.InitrdPath = opts.InitRd
	}

	if _, err := os.Stat(machine.Status.KernelPath); err != nil && os.IsNotExist(err) {
		return fmt.Errorf("cannot run the selected project target '%s' without building the kernel: try running `kraft build` first: %w", targetName, err)
	}

	return nil
}

// execute
func (opts *GithubAction) execute(ctx context.Context) error {
	var err error

	machine := &machineapi.Machine{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: machineapi.MachineSpec{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
			Emulation: true,
		},
	}

	runner := &runnerProject{}

	capable, err := runner.Runnable(ctx, opts)
	if !capable && err != nil {
		return err
	}

	log.G(ctx).WithField("runner", runner.String()).Debug("using")

	// Prepare the machine specification based on the compatible runner.
	if err := runner.Prepare(ctx, opts, machine); err != nil {
		return err
	}

	// Override with command-line flags
	if len(opts.Args) > 0 {
		machine.Spec.KernelArgs = opts.Args
	}

	if len(opts.Memory) > 0 {
		quantity, err := resource.ParseQuantity(opts.Memory)
		if err != nil {
			return err
		}

		machine.Spec.Resources.Requests[corev1.ResourceMemory] = quantity
	}

	machine.ObjectMeta.Name = opts.Name

	// If the user has supplied an initram path, set this now, this overrides any
	// preparation and is considered higher priority compared to what has been set
	// prior to this point.
	if opts.InitRd != "" {
		if machine.ObjectMeta.UID == "" {
			machine.ObjectMeta.UID = uuid.NewUUID()
		}

		if len(machine.Status.StateDir) == 0 {
			machine.Status.StateDir = filepath.Join(config.G[config.KraftKit](ctx).RuntimeDir, string(machine.ObjectMeta.UID))
		}

		if err := os.MkdirAll(machine.Status.StateDir, 0o755); err != nil {
			return fmt.Errorf("could not make machine state dir: %w", err)
		}

		var ramfs *initrd.InitrdConfig
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not get current working directory: %w", err)
		}

		if strings.Contains(opts.InitRd, initrd.InputDelimeter) {
			output := filepath.Join(machine.Status.StateDir, "initramfs.cpio")

			log.G(ctx).
				WithField("output", output).
				Debug("serializing initramfs cpio archive")

			ramfs, err = initrd.NewFromMapping(cwd, output, opts.InitRd)
			if err != nil {
				return fmt.Errorf("could not prepare initramfs: %w", err)
			}
		} else if f, err := os.Stat(opts.InitRd); err == nil && f.IsDir() {
			output := filepath.Join(machine.Status.StateDir, "initramfs.cpio")

			log.G(ctx).
				WithField("output", output).
				Debug("serializing initramfs cpio archive")

			ramfs, err = initrd.NewFromMapping(cwd, output, fmt.Sprintf("%s:/", opts.InitRd))
			if err != nil {
				return fmt.Errorf("could not prepare initramfs: %w", err)
			}
		} else {
			ramfs, err = initrd.NewFromFile(cwd, opts.InitRd)
			if err != nil {
				return fmt.Errorf("could not prepare initramfs: %w", err)
			}
		}

		machine.Spec.Rootfs = fmt.Sprintf("cpio+%s://%s", ramfs.Format, ramfs.Output)
		machine.Status.InitrdPath = ramfs.Output
	}

	// Create the machine
	machine, err = opts.machineController.Create(ctx, machine)
	if err != nil {
		return err
	}

	go func() {
		events, errs, err := opts.machineController.Watch(ctx, machine)
		if err != nil {
			log.G(ctx).Errorf("could not listen for machine updates: %v", err)
			signals.RequestShutdown()
			return
		}

		log.G(ctx).Trace("waiting for machine events")

	loop:
		for {
			// Wait on either channel
			select {
			case update := <-events:
				switch update.Status.State {
				case machineapi.MachineStateExited, machineapi.MachineStateFailed:
					signals.RequestShutdown()
					break loop
				}

			case err := <-errs:
				log.G(ctx).Errorf("received event error: %v", err)
				signals.RequestShutdown()
				break loop

			case <-ctx.Done():
				break loop
			}
		}
	}()

	// Start the machine
	machine, err = opts.machineController.Start(ctx, machine)
	if err != nil {
		signals.RequestShutdown()
		return err
	}

	logs, errs, err := opts.machineController.Logs(ctx, machine)
	if err != nil {
		signals.RequestShutdown()
		return fmt.Errorf("could not listen for machine logs: %v", err)
	}

loop:
	for {
		// Wait on either channel
		select {
		case line := <-logs:
			fmt.Fprint(iostreams.G(ctx).Out, line)

		case err := <-errs:
			log.G(ctx).Errorf("received event error: %v", err)
			signals.RequestShutdown()
			break loop

		case <-ctx.Done():
			break loop
		}
	}

	if machine.Status.State == machineapi.MachineStateExited {
		return nil
	}

	if machine.Status.State == machineapi.MachineStateFailed {
		return fmt.Errorf("machine failed when running")
	}

	if _, err := opts.machineController.Stop(ctx, machine); err != nil {
		log.G(ctx).Errorf("could not stop: %v", err)
	}

	if _, err := opts.machineController.Delete(ctx, machine); err != nil {
		log.G(ctx).Errorf("could not remove: %v", err)
	}

	return nil
}
