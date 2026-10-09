package main

import (
	"fmt"
	"time"
)

func main() {
	jobs := make(chan int)
	results := make(chan string)

	for w := range 3 {
		go worker(w, jobs, results)
	}

	go func() {
		for i := range 6 {
			jobs <- i
		}
		close(jobs)
	}()

	for range 6 {
		fmt.Println(<-results)
	}
}

func worker(id int, jobs <-chan int, results chan<- string) {
	for j := range jobs {
		time.Sleep(200 * time.Millisecond)
		results <- fmt.Sprintf("worker %d did job %d", id, j)
	}
}
