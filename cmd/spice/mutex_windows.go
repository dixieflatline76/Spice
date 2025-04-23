//go:build windows
// +build windows

package main

import (
	"errors"
	"syscall"

	"github.com/dixieflatline76/Spice/util/log"

	"github.com/dixieflatline76/Spice/config"
	"golang.org/x/sys/windows"
)

var (
	mutex windows.Handle
)

// acquireLock tries to acquire a single-instance lock (mutex on Windows).
func acquireLock() (bool, error) {
	namePtr, err := syscall.UTF16PtrFromString(config.AppName + "_SingleInstanceMutex")
	if err != nil {
		return false, err
	}

	mutex, err = windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		// Check if the error is because the mutex already exists.
		if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
			return false, nil // Another instance is running
		}
		return false, errors.New("another instance is already running")
	}

	return true, nil
}

// releaseLock releases the single-instance lock handle.
func releaseLock() {
	if mutex != 0 { // Important check
		// Remove this call - ReleaseMutex is not needed when only using CreateMutex for existence check
		/*
		   err := windows.ReleaseMutex(mutex)
		   if err != nil {
		       log.Printf("Failed to release mutex %v", err)
		   }
		*/
		err := windows.CloseHandle(mutex) // This is the important cleanup call
		if err != nil {
			log.Printf("Failed to close mutex handle: %v", err)
		}
		mutex = 0 // Optional: Clear the handle variable after closing
	}
}
