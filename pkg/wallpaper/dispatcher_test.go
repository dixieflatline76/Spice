package wallpaper

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"golang.org/x/time/rate"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

// MockPacedProvider perfectly mimics a downstream provider with customizable rate limits
type MockPacedProvider struct {
	id         string
	apiPacing  time.Duration
	procPacing time.Duration
}

func (m *MockPacedProvider) ID() string   { return m.id }
func (m *MockPacedProvider) Name() string { return m.id }
func (m *MockPacedProvider) FetchImages(ctx context.Context, url string, page int) ([]provider.Image, error) {
	return nil, nil
}
func (m *MockPacedProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}
func (m *MockPacedProvider) ParseURL(url string) (string, error) { return "", nil }
func (m *MockPacedProvider) SupportsUserQueries() bool           { return false }
func (m *MockPacedProvider) CreateQueryPanel(sm setting.SettingsManager, url string) fyne.CanvasObject {
	return nil
}
func (m *MockPacedProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}
func (m *MockPacedProvider) GetProviderIcon() fyne.Resource { return nil }
func (m *MockPacedProvider) HomeURL() string                { return "" }
func (m *MockPacedProvider) Title() string                  { return m.id }
func (m *MockPacedProvider) Type() provider.ProviderType    { return provider.TypeOnline }

func (m *MockPacedProvider) GetAPIPacing() time.Duration     { return m.apiPacing }
func (m *MockPacedProvider) GetProcessPacing() time.Duration { return m.procPacing }

func mockLimiterFactory(pr provider.ImageProvider) *rate.Limiter {
	if paced, ok := pr.(provider.PacedProvider); ok {
		limit := paced.GetAPIPacing() + paced.GetProcessPacing()
		if limit > 0 {
			return rate.NewLimiter(rate.Every(limit), 1)
		}
	}
	return nil
}

func TestDispatcher_Fairness_NoPacing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	globalChan := make(chan DownloadJob, 100)
	var wg sync.WaitGroup

	dispatcher := NewDispatcher(ctx, globalChan, mockLimiterFactory, nil, &wg)

	provA := &MockPacedProvider{id: "A", apiPacing: 1 * time.Millisecond, procPacing: 0}
	provB := &MockPacedProvider{id: "B", apiPacing: 1 * time.Millisecond, procPacing: 0}

	// Submit 50 jobs for A
	for i := 0; i < 50; i++ {
		dispatcher.Submit(DownloadJob{Ctx: ctx, Provider: provA})
	}
	// Submit 50 jobs for B
	for i := 0; i < 50; i++ {
		dispatcher.Submit(DownloadJob{Ctx: ctx, Provider: provB})
	}

	// Read 20 jobs from the dispatcher
	receivedA := 0
	receivedB := 0

	for i := 0; i < 20; i++ {
		select {
		case job := <-globalChan:
			if job.Provider.ID() == "A" {
				receivedA++
			} else {
				receivedB++
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for jobs")
		}
	}

	// If fairness works, the pumps will race and output an interleaved mix
	// instead of cleanly outputting 20 A's.
	if receivedA == 20 || receivedB == 20 {
		t.Errorf("Expected interleaved (fair) output, but got heavily biased output (A: %d, B: %d). Pumps are not interleaving.", receivedA, receivedB)
	} else {
		t.Logf("Fairness proven: interleaved A:%d, B:%d", receivedA, receivedB)
	}
	cancel()
	time.Sleep(50 * time.Millisecond) // Let pumps exit cleanly
}

func TestDispatcher_NoHeadOfLineBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	globalChan := make(chan DownloadJob, 100)
	var wg sync.WaitGroup

	dispatcher := NewDispatcher(ctx, globalChan, mockLimiterFactory, nil, &wg)

	provSlow := &MockPacedProvider{id: "Slow", apiPacing: 150 * time.Millisecond, procPacing: 0}
	provFast := &MockPacedProvider{id: "Fast", apiPacing: 0, procPacing: 0}

	// 1. Submit 3 Slow jobs
	for i := 0; i < 3; i++ {
		dispatcher.Submit(DownloadJob{Ctx: ctx, Provider: provSlow})
	}

	// Let the slow worker pick up the first job and begin its 150ms sleep
	time.Sleep(20 * time.Millisecond)

	// 2. Submit 10 Fast jobs while the slow worker is deeply blocked
	for i := 0; i < 10; i++ {
		dispatcher.Submit(DownloadJob{Ctx: ctx, Provider: provFast})
	}

	// 3. Read 10 jobs. They should ALL be Fast jobs, and they should arrive INSTANTLY (< 50ms)
	fastCount := 0
	slowCount := 0

	start := time.Now()
	// We expect precisely 10 Fast jobs and 1 Slow job (due to the Limiter's initial 1-token burst).
	for i := 0; i < 11; i++ {
		select {
		case job := <-globalChan:
			if job.Provider.ID() == "Fast" {
				fastCount++
			} else if job.Provider.ID() == "Slow" {
				slowCount++
			}
		case <-time.After(50 * time.Millisecond):
			t.Fatalf("Head-of-Line blocking detected: Timeout reading fast jobs because the slow provider stalled the pipeline.")
		}
	}
	elapsed := time.Since(start)

	if fastCount != 10 {
		t.Errorf("Expected 10 Fast jobs bypassing the slow pipeline, got %d", fastCount)
	}
	if slowCount != 1 {
		t.Errorf("Expected 1 Slow burst job, got %d", slowCount)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("HOL blocking detected: Fast jobs took %v to process, expected < 100ms", elapsed)
	}

	t.Logf("HOL Prevention proven: 10 Fast jobs bypassed 3 Slow jobs in %v", elapsed)
	cancel()
}

func TestDispatcher_ObservedRate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	globalChan := make(chan DownloadJob, 100)
	var wg sync.WaitGroup

	dispatcher := NewDispatcher(ctx, globalChan, mockLimiterFactory, nil, &wg)

	pacing := 100 * time.Millisecond
	prov := &MockPacedProvider{id: "Paced", apiPacing: 0, procPacing: pacing}

	// Submit 5 jobs instantly
	for i := 0; i < 5; i++ {
		dispatcher.Submit(DownloadJob{Ctx: ctx, Provider: prov})
	}

	var timestamps []time.Time
	for i := 0; i < 5; i++ {
		select {
		case <-globalChan:
			timestamps = append(timestamps, time.Now())
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for pacing jobs")
		}
	}

	// Calculate deltas
	// The first job (index 0) fires instantly due to limiter burst
	for i := 1; i < len(timestamps); i++ {
		delta := timestamps[i].Sub(timestamps[i-1])
		// Due to runtime scheduling, allow a small 10ms tolerance underneath the target
		if delta < pacing-(15*time.Millisecond) {
			t.Errorf("Jobs processed too quickly! Expected pacing ~%v, observed %v between job %d and %d", pacing, delta, i-1, i)
		} else {
			t.Logf("Job %d -> %d adhered to pacing: %v", i-1, i, delta)
		}
	}
}
