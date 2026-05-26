//go:build windows

package disk

import (
	"fmt"
	"os"
)

// EnumerateDisks probes for physical disks that Windows exposes.
const maxDisks = 16 // probe at most 16 physical drives

func EnumerateDisks() ([]string, error) {
	var disks []string

	for i := range maxDisks {
		// Construct the device path.
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, i)

		// Attempt to open to detect if the disk exists.
		f, err := os.Open(path)
		if err != nil {
			// Stop probing on any error.
			break
		}
		f.Close()

		disks = append(disks, path)
	}

	return disks, nil
}
