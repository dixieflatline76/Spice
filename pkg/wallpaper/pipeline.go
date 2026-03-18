package wallpaper

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Pipeline manages a pool of workers to process image downloads.
type Pipeline struct {
	jobChan    chan DownloadJob
	resultChan chan ProcessResult
	cmdChan    chan StateCmd
	workerWg   sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	config     *Config
	store      StoreInterface
	processor  ProcessFunc
}

// DownloadJob represents a task to download and process an image.
type DownloadJob struct {
	Ctx      context.Context
	Image    provider.Image
	Provider provider.ImageProvider
}

// ProcessResult represents the result of a processed image.
type ProcessResult struct {
	Image provider.Image
	Error error
}

// ProcessFunc is the function signature for processing a job.
type ProcessFunc func(ctx context.Context, job DownloadJob) (provider.Image, error)

type CmdType int

const (
	CmdMarkSeen CmdType = iota
	CmdRemove
	CmdClear
)

type StateCmd struct {
	Type    CmdType
	Payload interface{}
}

// NewPipeline creates a new pipeline with the given configuration and store.
func NewPipeline(ctx context.Context, cfg *Config, store StoreInterface, processor ProcessFunc) *Pipeline {
	ctx, cancel := context.WithCancel(ctx)
	return &Pipeline{
		jobChan:    make(chan DownloadJob, 100), // Buffered job channel
		resultChan: make(chan ProcessResult, 100),
		cmdChan:    make(chan StateCmd, 100),
		ctx:        ctx,
		cancel:     cancel,
		config:     cfg,
		store:      store,
		processor:  processor,
	}
}

// Start starts the worker pool.
func (p *Pipeline) Start(workerCount int) {
	log.Printf("Starting Pipeline with %d workers", workerCount)
	for i := 0; i < workerCount; i++ {
		p.workerWg.Add(1)
		go p.workerLoop(i)
	}

	// Start a collector goroutine to process results and add them to store
	// Start a collector goroutine to process results and add them to store
	go p.stateManagerLoop()
}

// Stop stops the pipeline and waits for workers to finish.
func (p *Pipeline) Stop() {
	log.Println("Stopping Pipeline...")
	p.cancel() // Signal cancellation
	p.workerWg.Wait()
	close(p.resultChan) // Close result channel after workers are done
	log.Println("Pipeline Stopped.")
}

// Submit submits a job to the pipeline.
// Returns false if pipeline is stopped or if the provided context is cancelled.
func (p *Pipeline) Submit(ctx context.Context, job DownloadJob) bool {
	select {
	case p.jobChan <- job:
		return true
	case <-ctx.Done():
		return false
	case <-p.ctx.Done():
		return false
	}
}

// workerLoop is the main loop for a worker goroutine.
func (p *Pipeline) workerLoop(id int) {
	defer p.workerWg.Done()
	log.Debugf("Worker %d started", id)

	for {
		select {
		case <-p.ctx.Done():
			log.Debugf("Worker %d stopping", id)
			return
		default:
			select {
			case <-p.ctx.Done():
				log.Debugf("Worker %d stopping", id)
				return
			case job := <-p.jobChan:
				// Ensure context exists (Testing safety net)
				if job.Ctx == nil {
					job.Ctx = p.ctx
				}

				// Process the job using the job's context rather than the global pipeline context
				processedImg, err := p.processor(job.Ctx, job)
				p.resultChan <- ProcessResult{Image: processedImg, Error: err}
			}
		}
	}
}

// StateManagerLoop is the single consumer that updates the store.
func (p *Pipeline) stateManagerLoop() {
	for {
		select {
		case res, ok := <-p.resultChan:
			if !ok {
				// Result channel closed, drain commands?
				// For now, if result chan closes, we assume pipeline stopping.
				return
			}
			if res.Error != nil {
				p.logPipelineError(res.Error)
				continue
			}
			p.store.Add(res.Image)

		case cmd := <-p.cmdChan:
			switch cmd.Type {
			case CmdMarkSeen:
				p.store.MarkSeen(cmd.Payload.(string))
			case CmdRemove:
				id := cmd.Payload.(string)
				p.store.Remove(id)
				// log.Debugf("Pipeline: Removed image %s from store.", id)
			case CmdClear:
				p.store.Clear()
				log.Printf("Pipeline: Store cleared.")
			}
		case <-p.ctx.Done():
			return
		}
		// Yield to allow Readers (UI) to acquire RLock
		runtime.Gosched()
	}
}

// SendCommand sends a state mutation command to the pipeline manager.
func (p *Pipeline) SendCommand(cmd StateCmd) {
	select {
	case p.cmdChan <- cmd:
	case <-p.ctx.Done():
	}
}

// logPipelineError categorizes and logs errors, ignoring expected ones.
func (p *Pipeline) logPipelineError(err error) {
	errMsg := err.Error()
	if strings.Contains(errMsg, "avoid set") {
		log.Debugf("Pipeline: %v", err)
	} else if strings.Contains(errMsg, "smart fit") || strings.Contains(errMsg, "aspect ratio") || strings.Contains(errMsg, "image resolution too low") {
		log.Debugf("Pipeline: %v", err)
	} else if strings.Contains(errMsg, "status 429") || strings.Contains(errMsg, "enrichment") {
		log.Debugf("Pipeline: %v", err)
	} else if strings.Contains(errMsg, "incompatible") {
		log.Debugf("Pipeline: %v", err)
	} else {
		log.Printf("Pipeline Error: %v", err)
	}
}
