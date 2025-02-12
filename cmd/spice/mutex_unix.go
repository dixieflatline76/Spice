//go:build !windows
// +build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/dixieflatline76/Spice/config"
)

var (
	lockFile *os.File
)

// acquireLock tries to acquire a single-instance lock (file lock on Unix).
func acquireLock() (bool, error) {
	lockFilePath := filepath.Join(os.TempDir(), config.ServiceName+".lock") // Use a lock file in /tmp
	file, err := os.OpenFile(lockFilePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return false, fmt.Errorf("failed to open lock file: %w", err)
	}

	//Try to get an exclusive lock. FcntlFlock implements a simple file locker.
	err = syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: 0,
		Start:  0,
		Len:    0, // Lock the entire file
	})

	if err != nil {
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
			// Another instance is running, lock is BUSY
			file.Close() // Close the file if we failed to acquire the lock
			return false, nil
		}
		file.Close()
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	lockFile = file
	return true, nil
}

// releaseLock releases the single-instance lock.
func releaseLock() {
	if lockFile != nil {
		//Best effort unlock
		syscall.FcntlFlock(lockFile.Fd(), syscall.F_SETLK, &syscall.Flock_t{
			Type:   syscall.F_UNLCK,
			Whence: 0,
			Start:  0,
			Len:    0, // Lock the entire file
		})
		lockFile.Close()           // Close the file
		os.Remove(lockFile.Name()) //remove lock file
	}
}
