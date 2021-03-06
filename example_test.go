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

package task_test

import (
	"fmt"
	"github.com/jgpruitt/task"
	"sync"
	"sync/atomic"
	"time"
)

func handlePanic(err interface{}, trace []*task.Frame) {
	// this PanicHandler just prints the error and stack trace
	fmt.Println("Error:", err)
	for frame, i := range trace {
		fmt.Println(i, frame)
	}
}

func ExampleRun_Synchronously() {
	run := task.New(handlePanic)
	run.Synchronously(func() {
		fmt.Println("I go first.")
	})
	fmt.Println("I was blocked until it finished.")

	// Output:
	// I go first.
	// I was blocked until it finished.
}

func ExampleRun_Asynchronously() {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	run := task.New(handlePanic)

	run.Asynchronously(func() {
		// do some hard work...
		time.Sleep(5 * time.Second)
		fmt.Println("I finished.")
		wg.Done()
	})

	fmt.Println("I'm not waiting.")

	// ok, NOW I'm waiting.
	wg.Wait()

	// Output:
	// I'm not waiting.
	// I finished.
}

func ExampleRun_At() {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	run := task.New(handlePanic)

	at := time.Now().Add(5 * time.Second)
	run.At(at, func() {
		fmt.Println("Hello World!")
		wg.Done()
	})

	fmt.Println("This happens while waiting...")
	wg.Wait()

	// Output:
	// This happens while waiting...
	// Hello World!
}

func ExampleRun_After() {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	run := task.New(handlePanic)

	after := 5 * time.Second
	run.After(after, func() {
		fmt.Println("Hello World!")
		wg.Done()
	})

	fmt.Println("This happens while waiting...")
	wg.Wait()

	// Output:
	// This happens while waiting...
	// Hello World!
}

func ExampleRun_Every() {
	wg := &sync.WaitGroup{}
	wg.Add(3)

	run := task.New(handlePanic)

	kill, _ := run.Every(1*time.Second, func() {
		fmt.Println("I ran.")
		wg.Done()
	})

	fmt.Println("Waiting...")
	wg.Wait()
	kill()

	// Output:
	// Waiting...
	// I ran.
	// I ran.
	// I ran.
}

func ExampleRun_Until() {
	run := task.New(handlePanic)

	every := 1 * time.Second
	until := time.Now().Add(3 * time.Second)
	run.Until(every, until, func() {
		fmt.Println("I ran.")
	})

	fmt.Println("Sleeping...")
	time.Sleep(5 * time.Second)

	// Output:
	// Sleeping...
	// I ran.
	// I ran.
	// I ran.
}

func ExampleRun_Times() {
	wg := &sync.WaitGroup{}
	wg.Add(3)

	run := task.New(handlePanic)

	run.Times(1*time.Second, 3, func() {
		fmt.Println("I ran.")
		wg.Done()
	})

	fmt.Println("Waiting...")
	wg.Wait()

	// Output:
	// Waiting...
	// I ran.
	// I ran.
	// I ran.
}

func ExampleRun_These() {
	wg := &sync.WaitGroup{}
	wg.Add(3)

	var accum int32 = 0

	run := task.New(handlePanic)

	task1 := func() {
		atomic.AddInt32(&accum, 1)
		wg.Done()
	}

	task2 := func() {
		atomic.AddInt32(&accum, 2)
		wg.Done()
	}

	task3 := func() {
		atomic.AddInt32(&accum, 3)
		wg.Done()
	}

	run.These(task1, task2, task3)

	wg.Wait()
	fmt.Printf("accum: %d\n", accum)

	// Output:
	// accum: 6
}
