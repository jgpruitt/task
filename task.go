// MIT License
//
// Copyright (c) 2018 John Pruitt
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

// Package task provides various ways to run tasks in goroutines
package task

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A Frame represents a stack frame in a trace of callers to a function
// which has panicked.
type Frame struct {
	File     string
	Line     int
	Function string
}

// String returns a simple string representation of the Frame's information
func (f *Frame) String() string {
	return fmt.Sprintf("File: %s Line: %d Function: %s", f.File, f.Line, f.Function)
}

// callers is a handy wrapper around runtime.Callers and
// runtime.CallersFrames to construct a stack trace
func callers(skip, depth int) (trace []*Frame) {
	if skip < 0 {
		skip = 0
	}
	if depth <= 0 {
		depth = 10
	}

	trace = make([]*Frame, 0)
	var pc = make([]uintptr, depth)
	var n = runtime.Callers(skip, pc)
	var fs = runtime.CallersFrames(pc[:n])
	var f, ok = fs.Next()
	for ok {
		var frame = &Frame{
			Line:     f.Line,
			Function: f.Function,
		}
		var file = filepath.ToSlash(f.File)
		if n := strings.LastIndex(file, "/src/"); n > 0 {
			file = file[n+5:]
		} else {
			file = filepath.Base(file)
		}
		frame.File = file

		trace = append(trace, frame)
		f, ok = fs.Next()
	}
	return
}

// A PanicHandler is called whenever a Task panics.
// The err argument contains the value returned by recover().
// The trace contains stack trace info.
type PanicHandler func(err interface{}, trace []*Frame)

// A Task is a unit of work to be executed
type Task func()

// A Kill is a closure returned from some methods of a *Run
// which if executed will abort future executions of a Task.
type Kill func()

// A Run provides various ways to execute Tasks
type Run struct {
	panicHandler PanicHandler
}

// New returns a newly constructed Run which will use the
// given PanicHandler to deal with any panics
func New(panicHandler PanicHandler) *Run {
	if panicHandler == nil {
		panicHandler = func(err interface{}, trace []*Frame) {
			return
		}
	}
	return &Run{
		panicHandler: panicHandler,
	}
}

// Synchronously blocks the caller while executing task.
// If task panics, it will use the PanicHandler registered
// with r to deal with it.
func (r *Run) Synchronously(task Task) {
	defer func() {
		if err := recover(); err != nil {
			var trace = callers(5, 25)
			r.panicHandler(err, trace)
		}
	}()
	task()
}

// Asynchronously executes task in a goroutine, and thus does
// not block the caller. If task panics, it will use the
// PanicHandler registered with r to deal with it.
func (r *Run) Asynchronously(task Task) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				var trace = callers(5, 25)
				r.panicHandler(err, trace)
			}
		}()
		task()
	}()
}

// At executes task at the given time "at" in the future.
// It returns a Kill which can be used to cancel the execution
// of Task prior to at. An error is returned if at is not a
// time in the future.
func (r *Run) At(at time.Time, task Task) (Kill, error) {
	var dur = time.Until(at)
	if dur <= 0 {
		return nil, errors.New("at must be a time in the future")
	}
	var timer = time.NewTimer(dur)
	var once sync.Once
	var die = make(chan struct{}, 1)

	// construct our killer
	var kill = func() {
		once.Do(func() {
			timer.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				var trace = callers(5, 25)
				r.panicHandler(err, trace)
			}
			// be sure to clean up our timer and channel
			kill()
		}()
		// wait for either the timer to go off or the signal to die
		select {
		case <-timer.C:
			task()
		case <-die:
			// noop; exit the goroutine
		}
	}()

	return kill, nil
}

// After executes task after the given duration "after" has elapsed.
// It returns a Kill which can be used to cancel the execution
// of Task prior to the duration elapsing. An error is returned if
// after is not a positive duration.
func (r *Run) After(after time.Duration, task Task) (Kill, error) {
	if after <= 0 {
		return nil, errors.New("after must be positive duration")
	}
	var timer = time.NewTimer(after)
	var once sync.Once
	var die = make(chan struct{}, 1)

	var kill = func() {
		once.Do(func() {
			timer.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				var trace = callers(5, 25)
				r.panicHandler(err, trace)
			}
			// be sure to clean up our timer and channel
			kill()
		}()
		// wait for the timer to go off or the die signal
		select {
		case <-timer.C:
			task()
		case <-die:
			// noop; exit the goroutine
		}
	}()

	return kill, nil
}

// Every executes task periodically each time every duration elapses.
// It returns a Kill which can be used to cancel any future executions
// of task. An error is returned if every is not a positive duration.
func (r *Run) Every(every time.Duration, task Task) (Kill, error) {
	if every <= 0 {
		return nil, errors.New("every must be a positive duration")
	}
	var tick = time.NewTicker(every)
	var once sync.Once
	var die = make(chan struct{}, 1)

	var kill = func() {
		once.Do(func() {
			tick.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	go func() {
		defer kill() // be sure to clean up our ticker and channel
		for {        // loop forever, or until the die signal is received
			select {
			case <-tick.C:
				// need to run task in an anonymous func so we can
				// handle any panics
				func() {
					defer func() {
						if err := recover(); err != nil {
							var trace = callers(5, 25)
							r.panicHandler(err, trace)
						}
					}()
					task()
				}()
			case <-die:
				// exit the goroutine
				return
			}
		}
	}()

	return kill, nil
}

// Until executes task periodically each time every duration elapses.
// It will cease to periodically execute task at until time in the future.
// It returns a Kill which can be used to cancel any future executions
// of task. An error is returned if every is not a positive duration or
// until is not a time in the future.
func (r *Run) Until(every time.Duration, until time.Time, task Task) (Kill, error) {
	if every <= 0 {
		return nil, errors.New("every must be a positive duration")
	}
	if until.Before(time.Now()) {
		return nil, errors.New("until must be in the future")
	}
	var tick = time.NewTicker(every)
	var once sync.Once
	var die = make(chan struct{}, 1)

	var kill = func() {
		once.Do(func() {
			tick.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	// kill it in the future
	r.After(until.Sub(time.Now()), func() {
		kill()
	})

	go func() {
		defer kill() // be sure to clean up the ticker and channel
		for {        // loop forever until we get the die signal
			select {
			case <-tick.C:
				// run task in an anonymous func so we can
				// handle any panics
				func() {
					defer func() {
						if err := recover(); err != nil {
							var trace = callers(5, 25)
							r.panicHandler(err, trace)
						}
					}()
					task()
				}()
			case <-die:
				// exit the goroutine
				return
			}
		}
	}()

	return kill, nil
}

// Times executes task periodically each time every duration elapses.
// It will cease to periodically execute task after times executions.
// It returns a Kill which can be used to cancel any future executions
// of task. An error is returned if every is not a positive duration or
// times is not a positive integer.
func (r *Run) Times(every time.Duration, times int, task Task) (Kill, error) {
	if every <= 0 {
		return nil, errors.New("every must be a positive duration")
	}
	if times < 1 {
		return nil, errors.New("times must be greater than or equal to 1")
	}
	var tick = time.NewTicker(every)
	var once sync.Once
	var die = make(chan struct{}, 1)

	var kill = func() {
		once.Do(func() {
			tick.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	go func() {
		defer kill() // be sure to clean up the ticker and channel
		// limit our iterations to times times
		for i := 0; i < times; i++ {
			select {
			case <-tick.C:
				// run in an anonymous func so we can handle panics
				func() {
					defer func() {
						if err := recover(); err != nil {
							var trace = callers(5, 25)
							r.panicHandler(err, trace)
						}
					}()
					task()
				}()
			case <-die:
				// exit the goroutine
				return
			}
		}
	}()

	return kill, nil
}

// These runs multiple tasks concurrently and blocks until they have all completed
func (r *Run) These(tasks ...Task) {
	var wg sync.WaitGroup
	wg.Add(len(tasks))
	for _, task := range tasks {
		go func(task Task) {
			defer func() {
				if err := recover(); err != nil {
					var trace = callers(5, 25)
					r.panicHandler(err, trace)
				}
				wg.Done()
			}()
			task()
		}(task)
	}
	wg.Wait()
}

// A Delayer takes the beginning and ending times of the last execution
// of a Task and returns the duration of time to delay before the next
// execution of the task.
type Delayer func(bgn, end time.Time) (delay time.Duration)

// Delayed runs non-overlapping instances of "task" separated by a delay.
// The initial execution of task happens after a delay of "init".
// After each execution, "delayer" is called with the beginning and ending
// times of the last execution. "delayer" will return the delay to wait
// before the next execution should begin. If "delayer" returns a negative
// delay, then no more executions are scheduled.
// A Kill is returned to cancel any future executions.
// An error is returned if "init" is not positive or if delayer is nil.
func (r *Run) Delayed(init time.Duration, delayer Delayer, task Task) (Kill, error) {
	if init < 0 {
		return nil, errors.New("init must be a non-negative duration")
	}
	if delayer == nil {
		return nil, errors.New("delayer must not be nil")
	}

	var timer = time.NewTimer(init)
	var once sync.Once
	var die = make(chan struct{}, 1)

	var kill = func() {
		once.Do(func() {
			timer.Stop()
			die <- struct{}{}
			close(die)
		})
	}

	go func() {
		// be sure to clean up our timer and channel
		defer kill()
		// loop forever, or until killed, or until delayer returns non-positive delay
		for {
			select {
			case <-timer.C:
				// run in an anonymous func so we can catch panics
				var delay = func() time.Duration {
					defer func() {
						if err := recover(); err != nil {
							var trace = callers(5, 25)
							r.panicHandler(err, trace)
						}
					}()
					var bgn = time.Now()
					task()
					return delayer(bgn, time.Now())
				}()
				if delay < 0 {
					return
				}
				timer.Reset(delay)
			case <-die:
				return
			}
		}
	}()

	return kill, nil
}
