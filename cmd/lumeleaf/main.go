package main

import (
	"fmt"
	lumeleaf "github.com/itishrishikesh/lumeleaf/internal/app"
	"os"
)

func main() {
	if err := lumeleaf.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lumeleaf:", err)
		os.Exit(1)
	}
}
