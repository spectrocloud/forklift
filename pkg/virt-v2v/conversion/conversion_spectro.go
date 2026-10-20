package conversion

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// vmdkDescriptorMagic is the first line of a VMDK descriptor file.
const vmdkDescriptorMagic = "# Disk DescriptorFile"

// diskHeaderSize is how many bytes we sniff to decide whether a disk file
// still holds leftover data from an earlier attempt.
const diskHeaderSize = 512

// prepareDiskFilesForInPlace ensures disk files are ready for virt-v2v-in-place.
//
// virt-v2v-in-place reads VM data from vSphere (via the libvirt connection) and
// writes RAW format directly to the disk files named in the libvirt XML. Those
// files must be empty or absent: if one still holds invalid or partially written
// VMDK data from a failed attempt, virt-v2v-in-place (or the qemu-img it shells
// out to) fails while probing the format.
func (c *Conversion) prepareDiskFilesForInPlace() error {
	for _, disk := range c.Disks {
		// Block devices need no preparation.
		if disk.IsBlockDev {
			continue
		}

		info, err := os.Stat(disk.Path)
		if err != nil {
			if os.IsNotExist(err) {
				// virt-v2v-in-place will create it.
				continue
			}
			return fmt.Errorf("failed to stat disk file %s: %w", disk.Path, err)
		}

		// Empty file is exactly what we want.
		if info.Size() == 0 {
			continue
		}

		header, err := readDiskHeader(disk.Path, diskHeaderSize)
		if err != nil {
			return fmt.Errorf("failed to read disk file %s: %w", disk.Path, err)
		}

		// Truncate when the file is a VMDK descriptor, or is too short to be a
		// usable image. A valid RAW image is left alone.
		if bytes.HasPrefix(header, []byte(vmdkDescriptorMagic)) || len(header) < diskHeaderSize {
			fmt.Printf("Truncating disk file %s (size: %d) to prepare for in-place conversion\n",
				disk.Path, info.Size())
			if err := os.Truncate(disk.Path, 0); err != nil {
				return fmt.Errorf("failed to truncate disk file %s: %w", disk.Path, err)
			}
		}
	}

	return nil
}

// readDiskHeader reads up to n leading bytes of path. A short read is not an
// error: the caller treats a short header as "not a usable image".
func readDiskHeader(path string, n int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buf := make([]byte, n)
	read, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:read], nil
}
