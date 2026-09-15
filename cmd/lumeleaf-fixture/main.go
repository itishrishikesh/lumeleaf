package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	lines := flag.Int("lines", 10000, "line count")
	output := flag.String("output", "", "output file")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "--output is required")
		os.Exit(2)
	}
	var b strings.Builder
	b.WriteString("package fixture;\npublic final class Large {\n")
	for i := 0; i < *lines; i++ {
		fmt.Fprintf(&b, "  private int value%d = %d;\n", i, i)
	}
	b.WriteString("}\n")
	if err := os.WriteFile(*output, []byte(b.String()), 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
