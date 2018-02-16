// Godu - a fast concurrent and parallel 'du' like utility written in Go.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// define and set default command parameter flags
var vFlag = flag.Bool("v", false, "Optional: show verbose progress messages")
var tFlag = flag.Int("t", runtime.NumCPU(), "Optional: set number of threads, defaults to number of logical cores")

// Program starts here
func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage: %s [-v, -t int] topdir1 topdirN\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample: %s -v /home/rmcdermo /fh/fast/mcdermott_r /fh/secure/research/mcdermott_r\n\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Println()
	}
	start := time.Now().Unix()
	flag.Parse()
	runtime.GOMAXPROCS(*tFlag)

	// Get the directory root(s) to start the file walk(s)
	roots := flag.Args()
	if len(roots) == 0 {
		roots = []string{"."}
	}

	// Walk the directory root(s) concurrently
	fileSizes := make(chan int64, 256)

	var n sync.WaitGroup
	for _, root := range roots {
		n.Add(1)
		go walkDir(root, &n, fileSizes)
	}
	go func() {
		n.Wait()
		close(fileSizes)
	}()

	// If the '-v' flag was provided, periodically print the progress stats
	var tick <-chan time.Time
	if *vFlag {
		tick = time.Tick(500 * time.Millisecond)
	}

	// Loop that builds up the running file count and size
	var nfiles, nbytes int64
loop:
	for {
		select {
		case size, ok := <-fileSizes:
			if !ok {
				break loop // fileSizes was closed
			}
			nfiles++
			nbytes += size
		case <-tick:
			printProgress(nfiles, nbytes, start)
		}
	}

	// Final totals
	printDiskUsage(nfiles, nbytes, start)
}

// Prints the final summary
func printDiskUsage(nfiles, nbytes int64, start int64) {
	stop := time.Now().Unix()
	elapsed := stop - start
	if elapsed == 0 {
		elapsed = 1
	}
	fps := nfiles / elapsed
	fmt.Printf("\nDone!\nFiles: %d, Size: %.1fGB, Avg FPS: %d, Elapsed: %d seconds\n", nfiles, float64(nbytes)/1e9, fps, elapsed)
}

// Prints the running progress summary if invoked with -v flag
func printProgress(nfiles, nbytes int64, start int64) {
	now := time.Now().Unix()
	elapsed := now - start
	if elapsed == 0 {
		elapsed = 1
	}
	fps := nfiles / elapsed
	fmt.Printf("Files: %d, Size: %.1fGB, Goroutines: %d, Cur FPS: %d\n", nfiles, float64(nbytes)/1e9, runtime.NumGoroutine(), fps)
}

// Recursively walks the file tree rooted at dir and sends the size of each found file on fileSizes channel.
func walkDir(dir string, n *sync.WaitGroup, fileSizes chan<- int64) {
	defer n.Done()
	for _, entry := range dirents(dir) {
		if entry.IsDir() {
			n.Add(1)
			subdir := filepath.Join(dir, entry.Name())
			go walkDir(subdir, n, fileSizes)
		} else {
			fileSizes <- entry.Size()
		}
	}
}

// sema is a semaphore for limiting concurrency in dirents to prevent tool many open files situation
var sema = make(chan struct{}, 256)

// dirents returns the entries of directory dir.
func dirents(dir string) []os.FileInfo {
	sema <- struct{}{}        // acquire token
	defer func() { <-sema }() // release token

	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "du: %v\n", err)
		return nil
	}
	return entries
}
