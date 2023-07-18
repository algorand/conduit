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
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		lastRnd := uint64(0)
		for {
			select {
			case <-p.ctx.Done():
				p.logger.Infof("importer handler exiting. lastRnd=%d", lastRnd)
				return
			case rnd := <-inChan:
				p.logger.Debugf("importing round %d", rnd)
				lastRnd = rnd
				var lastError error
				retry := uint64(0)
				for p.cfg.RetryCount == 0 || retry <= p.cfg.RetryCount {
					importStart := time.Now()
					blkData, err := importer.GetBlock(rnd)
					if err != nil {
						p.logger.Errorf("%v", err)
						retry++
						lastError = err
						continue
					}

					metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())
					select {
					case <-p.ctx.Done():
						return
					case outChan <- blkData:
					}
					lastError = nil
					break
				}
				if lastError != nil {
					errChan <- lastError
					return
				}
			}
		}
	}()
}

func (p *pipelineImpl) ProcessorHandler(proc processors.Processor, inChan <-chan data.BlockData, outChan chan<- data.BlockData, errChan chan<- error) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		lastRnd := uint64(0)
		for {
			select {
			case <-p.ctx.Done():
				p.logger.Infof("processor handler exiting lastRnd=%d", lastRnd)
				return
			case blkData := <-inChan:
				p.logger.Debugf("processing round %d", blkData.Round())
				lastRnd = blkData.Round()
				var lastError error
				retry := uint64(0)
				for p.cfg.RetryCount == 0 || retry <= p.cfg.RetryCount {
					processorStart := time.Now()
					blkData, err := proc.Process(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						retry++
						lastError = err
						continue
					}

					metrics.ProcessorTimeSeconds.WithLabelValues(proc.Metadata().Name).Observe(time.Since(processorStart).Seconds())
					select {
					case <-p.ctx.Done():
						return
					case outChan <- blkData:
					}
					lastError = nil
					break
				}
				if lastError != nil {
					errChan <- lastError
					return
				}
			}
		}
	}()
}

func (p *pipelineImpl) ExporterHandler(exporter exporters.Exporter, inChan <-chan data.BlockData, errChan chan<- error) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		lastRnd := uint64(0)
		for {
			select {
			case <-p.ctx.Done():
				p.logger.Infof("exporter handler exiting lastRnd=%d", lastRnd)
				return
			case blkData := <-inChan:
				p.logger.Debugf("exporting round %d", blkData.Round())
				lastRnd = blkData.Round()
				var lastError error
				retry := uint64(0)
				for p.cfg.RetryCount == 0 || retry <= p.cfg.RetryCount {
					exporterStart := time.Now()
					err := exporter.Receive(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						retry++
						lastError = err
						continue
					}

					// Increment Round, update metadata
					p.pipelineMetadata.NextRoundDEPRECATED++
					err = p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
					if err != nil {
						p.logger.Errorf("%v", err)
					}

				callbacksLoop:
					for _, cb := range p.completeCallback {
						err = cb(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							lastError = err
							break callbacksLoop
						}
					}

					metrics.ExporterTimeSeconds.Observe(time.Since(exporterStart).Seconds())
					lastError = nil
					break
				}
				if lastError != nil {
					errChan <- lastError
					return
				}
			}
		}
	}()
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) Start() {
	p.logger.Debug("Pipeline.Start()")

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
			p.logger.Debugf("pushing round %d", rnd)
			select {
			case <-p.ctx.Done():
				p.logger.Infof("round channel feed exiting. lastRnd=%d", rnd)
				return
			case roundChan <- rnd:
				rnd++
			}
		}
	}(p.pipelineMetadata.NextRoundDEPRECATED)

	select {
	case <-p.ctx.Done():
		return
	case err := <-errChan:
		if err != nil {
			p.logger.Errorf("%v", err)
			p.setError(err)
		} else {
			p.logger.Errorf("this is strange: Pipeline completed via nil error")
		}
	}
}
