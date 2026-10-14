package azure

import (
	"fmt"
	"sort"
	"sync"
)

// DiskLifecycler manages Azure Managed Disk states, dynamic VM attachments, and safe detachments.
type DiskLifecycler struct {
	mu    sync.RWMutex
	disks map[string]*ManagedDisk
}

// NewDiskLifecycler initializes an in-memory Azure Managed Disk lifecycler.
func NewDiskLifecycler() *DiskLifecycler {
	return &DiskLifecycler{
		disks: make(map[string]*ManagedDisk),
	}
}

// RegisterDisk registers a persistent Managed Disk in the lifecycler.
func (d *DiskLifecycler) RegisterDisk(disk ManagedDisk) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if disk.StorageAccountType == "" {
		disk.StorageAccountType = StandardSSDLRS
	}

	if disk.StorageAccountType != StandardSSDLRS && disk.StorageAccountType != PremiumLRS {
		return fmt.Errorf("%w: '%s'", ErrInvalidDiskType, disk.StorageAccountType)
	}

	if disk.State == "" {
		disk.State = StateAvailable
	}
	if disk.Caching == "" {
		disk.Caching = "ReadWrite"
	}

	cpy := disk
	d.disks[disk.DiskID] = &cpy
	return nil
}

// AttachDisk mounts the persistent Managed Disk to the target Azure Virtual Machine.
func (d *DiskLifecycler) AttachDisk(diskID, vmID string, lun int, caching string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	disk, exists := d.disks[diskID]
	if !exists {
		return ErrDiskNotFound
	}

	if disk.State == StateAttached {
		return fmt.Errorf("%w: disk %s is currently attached to vm %s", ErrDiskAlreadyAttached, diskID, disk.AttachedVMID)
	}

	if caching == "" {
		caching = "ReadWrite"
	}

	disk.State = StateAttached
	disk.AttachedVMID = vmID
	disk.LUN = lun
	disk.Caching = caching
	return nil
}

// DetachDisk safely unmounts and detaches the persistent Managed Disk, restoring it to available state.
func (d *DiskLifecycler) DetachDisk(diskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	disk, exists := d.disks[diskID]
	if !exists {
		return ErrDiskNotFound
	}

	if disk.State != StateAttached {
		return ErrDiskNotAttached
	}

	disk.State = StateAvailable
	disk.AttachedVMID = ""
	return nil
}

// GetDisk retrieves current status of a registered Managed Disk.
func (d *DiskLifecycler) GetDisk(diskID string) (*ManagedDisk, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	disk, exists := d.disks[diskID]
	if !exists {
		return nil, ErrDiskNotFound
	}

	cpy := *disk
	return &cpy, nil
}

// ListDisks returns all registered Managed Disks ordered by DiskID.
func (d *DiskLifecycler) ListDisks() []*ManagedDisk {
	d.mu.RLock()
	defer d.mu.RUnlock()

	list := make([]*ManagedDisk, 0, len(d.disks))
	for _, disk := range d.disks {
		cpy := *disk
		list = append(list, &cpy)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].DiskID < list[j].DiskID
	})

	return list
}
