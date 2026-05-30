//go:build !linux

package wallpaper

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestDeactivate_SuppressesNotificationDuringShutdown verifies that when
// Deactivate() is called while a fetch is in progress, the "Downloading X
// new images..." notification is NOT sent to the user.
//
// This is a regression test for the quit-notification race:
// 1. FetchNewImages starts with a slow provider
// 2. Deactivate() is called mid-fetch
// 3. The provider completes and queues images
// 4. The notification should be suppressed because interrupt is set
func TestDeactivate_SuppressesNotificationDuringShutdown(t *testing.T) {
	// Track whether the provider's FetchImages was actually called
	var fetchStarted atomic.Bool

	// Setup Plugin
	ctx, cancel := context.WithCancel(context.Background())
	wp := &Plugin{
		providers:          make(map[string]provider.ImageProvider),
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		interrupt:          util.NewSafeBool(),
		store:              &MockImageStore{},
		ctx:                ctx,
		cancel:             cancel,
		queryContexts:      make(map[string]context.Context),
		queryCancelFuncs:   make(map[string]context.CancelFunc),
		manager:            &MockPluginManager{},
		Monitors:           make(map[int]*MonitorController),
		os:                 &MockOS{},
	}

	// Mock manager — track NotifyUser calls
	mockManager := wp.manager.(*MockPluginManager)
	// We deliberately do NOT set .Maybe() here so we can assert it was NOT called
	// with the fetch notification title.

	// Mock Store
	mockStore := wp.store.(*MockImageStore)
	mockStore.On("GetByID", mock.Anything).Return(provider.Image{}, false)

	// Mock OS
	wp.os.(*MockOS).On("GetMonitors").Return([]Monitor{}, nil).Maybe()

	// Mock Pipeline — accepts all jobs
	mockPipeline := &MockPipeline{}
	wp.jobSubmitter = mockPipeline
	mockPipeline.On("Submit", mock.Anything, mock.Anything).Return(true)

	// Setup slow provider: takes 300ms to respond (simulates active HTTP call)
	slowProvider := &MockImageProvider{}
	slowProvider.On("ID").Return("SlowMuseum").Maybe()
	slowProvider.On("FetchImages", mock.Anything, "slow-query", 1).
		Return([]provider.Image{
			{ID: "img1", Path: "https://example.com/1.jpg"},
			{ID: "img2", Path: "https://example.com/2.jpg"},
			{ID: "img3", Path: "https://example.com/3.jpg"},
		}, nil).
		Run(func(args mock.Arguments) {
			fetchStarted.Store(true)
			time.Sleep(300 * time.Millisecond) // Simulate slow HTTP call
		}).Once()

	wp.providers["SlowMuseum"] = slowProvider

	// Config with one active query
	wp.cfg = &Config{
		Queries: []ImageQuery{
			{ID: "q1", Provider: "SlowMuseum", Active: true, URL: "slow-query", Description: "Slow"},
		},
	}

	// Ensure working dir exists
	workingDir := config.GetWorkingDir()
	_ = os.MkdirAll(filepath.Join(workingDir, "wallpaper_downloads"), 0755)

	// --- ACT ---

	// 1. Start fetch (runs in goroutine)
	wp.FetchNewImages(true)

	// 2. Wait until the provider is actually mid-call
	assert.Eventually(t, func() bool {
		return fetchStarted.Load()
	}, 2*time.Second, 10*time.Millisecond, "Provider FetchImages should have started")

	// 3. Deactivate while fetch is in progress
	wp.Deactivate()

	// 4. Wait for the fetch goroutine to complete (it will because the
	//    provider sleep finishes after 300ms)
	time.Sleep(500 * time.Millisecond)

	// --- ASSERT ---

	// The notification should NOT have been sent because interrupt was set
	// during Deactivate() before the fetch goroutine reached the notification.
	mockManager.AssertNotCalled(t, "NotifyUser", "Wallpaper Fetch", mock.Anything)
}
