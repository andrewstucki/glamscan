package main

import (
	"crypto/md5"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boltdb/bolt"
)

type ClamScanner struct {
	directory string
	size      int64
	wait      time.Duration

	quit     int32
	quitting chan bool
	done     chan bool
	logger   *Logger
	database *bolt.DB
	queue    *ScanQueue
}

func md5File(path string) ([]byte, error) {
	var result []byte
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return result, err
	}

	return hash.Sum(result), nil
}

func NewClamScanner(directory string, size int64, wait time.Duration, logger *Logger, database *bolt.DB, queue *ScanQueue) *ClamScanner {
	return &ClamScanner{
		directory: directory,
		size:      size,
		wait:      wait,
		quit:      0,
		quitting:  make(chan bool, 1),
		done:      make(chan bool),
		logger:    logger,
		database:  database,
		queue:     queue,
	}
}

var END = errors.New("terminate")

func (c *ClamScanner) Start() {
	go func() {
		for {
			var found uint64
			var okFiles uint64
			var errored uint64
			var skipped uint64
			start := time.Now()

			if atomic.LoadInt32(&c.quit) == 0 {
				c.logger.Info("Beginning scan of '%s'", c.directory)
			}
			var wg sync.WaitGroup
			err := filepath.Walk(c.directory, func(path string, info os.FileInfo, err error) error {
				if info.IsDir() {
					return nil
				}
				if info.Size() > c.size {
					c.logger.Debug("Skipping %s due to large file size", path)
					skipped++
					return nil
				}
				if err != nil {
					return err
				}

				if atomic.LoadInt32(&c.quit) != 0 {
					// do an atomic check and see if we should return from our traversal
					return END
				}

				modificationTime := info.ModTime().String()

				var exists = false
				c.database.View(func(tx *bolt.Tx) error {
					bucket := tx.Bucket([]byte("files"))
					value := bucket.Get([]byte(path))
					if value != nil && string(value) == modificationTime {
						exists = true
					}
					return nil
				})
				if exists {
					skipped += 1
					c.logger.Debug("Skipping '%s', already scanned.", path)
				} else {
					resultChannel := c.queue.Submit(path)
					wg.Add(1)
					go func() {
						defer wg.Done()
						result := <-resultChannel
						switch result {
						case ClamOK:
							c.logger.Debug("OK: %s", path)
							atomic.AddUint64(&okFiles, 1)
							err := c.database.Update(func(tx *bolt.Tx) error {
								bucket := tx.Bucket([]byte("files"))
								err := bucket.Put([]byte(path), []byte(modificationTime))
								return err
							})
							if err != nil {
								c.logger.Error("Error while updating database for '%s': %s", path, err.Error())
							}
							break
						case ClamERR:
							atomic.AddUint64(&errored, 1)
							c.logger.Debug("Error while scanning '%s', will retry next scan.", path)
							break
						case ClamFOUND: // don't add to the scanned list because if the file is recreated we want to scan it again
							atomic.AddUint64(&found, 1)
							c.logger.Warn("Virus found at: '%s'", path)
							err := c.database.Update(func(tx *bolt.Tx) error {
								bucket := tx.Bucket([]byte("viruses"))
								hash, err := md5File(path)
								if err != nil {
									return err
								}
								err = bucket.Put([]byte(path), hash)
								return err
							})
							if err != nil {
								c.logger.Error("Error while updating historical virus database for '%s': %s", path, err.Error())
							}
							go c.Clean(path)
							break
						}
					}()
				}
				return nil
			})

			if err == END { // return early
				c.done <- true
				return
			} else {
				if err != nil {
					c.logger.Error("Error while walking directory '%s': %s", c.directory, err.Error())
				}

				wg.Wait()
				c.logger.Info("Finished scan of '%s' in %f seconds", c.directory, time.Now().Sub(start).Seconds())
				c.logger.Print("===========================")
				c.logger.Print("Results:")
				c.logger.Print("  Skipped files: %d", skipped)
				c.logger.Print("  Scanned files:")
				c.logger.Print("    Ok: %d", okFiles)
				c.logger.Print("    Errors: %d", errored)
				c.logger.Print("    Viruses: %d", found)

				select {
				case <-c.quitting: // interrupt the wait
					break
				case <-time.After(c.wait):
					break
				}
			}
		}
	}()
}

func (c *ClamScanner) Stop() {
	atomic.StoreInt32(&c.quit, int32(1)) // in case in walk loop
	c.quitting <- true                   // in case we're in sleep
	<-c.done
}

func (c *ClamScanner) Clean(path string) { // this needs to be thread safe
	err := os.Remove(path)
	if err != nil {
		c.logger.Error("UNABLE TO REMOVE VIRUS FOUND AT '%s'", path)
	}
}
