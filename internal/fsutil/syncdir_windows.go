//go:build windows

package fsutil

// SyncDir is a no-op because Go does not support syncing directory handles on
// Windows. Files are still synced before their atomic rename.
func SyncDir(string) error { return nil }
