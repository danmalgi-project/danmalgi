package log

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

type daliyFileWriter struct {
	mutex      sync.Mutex
	basepath   string
	suffix     string
	currentDay string
	file       *os.File
}

func newDaliyFileWriter(basepath string, suffix string) io.Writer {
	daliyFileWriter := &daliyFileWriter{basepath: basepath, suffix: suffix}
	daliyFileWriter.rotateFile()
	return daliyFileWriter
}

func (d *daliyFileWriter) rotateFile() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	day := time.Now().Format("2006-01-02")
	if d.currentDay == day {
		return
	}

	if d.file != nil {
		d.file.Close()
	}

	filepath := path.Join(d.basepath, fmt.Sprintf("%s%s.log", day, d.suffix))
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	d.currentDay = day
	d.file = file
}

func (d *daliyFileWriter) Write(p []byte) (n int, err error) {
	d.rotateFile()
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.file.Write(p)
}
