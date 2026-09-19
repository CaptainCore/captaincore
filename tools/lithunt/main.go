// lithunt searches file trees for a set of literal strings in one pass and
// prints every file that contains any of them, with the literals it holds.
// It is the fleet-side half of the malware family hunt: indicators pulled
// from public research are checked against every quicksave tree on the core
// server without running the full scanner or one grep per indicator.
//
//	lithunt <literals.txt> <root>...  > hits.tsv    (path \t literal)
//
// Options (flags before the arguments):
//
//	-ext php,js,html,htaccess   only these extensions (default: the scanner's
//	                            scannable set plus txt, ico, json, css, log)
//	-max 8388608                skip files larger than this many bytes
//	-workers N                  default: CPU count
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/CaptainCore/captaincore/scan"
)

func main() {
	ext := flag.String("ext", "php,phtml,php5,php7,inc,phar,js,mjs,html,htm,svg,txt,ico,json,css,log,htaccess", "comma-separated extensions to read")
	maxSize := flag.Int64("max", 8<<20, "skip files larger than this")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel readers")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: lithunt [-ext ...] [-max N] <literals.txt> <root>...")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var lits []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if l := strings.TrimRight(sc.Text(), "\r"); l != "" {
			lits = append(lits, l)
		}
	}
	f.Close()
	if len(lits) == 0 {
		fmt.Fprintln(os.Stderr, "no literals")
		os.Exit(2)
	}
	want := map[string]bool{}
	for _, e := range strings.Split(*ext, ",") {
		if e = strings.TrimSpace(e); e != "" {
			want["."+strings.TrimPrefix(e, ".")] = true
		}
	}
	idx := scan.NewLiteralCounter(lits)
	paths := make(chan string, 4096)
	var wg sync.WaitGroup
	var mu sync.Mutex
	out := bufio.NewWriter(os.Stdout)
	files, hits := 0, 0
	for i := 0; i < *workers; i++ {
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
				var found []string
				for i, l := range lits {
					if hit[idx.ID(i)] {
						found = append(found, l)
					}
				}
				mu.Lock()
				files++
				if len(found) > 0 {
					hits++
					for _, l := range found {
						fmt.Fprintf(out, "%s\t%s\n", p, l)
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, root := range flag.Args()[1:] {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !want[scan.ExtOf(d.Name())] {
				return nil
			}
			if info, err := d.Info(); err != nil || info.Size() == 0 || info.Size() > *maxSize {
				return nil
			}
			paths <- p
			return nil
		})
	}
	close(paths)
	wg.Wait()
	out.Flush()
	fmt.Fprintf(os.Stderr, "%d files read, %d with hits\n", files, hits)
}
