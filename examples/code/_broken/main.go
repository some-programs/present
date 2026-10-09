// This program does not compile, to show that the line numbers in the
// compiler's errors match the ones on the slide. It is in a directory
// starting with _ so that the go command leaves it out of ./...
package main

import "fmt"

func greeting() string {
	return "hello"
}

func main() {
	n := 1
	fmt.Println(greeting(), n+"!")
	fmt.Println(missing)
}

func count() int {
	return "not a number"
}
