package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/service"
	"github.com/dixieflatline76/Spice/ui"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// CreateMutex creates a new mutex with the given name.
func CreateMutex(name string) (*windows.Handle, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	mutex, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		return nil, fmt.Errorf("failed to create mutex: %w", err)
	}

	return &mutex, nil
}

func main() {
	// Create a mutex to ensure only one instance of the application is running at a time
	mutex, err := CreateMutex(config.ServiceName + "_SingleInstanceMutex")
	if err != nil {
		log.Fatalf("Another instance of %v is already running.", config.ServiceName)
	}
	defer windows.ReleaseMutex(*mutex)
	defer windows.CloseHandle(*mutex)

	// Wait for the mutex to be released
	waitResult, err := windows.WaitForSingleObject(*mutex, 100)
	if err != nil {
		log.Fatalf("Failed to wait for mutex: %v", err)
	}
	if waitResult == uint32(windows.WAIT_TIMEOUT) {
		// Mutex is already held, another instance is running
		fmt.Println("Another instance of Wallhavener is already running.")
		return
	}

	config.LoadConfig()
	fmt.Println("LoadConfig done")
	log.Printf("API Key: %v", config.Cfg.APIKey)
	log.Printf("Frequency: %v", config.Cfg.Frequency)

	isSrvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("failed to determine if we are running in an interactive session: %v", err)
	}

	if isSrvc {
		go service.RunService(config.ServiceName, false) // Run the service in the background
	} else {
		// Handle command-line arguments for install/remove
		if len(os.Args) > 1 {
			switch os.Args[1] {
			case "install":
				err = service.InstallService(config.ServiceName, config.ServiceName+" Service")
			case "remove":
				err = service.RemoveService(config.ServiceName)
			default:
				err = fmt.Errorf("invalid command: %s", os.Args[1])
			}
			if err != nil {
				log.Fatalf("failed to %s %s: %v", os.Args[1], config.ServiceName, err)
			}
			return
		}

		// If no command-line arguments, start the service in debug mode
		go service.RunService(config.ServiceName, true)
	}

	// Create the Fyne application
	a := ui.GetInstance()
	if a != nil {
		a.Run() // Run the Fyne application
	}
}
