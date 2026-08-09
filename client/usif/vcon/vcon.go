package vcon

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	OutputBufferSize = 128 * 1024 // how much of the console output we keep in memory
	InputQueueSize   = 16         // how many not-yet-executed commands we accept from WebUI
	MaxLineLength    = 1024
)

var (
	out_mutex   sync.Mutex
	out_buf     []byte
	out_seq     uint64        // total number of bytes ever written to the console
	out_signal  chan struct{} // closed (and replaced) whenever new data arrives
	out_writers []io.Writer   // additional destinations (e.g. the log file)

	orig_stdout *os.File
	orig_stderr *os.File
	pipe_wr     []*os.File
	copy_done   sync.WaitGroup

	stdin_queue = make(chan string, 1)
	web_queue   = make(chan string, InputQueueSize)

	enabled bool
)

func init() {
	out_signal = make(chan struct{})
}

// Init redirects stdout & stderr through the virtual console.
// Set read_stdin to also feed ReadLine() from the real console.
func Init(read_stdin bool) {
	if !enabled {
		orig_stdout = os.Stdout
		orig_stderr = os.Stderr
		hook(&os.Stdout, orig_stdout)
		hook(&os.Stderr, orig_stderr)
		// the std logger has grabbed os.Stderr at its init time - rebind it
		log.SetOutput(os.Stderr)
		enabled = true
	}
	if read_stdin {
		go stdin_thread()
	}
}

// Close restores the original stdout & stderr and flushes whatever is still in the pipes.
func Close() {
	if !enabled {
		return
	}
	enabled = false
	os.Stdout = orig_stdout
	os.Stderr = orig_stderr
	for _, w := range pipe_wr {
		w.Close()
	}
	copy_done.Wait()
	pipe_wr = nil
}

func Enabled() bool {
	return enabled
}

func hook(std **os.File, orig *os.File) {
	r, w, er := os.Pipe()
	if er != nil {
		fmt.Fprintln(orig, "vcon:", er.Error())
		return
	}
	*std = w
	pipe_wr = append(pipe_wr, w)
	copy_done.Add(1)
	go func() {
		defer copy_done.Done()
		buf := make([]byte, 4096)
		for {
			n, er := r.Read(buf)
			if n > 0 {
				orig.Write(buf[:n])
				store(buf[:n])
			}
			if er != nil {
				return
			}
		}
	}()
}

// AddWriter attaches an extra destination for everything going to the console.
func AddWriter(w io.Writer) {
	out_mutex.Lock()
	out_writers = append(out_writers, w)
	out_mutex.Unlock()
}

func DelWriter(w io.Writer) {
	out_mutex.Lock()
	for i := range out_writers {
		if out_writers[i] == w {
			out_writers = append(out_writers[:i], out_writers[i+1:]...)
			break
		}
	}
	out_mutex.Unlock()
}

func store(p []byte) {
	if len(p) == 0 {
		return
	}
	out_mutex.Lock()
	out_buf = append(out_buf, p...)
	if len(out_buf) > OutputBufferSize {
		out_buf = append(out_buf[:0], out_buf[len(out_buf)-OutputBufferSize:]...)
	}
	out_seq += uint64(len(p))
	for _, w := range out_writers {
		w.Write(p)
	}
	close(out_signal) // wake up all the pending Fetch()es
	out_signal = make(chan struct{})
	out_mutex.Unlock()
}

// Seq returns the current output sequence number.
func Seq() (seq uint64) {
	out_mutex.Lock()
	seq = out_seq
	out_mutex.Unlock()
	return
}

// Fetch returns the console output appended since the given sequence number.
// It blocks for up to maxwait if there is nothing new yet.
// Pass from=0 to get the entire buffer we still have.
func Fetch(from uint64, maxwait time.Duration) (data []byte, seq uint64) {
	deadline := time.Now().Add(maxwait)
	for {
		out_mutex.Lock()
		seq = out_seq
		if from > seq {
			from = 0 // the client is ahead of us (node restarted) - resync it
		}
		if avail := seq - from; avail > 0 {
			if avail > uint64(len(out_buf)) {
				avail = uint64(len(out_buf)) // it has been over the buffer - give what we have
			}
			data = make([]byte, avail)
			copy(data, out_buf[uint64(len(out_buf))-avail:])
			out_mutex.Unlock()
			return
		}
		signal := out_signal
		out_mutex.Unlock()

		tout := time.Until(deadline)
		if tout <= 0 {
			return
		}
		tmr := time.NewTimer(tout)
		select {
		case <-signal:
		case <-tmr.C:
		}
		tmr.Stop()
	}
}

// ReadLine returns the next command line, from either of the two consoles.
func ReadLine() string {
	select {
	case s := <-stdin_queue:
		return s
	case s := <-web_queue:
		fmt.Println(s) // echo it, so it also shows up on the real console
		return s
	}
}

// PostLine queues a command line coming from the WebUI.
func PostLine(s string) bool {
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > MaxLineLength {
		s = s[:MaxLineLength]
	}
	select {
	case web_queue <- s:
		return true
	default:
		return false // too many commands pending
	}
}

func stdin_thread() {
	rd := bufio.NewReader(os.Stdin)
	for {
		li, er := rd.ReadString('\n')
		li = strings.TrimRight(li, "\r\n")
		if er != nil {
			if li == "" {
				return // stdin closed (e.g. running as a daemon) - only WebUI from now on
			}
			store([]byte(li + "\n"))
			stdin_queue <- li
			return
		}
		store([]byte(li + "\n")) // the real console has echoed it by itself
		stdin_queue <- li
	}
}
