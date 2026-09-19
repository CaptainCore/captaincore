// re2check reads one regular expression per line on stdin and prints, for
// each, "ok" or the compile error, so a translator written in another
// language can drop patterns Go's RE2 engine will not accept.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if _, err := regexp.Compile(sc.Text()); err != nil {
			fmt.Println("ERR " + err.Error())
		} else {
			fmt.Println("ok")
		}
	}
}
