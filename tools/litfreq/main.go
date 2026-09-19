// litfreq counts, for each literal read from a file (one per line), how many
// files under a directory contain it, using the scanner's one-pass literal
// index. A rule translator uses the counts to pick the rarest required
// literal as a prefilter, so a rule is not tried on every file that merely
// says base64_decode.
//
//	litfreq <literals.txt> <dir> > counts.tsv   (literal \t files)
package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/CaptainCore/captaincore/scan"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: litfreq <literals.txt> <dir>")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var lits []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if sc.Text() != "" {
			lits = append(lits, sc.Text())
		}
	}
	f.Close()
	idx := scan.NewLiteralCounter(lits)
	paths := make(chan string, 1024)
	var wg sync.WaitGroup
	var mu sync.Mutex
	files := 0
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hit := make([]bool, idx.Len())
			for p := range paths {
				data, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				for i := range hit {
					hit[i] = false
				}
				idx.Present(data, hit)
				mu.Lock()
				files++
				idx.Add(hit)
				mu.Unlock()
			}
		}()
	}
	filepath.WalkDir(os.Args[2], func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Type().IsRegular() {
			paths <- p
		}
		return nil
	})
	close(paths)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "%d files\n", files)
	for i, lit := range lits {
		fmt.Printf("%s\t%d\n", lit, idx.Count(i))
	}
}
