// +build darwin dragonfly freebsd linux netbsd openbsd

package main

import (
	"flag"
	"log"
	"log/syslog"
	"os"
)

var useSyslog bool

func init() {
	flag.BoolVar(&useSyslog, "syslog", false, "Log directly to syslog.")
}

func initializeSystemLogger(debug bool) *Logger {
	if useSyslog {
		debugLog, err := syslog.New(syslog.LOG_DEBUG, "glamscan")
		if err != nil {
			log.Fatal(err)
		}
		infoLog, err := syslog.New(syslog.LOG_INFO, "glamscan")
		if err != nil {
			log.Fatal(err)
		}
		warnLog, err := syslog.New(syslog.LOG_WARNING, "glamscan")
		if err != nil {
			log.Fatal(err)
		}
		errorLog, err := syslog.New(syslog.LOG_ALERT, "glamscan")
		if err != nil {
			log.Fatal(err)
		}
		logger := NewLogger(debug, debugLog, infoLog, warnLog, errorLog)
		return logger
	}

	return NewLogger(debug, os.Stdout, os.Stdout, os.Stdout, os.Stderr)
}
