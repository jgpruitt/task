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

type Frame struct {
	File     string
	Line     int
	Function string
}

func (f *Frame) String() string {
	return fmt.Sprintf("File: %s Line: %d Function: %s", f.File, f.Line, f.Function)
}

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

type PanicHandler func(err interface{}, trace []*Frame)

type Task func()

type Kill func()

type Run struct {
	panicHandler PanicHandler
}

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

func (r *Run) Synchronously(task Task) {
	defer func() {
		if err := recover(); err != nil {
			var trace = callers(5, 25)
			r.panicHandler(err, trace)
		}
	}()
	task()
}

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

func (r *Run) At(at time.Time, task Task) (Kill, error) {
	var dur = time.Until(at)
	if dur <= 0 {
		return nil, errors.New("at must be a time in the future")
	}
	var timer = time.NewTimer(dur)
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
			kill()
		}()
		select {
		case <-timer.C:
			task()
		case <-die:
		}
	}()

	return kill, nil
}

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
			kill()
		}()
		select {
		case <-timer.C:
			task()
		case <-die:
		}
	}()

	return kill, nil
}

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
		defer kill()
		for {
			select {
			case <-tick.C:
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
				return
			}
		}
	}()

	return kill, nil
}

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
			die <-struct{}{}
			close(die)
		})
	}

	r.After(until.Sub(time.Now()), func() {
		kill()
	})

	go func() {
		defer kill()
		for {
			select {
			case <-tick.C:
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
				return
			}
		}
	}()

	return kill, nil
}

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
			die <-struct{}{}
			close(die)
		})
	}

	go func() {
		defer kill()
		for i := 0; i < times; i++ {
			select {
			case <-tick.C:
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
