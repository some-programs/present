package main

import "fmt"

// START OMIT
func fib(n int) int {
	a, b := 0, 1
	for range n {
		a, b = b, a+b // HLstep
	}
	return a
}

// END OMIT

func main() {
	for i := range 10 {
		fmt.Print(fib(i), " ") // HLmain
	}
	fmt.Println()
}
