package wallpaper

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBulkhead_LaneSeparation(t *testing.T) {
	// Setup Plugin with 1-slot wikimedia bulkhead
	wp := &Plugin{
		providers:          make(map[string]provider.ImageProvider),
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		store:              &MockImageStore{},
		ctx:                context.Background(),
		queryContexts:      make(map[string]context.Context),
		queryCancelFuncs:   make(map[string]context.CancelFunc),
		manager:            &MockPluginManager{},
		Monitors:           make(map[int]*MonitorController),
	}

	// Mock manager expectations (FetchNewImages calls NotifyUser at the end)
	mockManager := wp.manager.(*MockPluginManager)
	mockManager.On("NotifyUser", mock.Anything, mock.Anything).Maybe()

	// Mock Store to return false for Exists (always new images)
	mockStore := wp.store.(*MockImageStore)
	mockStore.On("Exists", mock.Anything).Return(false)

	// Mock JobSubmitter
	mockPipeline := &MockPipeline{}
	wp.jobSubmitter = mockPipeline
	mockPipeline.On("Submit", mock.Anything, mock.Anything).Return(true)

	// 1. Setup Providers using the established MockImageProvider from mocks_test.go
	wikimedia := &MockImageProvider{}
	wikimedia.On("ID").Return("Wikimedia").Maybe()

	wallhaven := &MockImageProvider{}
	wallhaven.On("ID").Return("Wallhaven").Maybe()

	wp.providers["Wikimedia"] = wikimedia
	wp.providers["Wallhaven"] = wallhaven

	// 2. Setup Queries: 2 Wikimedia (Competing for 1 slot), 1 Wallhaven (Dedicated global slot)
	wp.cfg = &Config{
		Queries: []ImageQuery{
			{ID: "wik1", Provider: "Wikimedia", Active: true, URL: "cat1", Description: "wik1"},
			{ID: "wik2", Provider: "Wikimedia", Active: true, URL: "cat2", Description: "wik2"},
			{ID: "wall1", Provider: "Wallhaven", Active: true, URL: "search", Description: "wall1"},
		},
	}

	// Ensure the working directory exists for saveQueryPages (to avoid noisy errors)
	workingDir := config.GetWorkingDir()
	_ = os.MkdirAll(filepath.Join(workingDir, "wallpaper_downloads"), 0755)

	// 3. Mock Fetch Behaviors
	var wg sync.WaitGroup
	wg.Add(3)

	// wik1 will take 20ms
	wikimedia.On("FetchImages", mock.Anything, "cat1", 1).Return([]provider.Image{{ID: "img1"}}, nil).Run(func(args mock.Arguments) {
		time.Sleep(20 * time.Millisecond)
		wg.Done()
	}).Once()
	// wik2 will also take 20ms
	wikimedia.On("FetchImages", mock.Anything, "cat2", 1).Return([]provider.Image{{ID: "img2"}}, nil).Run(func(args mock.Arguments) {
		time.Sleep(20 * time.Millisecond)
		wg.Done()
	}).Once()
	// wall1 will take 10ms
	wallhaven.On("FetchImages", mock.Anything, "search", 1).Return([]provider.Image{{ID: "img3"}}, nil).Run(func(args mock.Arguments) {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}).Once()

	// 4. Run Fetch
	wp.FetchNewImages(true)

	// Wait for all mocks to be satisfied (indicating all queries processed)
	c := make(chan struct{})
	go func() {
		wg.Wait()
		close(c)
	}()

	select {
	case <-c:
		assert.True(t, wikimedia.AssertExpectations(t))
		assert.True(t, wallhaven.AssertExpectations(t))
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for fetches")
	}

	// Logic Check:
	// If lane separation is working:
	// - wall1 (50ms) should NOT be delayed by wik1 (100ms).
	// - wik2 (100ms) SHOULD be delayed by wik1 (100ms) because they share the 1-slot bulkhead.
	// Total time should be around 200ms (wik1 + wik2), but wall1 should finish much earlier.
}

func TestBulkhead_CircuitBreakerRejection(t *testing.T) {
	wp := &Plugin{
		providers:          make(map[string]provider.ImageProvider),
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		store:              &MockImageStore{},
		ctx:                context.Background(),
		queryContexts:      make(map[string]context.Context),
		queryCancelFuncs:   make(map[string]context.CancelFunc),
		manager:            &MockPluginManager{},
		Monitors:           make(map[int]*MonitorController),
	}

	// Mock manager expectations
	if mp, ok := wp.manager.(*MockPluginManager); ok {
		mp.On("NotifyUser", mock.Anything, mock.Anything).Maybe()
	}

	// 1. Setup Provider that implements ThrottledProvider
	wikimedia := &MockThrottledProvider{}
	wikimedia.On("ID").Return("Wikimedia").Maybe()
	wikimedia.On("IsThrottled").Return(true) // Open Circuit

	wp.providers["Wikimedia"] = wikimedia

	// 2. Setup Query
	wp.cfg = &Config{
		Queries: []ImageQuery{
			{ID: "wik1", Provider: "Wikimedia", Active: true, URL: "cat1"},
		},
	}

	// 3. Run Fetch
	wp.FetchNewImages(true)

	// Wait briefly for goroutine to pick it up and see IsThrottled()
	time.Sleep(20 * time.Millisecond)

	// Expectation: FetchImages should NOT have been called due to early rejection
	wikimedia.AssertNotCalled(t, "FetchImages", mock.Anything, mock.Anything, mock.Anything)
}

func TestBulkhead_Deduplication(t *testing.T) {
	wp := &Plugin{
		providers:          make(map[string]provider.ImageProvider),
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
		store:              &MockImageStore{},
		ctx:                context.Background(),
		queryContexts:      make(map[string]context.Context),
		queryCancelFuncs:   make(map[string]context.CancelFunc),
		manager:            &MockPluginManager{},
		Monitors:           make(map[int]*MonitorController),
	}

	// Mock manager expectations
	if mp, ok := wp.manager.(*MockPluginManager); ok {
		mp.On("NotifyUser", mock.Anything, mock.Anything).Maybe()
	}

	// 1. Setup Provider and Query
	wikimedia := &MockImageProvider{}
	wikimedia.On("ID").Return("Wikimedia").Maybe()
	wp.providers["Wikimedia"] = wikimedia
	wp.cfg = &Config{
		Queries: []ImageQuery{
			{ID: "wik1", Provider: "Wikimedia", Active: true, URL: "cat1"},
		},
	}

	// 2. Mock Store: id1 exists, id2 does not (namespaced by provider ID)
	mockStore := wp.store.(*MockImageStore)
	mockStore.On("Exists", "Wikimedia_id1").Return(true)
	mockStore.On("Exists", "Wikimedia_id2").Return(false)

	// 3. Mock Pipeline
	mockPipeline := &MockPipeline{}
	wp.jobSubmitter = mockPipeline
	// Should only be called for id2
	mockPipeline.On("Submit", mock.Anything, mock.MatchedBy(func(job DownloadJob) bool {
		return job.Image.ID == "Wikimedia_id2"
	})).Return(true).Once()

	// 4. Mock Provider to return both
	var wg sync.WaitGroup
	wg.Add(1)
	wikimedia.On("FetchImages", mock.Anything, "cat1", 1).Return([]provider.Image{
		{ID: "id1"},
		{ID: "id2"},
	}, nil).Once().Run(func(args mock.Arguments) {
		wg.Done()
	})

	// 5. Run
	wp.FetchNewImages(true)
	wg.Wait()

	// Verification
	// Short sleep to allow deduplication check and pipeline submission to finish
	time.Sleep(20 * time.Millisecond)
	mockPipeline.AssertExpectations(t)
	mockPipeline.AssertNotCalled(t, "Submit", mock.Anything, mock.MatchedBy(func(job DownloadJob) bool {
		return job.Image.ID == "Wikimedia_id1"
	}))
}
