// Command docsgen generates the parts of the documentation that follow the code
// or the navigation map and checks the rest. See docs/contributing/documentation.md.
//
//	go run ./cmd/docsgen -write            # regenerate (make docs)
//	go run ./cmd/docsgen                   # check only, exit 1 on drift (make docs-check)
//	go run ./cmd/docsgen -write -skip-cli  # everything but the CLI reference (make docs-fast)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hijera/foxxycode-agent/internal/docsgen"
)

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "write the generated files instead of checking them")
	tags := flag.String("tags", "", "build tags of the foxxycode binary the CLI reference is generated from")
	binary := flag.String("foxxycode", "", "path to a built foxxycode binary (built from source when empty)")
	skipCLI := flag.Bool("skip-cli", false, "leave the CLI reference untouched (no binary is built or run)")
	rawBase := flag.String("raw-base", docsgen.DefaultRawBase, "base URL of the raw Markdown for llms.txt")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}
	res, err := docsgen.Generate(docsgen.Options{Root: abs, Binary: *binary, Tags: *tags, RawBase: *rawBase, SkipCLI: *skipCLI})
	if err != nil {
		fail(err)
	}
	if *write {
		if err := res.Write(abs); err != nil {
			fail(err)
		}
		for rel := range res.Files {
			fmt.Println("wrote", rel)
		}
	} else {
		res.Problems = append(res.Problems, res.Stale(abs)...)
	}
	for _, p := range res.Problems {
		fmt.Fprintln(os.Stderr, p)
	}
	if len(res.Problems) > 0 {
		fmt.Fprintf(os.Stderr, "docsgen: %d problem(s)\n", len(res.Problems))
		os.Exit(1)
	}
	fmt.Println("docsgen: ok")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "docsgen:", err)
	os.Exit(2)
}
