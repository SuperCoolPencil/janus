# Janus

Janus is a read-only ext4 filesystem driver for Windows. It allows you to mount Linux ext4 partitions as native Windows drive letters, enabling seamless access to your Linux files directly from Windows Explorer, Command Prompt, or any Windows application.

It is written entirely in Go and relies on WinFsp to interface with the Windows kernel.

## Features

*   **Read-Only:** Safely read from ext4 partitions without risking corruption.
*   **Native Windows Integration:** Mounts as a standard drive letter (e.g., `G:`).
*   **High Performance:** Caches directory entries and uses on-demand block fetching to ensure Windows Explorer remains responsive even when browsing large directories or files.
*   **Self-Contained Installer:** Janus can automatically download and install its WinFsp dependency.

## Prerequisites

*   Windows 10 or Windows 11 (64-bit)
*   Administrator privileges (required for raw disk access)

## Installation

Janus requires WinFsp, a FUSE-like file system in userspace framework for Windows. You can have Janus automatically install it for you.

1.  Open **Command Prompt** or **PowerShell** as **Administrator**.
2.  Run the installation command:
    ```cmd
    janus.exe install
    ```
    Janus will check if WinFsp is installed. If not, it will dynamically fetch the latest release from GitHub and install it silently.

## Usage

You must run Janus from an **Administrator** command prompt so it can read raw physical disks.

### 1. List Available Disks
Find the physical disk containing your Linux partition:
```cmd
janus.exe devices
```
*Note the Disk number and Partition number.*

### 2. List Partitions on a Disk
To inspect the partitions on a specific disk (e.g., Disk 0):
```cmd
janus.exe \\.\PhysicalDrive0
```

### 3. Mount the Partition
Mount the desired partition to a drive letter (e.g., `G:`). 
For example, to mount Partition 1 on Disk 0 to drive `G:`:
```cmd
janus.exe mount G: 0 1
```

Keep the terminal window open. The drive will remain mounted until you press `Ctrl+C` in the terminal to unmount it.

## Architecture

*   **`ext4`:** A pure Go implementation of an ext4 reader. It parses the superblock, block group descriptors, inode tables, and extent trees directly from raw bytes.
*   **`disk`:** Interacts with the Windows storage subsystem to enumerate physical drives and read partition tables.
*   **`mount`:** Translates ext4 concepts into FUSE operations using `cgofuse`.
*   **WinFsp:** The Windows kernel driver that exposes the FUSE filesystem to the Windows I/O manager.

## Limitations

*   **Read-Only:** Write support is not currently implemented.
*   **Case Sensitivity:** Ext4 is case-sensitive, while Windows is generally case-insensitive. Janus exposes the raw names. Applications that expect case-insensitivity might behave unpredictably if multiple files differ only by case.
*   **Permissions:** Unix permissions are ignored. All files are presented as readable to ensure compatibility with Windows Explorer.