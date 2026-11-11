package supervisor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"
)

// Chromium can print the same warning many times a second for days (GPU,
// VA-API, WebRTC). Every line ends up in the browser log on the SD card and,
// through stdout, in the journal. logFilter passes each line once and folds
// identical repeats into one summary line, and it keeps the log file below a
// size limit while the browser runs (rotation only happens at start).

const (
	browserLogLimit = 5 << 20
	// repeatSummaryEvery writes the repeat count at least this often while
	// the same line keeps coming.
	repeatSummaryEvery = 10 * time.Minute
	maxLineBytes       = 8 << 10
)

// chromiumPrefix matches "[1234:5678:1109/101500.123456:WARNING:file.cc(12)] ".
var chromiumPrefix = regexp.MustCompile(`^\[\d+:\d+:\d+/\d+\.\d+:`)

type logFilter struct {
	mu        sync.Mutex
	out       io.Writer
	file      *os.File
	limit     int64
	written   int64
	partial   []byte
	last      []byte
	repeats   int
	summaryAt time.Time
	now       func() time.Time
}

func newLogFilter(out io.Writer, file *os.File) *logFilter {
	f := &logFilter{out: out, file: file, limit: browserLogLimit, now: time.Now}
	if file != nil {
		if info, err := file.Stat(); err == nil {
			f.written = info.Size()
		}
	}
	return f
}

func (f *logFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.partial = append(f.partial, p...)
	for {
		index := bytes.IndexByte(f.partial, '\n')
		if index < 0 {
			if len(f.partial) > maxLineBytes {
				f.line(f.partial)
				f.partial = f.partial[:0]
			}
			break
		}
		f.line(f.partial[:index])
		f.partial = f.partial[index+1:]
	}
	if len(f.partial) == 0 {
		f.partial = nil
	}
	return len(p), nil
}

func (f *logFilter) line(line []byte) {
	key := chromiumPrefix.ReplaceAll(line, nil)
	now := f.now()
	if f.last != nil && bytes.Equal(key, f.last) {
		f.repeats++
		if now.Sub(f.summaryAt) >= repeatSummaryEvery {
			f.flushRepeats(now)
		}
		return
	}
	f.flushRepeats(now)
	f.last = append(f.last[:0], key...)
	f.summaryAt = now
	f.emit(append(append([]byte(nil), line...), '\n'))
}

func (f *logFilter) flushRepeats(now time.Time) {
	if f.repeats == 0 {
		return
	}
	f.emit([]byte(fmt.Sprintf("    (previous line repeated %d times)\n", f.repeats)))
	f.repeats = 0
	f.summaryAt = now
}

func (f *logFilter) emit(data []byte) {
	if f.out != nil {
		_, _ = f.out.Write(data)
	}
	if f.file == nil {
		return
	}
	if f.limit > 0 && f.written+int64(len(data)) > f.limit {
		if f.file.Truncate(0) == nil {
			f.written = 0
			marker := fmt.Sprintf("--- log reached %d MB at %s, older lines removed ---\n", f.limit>>20, f.now().Format(time.RFC3339))
			n, _ := f.file.Write([]byte(marker))
			f.written += int64(n)
		}
	}
	n, _ := f.file.Write(data)
	f.written += int64(n)
}

// Flush writes a pending partial line and the repeat count, for example when
// the browser exits.
func (f *logFilter) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.partial) > 0 {
		f.line(f.partial)
		f.partial = nil
	}
	f.flushRepeats(f.now())
}
