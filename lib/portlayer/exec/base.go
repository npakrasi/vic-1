// Copyright 2016 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/vic/lib/config/executor"
	"github.com/vmware/vic/pkg/trace"
	"github.com/vmware/vic/pkg/vsphere/extraconfig"
	"github.com/vmware/vic/pkg/vsphere/extraconfig/vmomi"
	"github.com/vmware/vic/pkg/vsphere/tasks"
	"github.com/vmware/vic/pkg/vsphere/vm"

	log "github.com/Sirupsen/logrus"
)

// NotYetExistError is returned when a call that requires a VM exist is made
type NotYetExistError struct {
	ID string
}

func (e NotYetExistError) Error() string {
	return fmt.Sprintf("%s is not completely created", e.ID)
}

// containerBase holds fields common between Handle and Container. The fields and
// methods in containerBase should not require locking as they're primary use is:
// a. for read-only reference when used in Container
// b. single use/no-concurrent modification when used in Handle
type containerBase struct {
	ExecConfig *executor.ExecutorConfig

	// original - can be pointers so long as refreshes
	// use different instances of the structures
	Config  *types.VirtualMachineConfigInfo
	Runtime *types.VirtualMachineRuntimeInfo

	// doesn't change so can be copied here
	vm *vm.VirtualMachine
}

func newBase(vm *vm.VirtualMachine, c *types.VirtualMachineConfigInfo, r *types.VirtualMachineRuntimeInfo) *containerBase {
	base := &containerBase{
		ExecConfig: &executor.ExecutorConfig{},
		Config:     c,
		Runtime:    r,
		vm:         vm,
	}

	// construct a working copy of the exec config
	if c != nil && c.ExtraConfig != nil {
		src := vmomi.OptionValueSource(c.ExtraConfig)
		extraconfig.Decode(src, base.ExecConfig)
	}

	return base
}

// unlocked refresh of container state
func (c *containerBase) refresh(ctx context.Context) error {
	defer trace.End(trace.Begin(c.ExecConfig.ID))

	base, err := c.updates(ctx)
	if err != nil {
		log.Errorf("Unable to update container %s", c.ExecConfig.ID)
		return err
	}

	// copy over the new state
	*c = *base
	return nil
}

// updates acquires updates from the infrastructure without holding a lock
func (c *containerBase) updates(ctx context.Context) (*containerBase, error) {
	defer trace.End(trace.Begin(c.ExecConfig.ID))

	var o mo.VirtualMachine

	// make sure we have vm
	if c.vm == nil {
		return nil, NotYetExistError{c.ExecConfig.ID}
	}

	if err := c.vm.Properties(ctx, c.vm.Reference(), []string{"config", "runtime"}, &o); err != nil {
		return nil, err
	}

	base := &containerBase{
		vm:         c.vm,
		Config:     o.Config,
		Runtime:    &o.Runtime,
		ExecConfig: &executor.ExecutorConfig{},
	}

	// Get the ExtraConfig
	extraconfig.Decode(vmomi.OptionValueSource(o.Config.ExtraConfig), base.ExecConfig)

	return base, nil
}

func (c *containerBase) startGuestProgram(ctx context.Context, name string, args string) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	defer trace.End(trace.Begin(c.ExecConfig.ID))
	o := guest.NewOperationsManager(c.vm.Client.Client, c.vm.Reference())
	m, err := o.ProcessManager(ctx)
	if err != nil {
		return err
	}

	spec := types.GuestProgramSpec{
		ProgramPath: name,
		Arguments:   args,
	}

	auth := types.NamePasswordAuthentication{
		Username: c.ExecConfig.ID,
	}

	_, err = m.StartProgram(ctx, &auth, &spec)

	return err
}

func (c *containerBase) start(ctx context.Context) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	// Power on
	_, err := c.vm.WaitForResult(ctx, func(ctx context.Context) (tasks.Task, error) {
		return c.vm.PowerOn(ctx)
	})
	if err != nil {
		return err
	}

	// guestinfo key that we want to wait for
	key := extraconfig.CalculateKeys(c.ExecConfig, fmt.Sprintf("Sessions.%s.Started", c.ExecConfig.ID), "")[0]
	var detail string

	// Wait some before giving up...
	ctx, cancel := context.WithTimeout(ctx, propertyCollectorTimeout)
	defer cancel()

	detail, err = c.vm.WaitForKeyInExtraConfig(ctx, key)
	if err != nil {
		return fmt.Errorf("unable to wait for process launch status: %s", err.Error())
	}

	if detail != "true" {
		return errors.New(detail)
	}

	return nil
}

func (c *containerBase) stop(ctx context.Context, waitTime *int32) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	// get existing state and set to stopping
	// if there's a failure we'll revert to existing

	err := c.shutdown(ctx, waitTime)
	if err == nil {
		return nil
	}

	log.Warnf("stopping %s via hard power off due to: %s", c.ExecConfig.ID, err)

	return c.poweroff(ctx)
}

func (c *containerBase) kill(ctx context.Context) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	wait := 10 * time.Second // default
	sig := string(ssh.SIGKILL)
	log.Infof("sending kill -%s %s", sig, c.ExecConfig.ID)

	err := c.startGuestProgram(ctx, "kill", sig)
	if err == nil {
		log.Infof("waiting %s for %s to power off", wait, c.ExecConfig.ID)
		timeout, err := c.waitForPowerState(ctx, wait, types.VirtualMachinePowerStatePoweredOff)
		if err == nil {
			return nil // VM has powered off
		}

		if timeout {
			log.Warnf("timeout (%s) waiting for %s to power off via SIG%s", wait, c.ExecConfig.ID, sig)
		}
	}

	if err != nil {
		log.Warnf("killing %s attempt resulted in: %s", c.ExecConfig.ID, err)
	}

	log.Warnf("killing %s via hard power off", c.ExecConfig.ID)

	return c.poweroff(ctx)
}

func (c *containerBase) shutdown(ctx context.Context, waitTime *int32) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	wait := 10 * time.Second // default
	if waitTime != nil && *waitTime > 0 {
		wait = time.Duration(*waitTime) * time.Second
	}

	cs := c.ExecConfig.Sessions[c.ExecConfig.ID]
	stop := []string{cs.StopSignal, string(ssh.SIGKILL)}
	if stop[0] == "" {
		stop[0] = string(ssh.SIGTERM)
	}

	for _, sig := range stop {
		msg := fmt.Sprintf("sending kill -%s %s", sig, c.ExecConfig.ID)
		log.Info(msg)

		err := c.startGuestProgram(ctx, "kill", sig)
		if err != nil {
			return fmt.Errorf("%s: %s", msg, err)
		}

		log.Infof("waiting %s for %s to power off", wait, c.ExecConfig.ID)
		timeout, err := c.waitForPowerState(ctx, wait, types.VirtualMachinePowerStatePoweredOff)
		if err == nil {
			return nil // VM has powered off
		}

		if !timeout {
			return err // error other than timeout
		}

		log.Warnf("timeout (%s) waiting for %s to power off via SIG%s", wait, c.ExecConfig.ID, sig)
	}

	return fmt.Errorf("failed to shutdown %s via kill signals %s", c.ExecConfig.ID, stop)
}

func (c *containerBase) poweroff(ctx context.Context) error {
	// make sure we have vm
	if c.vm == nil {
		return NotYetExistError{c.ExecConfig.ID}
	}

	_, err := c.vm.WaitForResult(ctx, func(ctx context.Context) (tasks.Task, error) {
		return c.vm.PowerOff(ctx)
	})

	if err != nil {

		// It is possible the VM has finally shutdown in between, ignore the error in that case
		if terr, ok := err.(task.Error); ok {
			switch terr := terr.Fault().(type) {
			case *types.InvalidPowerState:
				if terr.ExistingState == types.VirtualMachinePowerStatePoweredOff {
					log.Warnf("power off %s task skipped (state was already %s)", c.ExecConfig.ID, terr.ExistingState)
					return nil
				}
				log.Warnf("invalid power state during power off: %s", terr.ExistingState)

			case *types.GenericVmConfigFault:

				// Check if the poweroff task was canceled due to a concurrent guest shutdown
				if len(terr.FaultMessage) > 0 && terr.FaultMessage[0].Key == vmNotSuspendedKey {
					log.Infof("power off %s task skipped due to guest shutdown", c.ExecConfig.ID)
					return nil
				}
				log.Warnf("generic vm config fault during power off: %#v", terr)

			default:
				log.Warnf("hard power off failed due to: %#v", terr)
			}
		}

		return err
	}

	return nil
}

func (c *containerBase) waitForPowerState(ctx context.Context, max time.Duration, state types.VirtualMachinePowerState) (bool, error) {
	defer trace.End(trace.Begin(c.ExecConfig.ID))
	timeout, cancel := context.WithTimeout(ctx, max)
	defer cancel()

	err := c.vm.WaitForPowerState(timeout, state)
	if err != nil {
		return timeout.Err() != nil, err
	}

	return false, nil
}
