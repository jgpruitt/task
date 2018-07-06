package task_test

import (
	"fmt"
	"github.com/jgpruitt/task"
	"time"
	"sync"
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

func ExampleRun_At() {
	// don't exit the example while we are waiting on
	// the time to expire
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
	// don't exit the example while we are waiting on
	// the time to expire
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