// Template baseline for `rustrush candidate`. Contract: accept --iterations N
// and --input FILE, print exactly one deterministic result line to stdout.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	iterations := flag.Uint64("iterations", 0, "number of passes over the input")
	input := flag.String("input", "", "file with one signed integer per line")
	flag.Parse()
	if *iterations == 0 || *input == "" {
		fmt.Fprintln(os.Stderr, "--iterations and --input are required")
		os.Exit(2)
	}
	file, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	var numbers []int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		value, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		numbers = append(numbers, value)
	}
	fmt.Println(checksum(numbers, *iterations))
}

// checksum is the operation under test: an FNV-style fold over the input.
func checksum(numbers []int64, iterations uint64) uint64 {
	value := uint64(0xcbf29ce484222325)
	for i := uint64(0); i < iterations; i++ {
		for _, number := range numbers {
			value = (value ^ uint64(number)) * 0x100000001b3
		}
	}
	return value
}
