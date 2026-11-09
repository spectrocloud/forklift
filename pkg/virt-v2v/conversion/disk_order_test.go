package conversion

import (
	"reflect"
	"testing"
)

func TestSortDiskPaths(t *testing.T) {
	got := sortDiskPaths([]string{
		"/mnt/disks/disk0",
		"/mnt/disks/disk10",
		"/mnt/disks/disk2",
		"/mnt/disks/disk3",
		"/dev/block1",
		"/dev/block11",
	})
	want := []string{
		"/mnt/disks/disk0",
		"/dev/block1",
		"/mnt/disks/disk2",
		"/mnt/disks/disk3",
		"/mnt/disks/disk10",
		"/dev/block11",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortDiskPaths:\n got  %v\n want %v", got, want)
	}
}
