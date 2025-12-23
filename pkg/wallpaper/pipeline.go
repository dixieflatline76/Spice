package wallpaper

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
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
	store      *ImageStore
	processor  ProcessFunc
}

// DownloadJob represents a task to download and process an image.
type DownloadJob struct {
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
func NewPipeline(cfg *Config, store *ImageStore, processor ProcessFunc) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
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
// Returns false if pipeline is stopped or full (though buffer is large).
func (p *Pipeline) Submit(job DownloadJob) bool {
	select {
	case p.jobChan <- job:
		return true
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
		case job := <-p.jobChan:
			// Process the job
			processedImg, err := p.processor(p.ctx, job)
			p.resultChan <- ProcessResult{Image: processedImg, Error: err}
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
				if strings.Contains(res.Error.Error(), "avoid set") {
					log.Debugf("Pipeline: %v", res.Error)
				} else if strings.Contains(res.Error.Error(), "smart fit") {
					log.Debugf("Pipeline: %v", res.Error)
				} else {
					log.Printf("Pipeline Error: %v", res.Error)
				}
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
