package aws

import (
	"fmt"
	"sync"
)

// VolumeLifecycler manages EBS volume states, attachments, and clean unmounting to prevent corruption.
type VolumeLifecycler struct {
	mu      sync.RWMutex
	volumes map[string]*EBSVolume
}

// NewVolumeLifecycler initializes an in-memory EBS lifecycler.
func NewVolumeLifecycler() *VolumeLifecycler {
	return &VolumeLifecycler{
		volumes: make(map[string]*EBSVolume),
	}
}

// RegisterVolume registers a persistent gp3 EBS volume in the lifecycler.
func (v *VolumeLifecycler) RegisterVolume(vol EBSVolume) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if vol.VolumeType == "" {
		vol.VolumeType = "gp3"
	}
	if vol.VolumeType != "gp3" {
		return fmt.Errorf("%w: '%s'", ErrInvalidVolumeType, vol.VolumeType)
	}
	if vol.State == "" {
		vol.State = StateAvailable
	}

	cpy := vol
	v.volumes[vol.VolumeID] = &cpy
	return nil
}

// AttachVolume mounts the persistent EBS volume to the target EC2 instance.
func (v *VolumeLifecycler) AttachVolume(volID, instanceID, deviceName string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	vol, exists := v.volumes[volID]
	if !exists {
		return ErrVolumeNotFound
	}

	if vol.State == StateAttached {
		return fmt.Errorf("%w: volume %s currently attached to %s", ErrVolumeAlreadyAttached, volID, vol.AttachedInstanceID)
	}

	if deviceName == "" {
		deviceName = "/dev/xvdf"
	}

	vol.State = StateAttached
	vol.AttachedInstanceID = instanceID
	vol.DeviceName = deviceName
	return nil
}

// DetachVolume safely unmounts the persistent EBS volume, restoring it to available state.
func (v *VolumeLifecycler) DetachVolume(volID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	vol, exists := v.volumes[volID]
	if !exists {
		return ErrVolumeNotFound
	}

	if vol.State != StateAttached {
		return ErrVolumeNotAttached
	}

	vol.State = StateAvailable
	vol.AttachedInstanceID = ""
	vol.DeviceName = ""
	return nil
}

// GetVolume retrieves current volume status.
func (v *VolumeLifecycler) GetVolume(volID string) (*EBSVolume, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	vol, exists := v.volumes[volID]
	if !exists {
		return nil, ErrVolumeNotFound
	}

	cpy := *vol
	return &cpy, nil
}
