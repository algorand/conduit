package pipeline

import (
	"time"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/metrics"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
)

func (p *pipelineImpl) ImportHandler(importer importers.Importer, inChan <-chan uint64, outChan chan<- data.BlockData, errChan chan<- error) {
	select {
	case <-p.ctx.Done():
	case rnd := <-inChan:
		var lastError error
		retry := uint64(0)
		for retry > p.cfg.RetryCount && p.cfg.RetryCount != 0 {
			importStart := time.Now()
			blkData, err := importer.GetBlock(rnd)
			if err != nil {
				p.logger.Errorf("%v", err)
				retry++
				lastError = err
				continue
			}

			metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())
			outChan <- blkData
			lastError = nil
		}
		if lastError != nil {
			errChan <- lastError
			return
		}
	}
}

func (p *pipelineImpl) ProcessorHandler(importer processors.Processor, inChan <-chan data.BlockData, outChan chan<- data.BlockData, errChan chan<- error) {
}

func (p *pipelineImpl) ExporterHandler(importer exporters.Exporter, inChan <-chan data.BlockData, errChan chan<- error) {
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) WStart() {
	// Setup channels
	roundChan := make(chan uint64)
	processorBlkInChan := make(chan data.BlockData)
	errChan := make(chan error)
	p.ImportHandler(p.importer, roundChan, processorBlkInChan, errChan)

	var processorBlkOutChan chan data.BlockData
	for _, proc := range p.processors {
		processorBlkOutChan = make(chan data.BlockData)
		p.ProcessorHandler(proc, processorBlkInChan, processorBlkOutChan, errChan)
		processorBlkInChan = processorBlkOutChan
	}

	p.ExporterHandler(p.exporter, processorBlkOutChan, errChan)

	p.wg.Add(1)
	// Main loop
	go func(startRound uint64) {
		defer p.wg.Done()
		defer close(errChan)
		rnd := startRound
		for {
			select {
			case <-p.ctx.Done():
				return
			case roundChan <- rnd:
				rnd++
			}
		}
	}(p.pipelineMetadata.NextRoundDEPRECATED)

	// Check for errors
	for err := range errChan {
		if err != nil {
			p.logger.Errorf("%v", err)
			p.setError(err) // TODO: probably it's better to p.joinError(...)
			return
		}
	}
}
