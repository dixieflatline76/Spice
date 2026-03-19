package wallpaper

import (
	"context"
	"golang.org/x/time/rate"
	"sync"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
)

// Dispatcher manages heterogeneous jobs from different providers and natively
// prevents Head-Of-Line blocking starvation by pumping jobs through individual
// rate-limited goroutines before releasing them to the generic worker pool.
type Dispatcher struct {
	ctx           context.Context
	globalJobChan chan<- DownloadJob

	apiLimiterFunc     func(provider.ImageProvider) *rate.Limiter
	processLimiterFunc func(provider.ImageProvider) *rate.Limiter

	mu        sync.Mutex
	providers map[string]chan DownloadJob
	wg        *sync.WaitGroup
}

// NewDispatcher initializes a Fair Queuing Dispatcher.
func NewDispatcher(
	ctx context.Context,
	outChan chan<- DownloadJob,
	apiLim func(provider.ImageProvider) *rate.Limiter,
	procLim func(provider.ImageProvider) *rate.Limiter,
	wg *sync.WaitGroup,
) *Dispatcher {
	return &Dispatcher{
		ctx:                ctx,
		globalJobChan:      outChan,
		apiLimiterFunc:     apiLim,
		processLimiterFunc: procLim,
		providers:          make(map[string]chan DownloadJob),
		wg:                 wg,
	}
}

// Submit enqueues a job into its provider-specific pump.
func (d *Dispatcher) Submit(job DownloadJob) bool {
	if job.Provider == nil {
		// No provider rules, dump straight into global pool
		select {
		case d.globalJobChan <- job:
			return true
		case <-d.ctx.Done():
			return false
		case <-job.Ctx.Done():
			return false
		}
	}

	providerID := job.Provider.ID()

	d.mu.Lock()
	ch, exists := d.providers[providerID]
	if !exists {
		// Create buffer large enough for a typical query page (usually ~100 jobs)
		ch = make(chan DownloadJob, 200)
		d.providers[providerID] = ch
		d.wg.Add(1)
		go d.pump(job.Provider, ch)
	}
	d.mu.Unlock()

	// Push job to the isolated provider-specific queue
	select {
	case ch <- job:
		return true
	case <-d.ctx.Done():
		return false
	case <-job.Ctx.Done():
		return false
	}
}

// pump manages pacing for exactly ONE provider
func (d *Dispatcher) pump(pr provider.ImageProvider, inChan <-chan DownloadJob) {
	defer d.wg.Done()

	var apiLimiter *rate.Limiter
	if d.apiLimiterFunc != nil {
		apiLimiter = d.apiLimiterFunc(pr)
	}

	var processLimiter *rate.Limiter
	if d.processLimiterFunc != nil {
		processLimiter = d.processLimiterFunc(pr)
	}

	for {
		select {
		case <-d.ctx.Done():
			return
		case job := <-inChan:
			// Ensure context is still alive
			if job.Ctx != nil && job.Ctx.Err() != nil {
				continue
			}

			// Natively pace the job emission into the downstream queue.
			// This completely isolates any cooldown periods to this specific goroutine.
			if apiLimiter != nil {
				if job.Ctx != nil {
					_ = apiLimiter.Wait(job.Ctx)
				} else {
					_ = apiLimiter.Wait(d.ctx)
				}
			}

			// Ensure context wasn't cancelled during the first wait
			if job.Ctx != nil && job.Ctx.Err() != nil {
				continue
			}

			if processLimiter != nil {
				if job.Ctx != nil {
					_ = processLimiter.Wait(job.Ctx)
				} else {
					_ = processLimiter.Wait(d.ctx)
				}
			}

			// Job is perfectly paced. Ready for instant generic execution.
			select {
			case d.globalJobChan <- job:
			case <-d.ctx.Done():
				return
			}
		}
	}
}
