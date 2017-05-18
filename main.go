package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/boltdb/bolt"
)

var Version = "0.0.1"

func main() {
	var port int
	var debug bool
	var address string
	var dbFile string
	var directory string
	var concurrency int
	var timeout int
	var version bool
	var sleep int
	var size int64
	flag.StringVar(&address, "address", "", "The address of the clamAV server (required).")
	flag.StringVar(&directory, "directory", "", "The directory of files to scan (required).")
	flag.Int64Var(&size, "size", 26214400, "The maximum byte size of files to scan (should be equal to or less than clamd configuration).")
	flag.IntVar(&port, "port", 3310, "The port that clamAV is running on.")
	flag.StringVar(&dbFile, "database", "glamscan.db", "The database of files that have been scanned.")
	flag.IntVar(&concurrency, "concurrency", 10, "How many files should we scan at a time.")
	flag.IntVar(&timeout, "timeout", 2, "Socket level timeout for tcp connection in seconds.")
	flag.BoolVar(&version, "version", false, "Print version and exit.")
	flag.IntVar(&sleep, "sleep", 60, "How many seconds to wait between scans.")
	flag.BoolVar(&debug, "debug", false, "Turn on debugging.")

	flag.Parse()

	if version {
		fmt.Println(Version)
		os.Exit(0)
	}

	if address == "" || directory == "" {
		fmt.Fprintf(os.Stderr, "Must specify a value for 'address' and 'directory'.\n")
		flag.Usage()
		os.Exit(1)
	}

	// handle ctrl+c
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	database, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	err = database.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("files"))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte("viruses"))
		if err != nil {
			return err
		}
		return err
	})
	if err != nil {
		log.Fatal(err)
	}

	logger := initializeSystemLogger(debug)

	queue, err := NewScanQueue(logger, concurrency, ClamConfiguration{
		Protocol: "tcp",
		Address:  address,
		Port:     port,
		Timeout:  time.Duration(timeout) * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	queue.Start()
	defer queue.Stop()

	scanner := NewClamScanner(directory, size, time.Duration(sleep)*time.Second, logger, database, queue)
	scanner.Start()
	defer scanner.Stop()

	<-quit
	logger.Debug("Cleaning up")
}
