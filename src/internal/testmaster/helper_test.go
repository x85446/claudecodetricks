package testmaster

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// helperArgv runs this test binary as a child process in one of the modes
// TestHelperProcess knows, so process-tree and lock tests need no shell and
// run the same on every OS.
func helperArgv(mode string, args ...string) []string {
	return append([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}, args...)
}

func helperEnv() []string { return []string{"TESTMASTER_HELPER=1"} }

// TestHelperProcess is not a test: it is the child process the tests above
// start. Modes:
//
//	tree <marker>     start a grandchild that writes <marker> after 2s, then sleep 60s
//	detach <marker>   start that grandchild and exit 0 at once
//	grandchild <m>    sleep 2s, write <m>
//	exit <n>          exit with status n
//	lock <dir>        take <dir>'s project lock, print "locked", sleep 60s
func TestHelperProcess(t *testing.T) {
	if os.Getenv("TESTMASTER_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	switch args[0] {
	case "tree", "detach":
		gc := exec.Command(os.Args[0], helperArgv("grandchild", args[1])[1:]...)
		if err := gc.Start(); err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		if args[0] == "detach" {
			os.Exit(0)
		}
		time.Sleep(60 * time.Second)
	case "grandchild":
		time.Sleep(2 * time.Second)
		_ = os.WriteFile(args[1], []byte("survived"), 0o644)
	case "exit":
		n, _ := strconv.Atoi(args[1])
		os.Exit(n)
	case "lock":
		p, err := Open(args[1])
		if err != nil {
			fmt.Println(err)
			os.Exit(2)
		}
		if _, err := p.Lock(); err != nil {
			fmt.Println(err)
			os.Exit(3)
		}
		fmt.Println("locked")
		time.Sleep(60 * time.Second)
	}
	os.Exit(0)
}
