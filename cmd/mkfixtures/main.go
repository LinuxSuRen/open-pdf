// Command mkfixtures writes sample PDF and image files into a target
// directory. It is used by the e2e test suite and by humans who want files
// to play with.
//
// Usage: go run ./cmd/mkfixtures [target-dir]
package main

import (
	"fmt"
	"os"

	"github.com/linuxsuren/open-pdf/internal/testutil"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"sample.pdf (3 pages)", func() error { return testutil.WritePDFFile(dir+"/sample.pdf", 3) }},
		{"long.pdf (12 pages)", func() error { return testutil.WritePDFFile(dir+"/long.pdf", 12) }},
		{"photo.png", func() error { return testutil.WritePNGFile(dir+"/photo.png", 320, 200) }},
		{"photo.jpg", func() error { return testutil.WriteJPEGFile(dir+"/photo.jpg", 320, 200) }},
		{"logo.png", func() error { return testutil.WritePNGFile(dir+"/logo.png", 96, 96) }},
	}

	for _, st := range steps {
		if err := st.fn(); err != nil {
			fmt.Fprintf(os.Stderr, "mkfixtures: %s: %v\n", st.name, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", st.name)
	}
}
