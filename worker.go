package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type ClamResult byte

const (
	ClamOK ClamResult = iota
	ClamFOUND
	ClamERR
)

type ClamConfiguration struct {
	Protocol string
	Address  string
	Port     int
	Timeout  time.Duration
}

type ClamSubmission struct {
	Path   string
	Result chan ClamResult
}

// ClamWorker - the worker threads that actually process the jobs
type ClamWorker struct {
	logger *Logger

	config     ClamConfiguration
	connection net.Conn

	done          sync.WaitGroup
	ready         chan chan *ClamSubmission
	assignedFiles chan *ClamSubmission

	quit chan bool
}

// ScanQueue - a queue for enqueueing jobs to be processed
type ScanQueue struct {
	logger *Logger

	internal chan *ClamSubmission

	ready   chan chan *ClamSubmission
	workers []*ClamWorker

	dispatcherStopped sync.WaitGroup
	workersStopped    sync.WaitGroup

	quit chan bool
}

// NewScanQueue - creates a new job queue
func NewScanQueue(logger *Logger, maxWorkers int, config ClamConfiguration) (*ScanQueue, error) {
	workersStopped := sync.WaitGroup{}
	ready := make(chan chan *ClamSubmission, maxWorkers)
	workers := make([]*ClamWorker, maxWorkers, maxWorkers)

	for i := 0; i < maxWorkers; i++ {
		worker, err := NewClamWorker(logger, ready, workersStopped, config)
		if err != nil {
			return nil, err
		}
		workers[i] = worker
	}

	return &ScanQueue{
		logger:            logger,
		internal:          make(chan *ClamSubmission, maxWorkers),
		ready:             ready,
		workers:           workers,
		dispatcherStopped: sync.WaitGroup{},
		workersStopped:    workersStopped,
		quit:              make(chan bool),
	}, nil
}

// Start - starts the worker routines and dispatcher routine
func (q *ScanQueue) Start() {
	for i := 0; i < len(q.workers); i++ {
		q.workers[i].Start()
	}
	go q.dispatch()
}

// Stop - stops the workers and sispatcher routine
func (q *ScanQueue) Stop() {
	q.quit <- true
	q.dispatcherStopped.Wait()
}

func (q *ScanQueue) dispatch() {
	q.dispatcherStopped.Add(1)
	for {
		select {
		case job := <-q.internal: // We got something in on our queue
			workerChannel := <-q.ready // Check out an available worker
			workerChannel <- job       // Send the request to the channel
			break
		case <-q.quit:
			for i := 0; i < len(q.workers); i++ {
				q.workers[i].Stop()
			}
			q.workersStopped.Wait()
			q.dispatcherStopped.Done()
			return
		}
	}
}

// Submit - adds a new job to be processed
func (q *ScanQueue) Submit(path string) chan ClamResult {
	result := make(chan ClamResult)
	q.internal <- &ClamSubmission{
		Path:   path,
		Result: result,
	}
	return result
}

// NewClamWorker - creates a new worker
func NewClamWorker(logger *Logger, ready chan chan *ClamSubmission, done sync.WaitGroup, config ClamConfiguration) (*ClamWorker, error) {
	worker := &ClamWorker{
		logger:        logger,
		config:        config,
		done:          done,
		ready:         ready,
		assignedFiles: make(chan *ClamSubmission),
		quit:          make(chan bool),
	}

	if err := worker.Connect(); err != nil { // open a connection check, then close it
		return nil, err
	}
	worker.connection.Close()
	return worker, nil
}

func (w *ClamWorker) Connect() error {
	var conn net.Conn
	var err error

	switch w.config.Protocol {
	case "tcp":
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", w.config.Address, w.config.Port), w.config.Timeout)
	case "unix":
		conn, err = net.Dial("unix", w.config.Address)
	default:
		return fmt.Errorf("Unsupported protocol: '%s'\n", w.config.Protocol)
	}

	if err != nil {
		return err
	}

	w.connection = conn
	return nil
}

// Start - begins the job processing loop for the worker
func (w *ClamWorker) Start() {
	go func() {
		w.done.Add(1)
		for {
			w.ready <- w.assignedFiles // check the job queue in
			select {
			case submission := <-w.assignedFiles: // see if anything has been assigned to the queue
				w.Process(submission)
				break
			case <-w.quit:
				w.done.Done()
				return
			}
		}
	}()
}

// Stop - stops the worker
func (w *ClamWorker) Stop() {
	w.quit <- true
	w.connection.Close()
}

// Process - actually do the file processing
func (w *ClamWorker) Process(submission *ClamSubmission) {
	w.Connect()
	defer w.connection.Close()
	file, err := os.Open(submission.Path)
	result := bufio.NewReader(w.connection)
	buffer := make([]byte, 2048)
	var data string

	defer file.Close()
	if err != nil {
		submission.Result <- ClamERR
		return
	}

	w.logger.Debug("Submitting: %s\n", submission.Path)

	if _, err := w.connection.Write([]byte("nINSTREAM\n")); err != nil {
		w.logger.Debug("INSTREAM write error")
		submission.Result <- ClamERR
		return
	}

	for {
		readSize, err := file.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			w.logger.Error("Error while reading file '%s': %s", submission.Path, err.Error())
			submission.Result <- ClamERR
			return
		}

		if readSize > 0 {
			if err = binary.Write(w.connection, binary.BigEndian, uint32(readSize)); err != nil {
				w.logger.Info("size write error %s, %d, %s", submission.Path, readSize, err.Error())
				submission.Result <- ClamERR
				return
			}
			if _, err = w.connection.Write(buffer[0:readSize]); err != nil {
				w.logger.Info("data write error %s, %d, %s", submission.Path, readSize, err.Error())
				submission.Result <- ClamERR
				return
			}
		}
	}

	binary.Write(w.connection, binary.BigEndian, uint32(0))

	data, err = result.ReadString('\n')
	if err != nil {
		w.logger.Debug("read error")
		submission.Result <- ClamERR
		return
	}

	if strings.Contains(data, "FOUND") {
		submission.Result <- ClamFOUND
		return
	}
	if strings.Contains(data, "ERROR") {
		submission.Result <- ClamERR
		return
	}
	submission.Result <- ClamOK
	return
}
