package mount

// Package mount implements the FUSE filesystem interface using cgofuse.

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/supercoolpencil/janus/ext4"
	"github.com/winfsp/cgofuse/fuse"
)

// JanusFS implements fuse.FileSystemInterface for a read-only ext4 partition.
type JanusFS struct {
	fuse.FileSystemBase
	ext4        *ext4.FileSystem
	fileHandles HandleTable[[]byte]
	dirHandles  HandleTable[[]ext4.DirEntry2]
}

// NewJanusFS creates a JanusFS backed by an already-initialised ext4.FileSystem.
// ReadSuperBlock and ReadGroupDescriptors must have been called already.
func NewJanusFS(fs *ext4.FileSystem) *JanusFS {
	return &JanusFS{ext4: fs}
}

// Lifecycle

// Init is called once by WinFsp immediately after the filesystem is mounted.
// This is our first chance to log that the FUSE callbacks are live.
func (j *JanusFS) Init() {
	log.Printf("[FUSE] Init() — filesystem is live, WinFsp handshake complete")
}

// Destroy is called once when the filesystem is unmounted.
func (j *JanusFS) Destroy() {
	log.Printf("[FUSE] Destroy() — filesystem unmounted")
}

// Filesystem metadata

// Statfs returns filesystem-wide statistics.
func (j *JanusFS) Statfs(path string, stat *fuse.Statfs_t) int {
	sb, err := j.ext4.Superblock()
	if err != nil {
		log.Printf("[FUSE] Statfs(%q) ERROR — superblock not ready: %v", path, err)
		return -fuse.EIO
	}

	stat.Bsize = uint64(sb.BlockSize())
	stat.Frsize = stat.Bsize // MUST equal Bsize; WinFsp computes size as Frsize*Blocks
	stat.Blocks = uint64(sb.S_blocks_count_lo)
	stat.Bfree = uint64(sb.S_free_blocks_count_lo)
	stat.Bavail = stat.Bfree
	stat.Files = uint64(sb.S_inodes_count)
	stat.Ffree = uint64(sb.S_free_inodes_count)
	stat.Favail = stat.Ffree
	stat.Namemax = 255

	log.Printf("[FUSE] Statfs(%q) OK — bsize=%d blocks=%d bfree=%d",
		path, stat.Bsize, stat.Blocks, stat.Bfree)
	return 0
}

// File / directory attribute lookup

// Getattr returns the metadata of the inode at path.
func (j *JanusFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	inode, err := j.ext4.Walk(path)
	if err != nil {
		if errors.Is(err, ext4.ErrNotExist) {
			log.Printf("[FUSE] Getattr(%q) → ENOENT", path)
			return -fuse.ENOENT
		}
		log.Printf("[FUSE] Getattr(%q) ERROR: %v", path, err)
		return -fuse.EIO
	}

	inodeToStat(inode, stat)
	log.Printf("[FUSE] Getattr(%q) OK — mode=0%o size=%d", path, stat.Mode, stat.Size)
	return 0
}

// Directory operations

// Opendir resolves path to a directory, caches its entries, and returns a handle.
func (j *JanusFS) Opendir(path string) (int, uint64) {
	inode, err := j.ext4.Walk(path)
	if err != nil {
		if errors.Is(err, ext4.ErrNotExist) {
			log.Printf("[FUSE] Opendir(%q) → ENOENT", path)
			return -fuse.ENOENT, 0
		}
		log.Printf("[FUSE] Opendir(%q) ERROR walk: %v", path, err)
		return -fuse.EIO, 0
	}

	if !inode.IsDir() {
		log.Printf("[FUSE] Opendir(%q) → ENOTDIR (mode=0%o)", path, inode.I_mode)
		return -fuse.ENOTDIR, 0
	}

	entries, err := j.ext4.ReadDir(inode)
	if err != nil {
		log.Printf("[FUSE] Opendir(%q) ERROR ReadDir: %v", path, err)
		return -fuse.EIO, 0
	}

	fh := j.dirHandles.Store(entries)
	log.Printf("[FUSE] Opendir(%q) OK — fh=%d entries=%d", path, fh, len(entries))
	return 0, fh
}

// Readdir feeds directory entries to the kernel via fill.
func (j *JanusFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {

	entries, ok := j.dirHandles.Load(fh)
	if !ok {
		log.Printf("[FUSE] Readdir(%q) fh=%d → EBADF (handle not found)", path, fh)
		return -fuse.EBADF
	}

	log.Printf("[FUSE] Readdir(%q) fh=%d — filling %d entries", path, fh, len(entries))
	for i := range entries {
		e := &entries[i]

		var statPtr *fuse.Stat_t
		if inode, err := j.ext4.ReadInode(e.Inode); err == nil {
			var s fuse.Stat_t
			inodeToStat(inode, &s)
			statPtr = &s
		} else {
			log.Printf("[FUSE] Readdir(%q) — ReadInode(%d) failed for %q: %v",
				path, e.Inode, e.Name, err)
		}

		if !fill(e.Name, statPtr, 0) {
			log.Printf("[FUSE] Readdir(%q) — fill() returned false at entry %q (buffer full)", path, e.Name)
			break
		}
	}
	return 0
}

// Releasedir frees the cached entry list for a directory handle.
func (j *JanusFS) Releasedir(path string, fh uint64) int {
	log.Printf("[FUSE] Releasedir(%q) fh=%d", path, fh)
	j.dirHandles.Delete(fh)
	return 0
}

// File operations

// Open resolves path to a file, reads its contents, and returns a handle.
func (j *JanusFS) Open(path string, flags int) (int, uint64) {
	inode, err := j.ext4.Walk(path)
	if err != nil {
		if errors.Is(err, ext4.ErrNotExist) {
			log.Printf("[FUSE] Open(%q) → ENOENT", path)
			return -fuse.ENOENT, 0
		}
		log.Printf("[FUSE] Open(%q) ERROR walk: %v", path, err)
		return -fuse.EIO, 0
	}

	if inode.IsDir() {
		log.Printf("[FUSE] Open(%q) → EISDIR", path)
		return -fuse.EISDIR, 0
	}

	if !inode.IsRegular() {
		// Device nodes, FIFOs, sockets — serve as empty files.
		fh := j.fileHandles.Store([]byte{})
		log.Printf("[FUSE] Open(%q) OK (non-regular, mode=0%o) fh=%d", path, inode.I_mode, fh)
		return 0, fh
	}

	data, err := j.ext4.ReadFile(inode)
	if err != nil {
		log.Printf("[FUSE] Open(%q) ERROR ReadFile: %v", path, err)
		return -fuse.EIO, 0
	}

	fh := j.fileHandles.Store(data)
	log.Printf("[FUSE] Open(%q) OK — fh=%d size=%d bytes", path, fh, len(data))
	return 0, fh
}

// Read copies bytes from a cached file into the caller's buffer.
func (j *JanusFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	data, ok := j.fileHandles.Load(fh)
	if !ok {
		log.Printf("[FUSE] Read(%q) fh=%d → EBADF", path, fh)
		return -fuse.EBADF
	}

	if ofst < 0 || int(ofst) >= len(data) {
		return 0 // EOF
	}

	n := copy(buff, data[ofst:])
	log.Printf("[FUSE] Read(%q) fh=%d ofst=%d n=%d", path, fh, ofst, n)
	return n
}

// Release frees the cached file content for an open file handle.
func (j *JanusFS) Release(path string, fh uint64) int {
	log.Printf("[FUSE] Release(%q) fh=%d", path, fh)
	j.fileHandles.Delete(fh)
	return 0
}

// Symbolic link operations

// Readlink returns the target string of a symbolic link inode.
func (j *JanusFS) Readlink(path string) (int, string) {
	inode, err := j.ext4.Walk(path)
	if err != nil {
		if errors.Is(err, ext4.ErrNotExist) {
			log.Printf("[FUSE] Readlink(%q) → ENOENT", path)
			return -fuse.ENOENT, ""
		}
		log.Printf("[FUSE] Readlink(%q) ERROR: %v", path, err)
		return -fuse.EIO, ""
	}

	if !inode.IsSymlink() {
		log.Printf("[FUSE] Readlink(%q) → EINVAL (not a symlink, mode=0%o)", path, inode.I_mode)
		return -fuse.EINVAL, ""
	}

	target, err := j.ext4.ReadSymlink(inode)
	if err != nil {
		log.Printf("[FUSE] Readlink(%q) ERROR ReadSymlink: %v", path, err)
		return -fuse.EIO, ""
	}

	log.Printf("[FUSE] Readlink(%q) → %q", path, target)
	return 0, target
}

// Write stubs — always return EROFS
// All write operations are rejected with EROFS.

func (j *JanusFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	log.Printf("[FUSE] Write(%q) → EROFS", path)
	return -fuse.EROFS
}
func (j *JanusFS) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Printf("[FUSE] Create(%q) → EROFS", path)
	return -fuse.EROFS, 0
}
func (j *JanusFS) Mkdir(path string, mode uint32) int {
	log.Printf("[FUSE] Mkdir(%q) → EROFS", path)
	return -fuse.EROFS
}
func (j *JanusFS) Unlink(path string) int {
	log.Printf("[FUSE] Unlink(%q) → EROFS", path)
	return -fuse.EROFS
}
func (j *JanusFS) Rmdir(path string) int {
	log.Printf("[FUSE] Rmdir(%q) → EROFS", path)
	return -fuse.EROFS
}
func (j *JanusFS) Rename(oldpath string, newpath string) int {
	log.Printf("[FUSE] Rename(%q → %q) → EROFS", oldpath, newpath)
	return -fuse.EROFS
}
func (j *JanusFS) Chmod(path string, mode uint32) int {
	return -fuse.EROFS
}
func (j *JanusFS) Chown(path string, uid uint32, gid uint32) int {
	return -fuse.EROFS
}
func (j *JanusFS) Truncate(path string, size int64, fh uint64) int {
	return -fuse.EROFS
}
func (j *JanusFS) Symlink(target string, newpath string) int {
	return -fuse.EROFS
}
func (j *JanusFS) Link(oldpath string, newpath string) int {
	return -fuse.EROFS
}

// Helper: inode → fuse.Stat_t

// inodeToStat maps an ext4 Inode's fields to a fuse.Stat_t.
func inodeToStat(inode *ext4.Inode, stat *fuse.Stat_t) {
	stat.Mode = uint32(inode.I_mode)
	stat.Nlink = uint32(inode.I_links_count)
	stat.Size = int64(inode.Size())
	stat.Atim = fuse.NewTimespec(time.Unix(int64(inode.I_atime), 0))
	stat.Ctim = fuse.NewTimespec(time.Unix(int64(inode.I_ctime), 0))
	stat.Mtim = fuse.NewTimespec(time.Unix(int64(inode.I_mtime), 0))
	stat.Blksize = 512
	stat.Blocks = int64(inode.I_blocks_lo)
}

// fmtErrc is a helper for human-readable FUSE error codes in log output.
func fmtErrc(errc int) string {
	if errc == 0 {
		return "OK"
	}
	return fmt.Sprintf("error(%d)", errc)
}
