// Command fakecmd is the recording binary used by cmdprofile tests. It writes
// its argv and working directory as one JSON line to the file named by
// FOXXYCODE_FAKE_CMD_LOG, optionally prints FOXXYCODE_FAKE_CMD_STDOUT, sleeps
// FOXXYCODE_FAKE_CMD_SLEEP_MS, and exits with FOXXYCODE_FAKE_CMD_EXIT.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type call struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

func main() {
	if path := os.Getenv("FOXXYCODE_FAKE_CMD_LOG"); path != "" {
		dir, _ := os.Getwd()
		raw, err := json.Marshal(call{Args: os.Args[1:], Dir: dir})
		if err == nil {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				_, _ = file.Write(append(raw, '\n'))
				_ = file.Close()
			}
		}
	}
	if ms, err := strconv.Atoi(os.Getenv("FOXXYCODE_FAKE_CMD_SLEEP_MS")); err == nil && ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	if os.Getenv("FOXXYCODE_FAKE_CMD_CREATE_LAST_FILE") == "1" && len(os.Args) > 1 {
		if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("encoded fixture"), 0o600); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	}
	if out := os.Getenv("FOXXYCODE_FAKE_CMD_STDOUT"); out != "" {
		fmt.Print(out)
	}
	if msg := os.Getenv("FOXXYCODE_FAKE_CMD_STDERR"); msg != "" {
		fmt.Fprint(os.Stderr, msg)
	}
	code, err := strconv.Atoi(os.Getenv("FOXXYCODE_FAKE_CMD_EXIT"))
	if err != nil {
		code = 0
	}
	os.Exit(code)
}
