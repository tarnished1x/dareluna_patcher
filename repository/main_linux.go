//go:build linux

package main

import (
    "fmt"
    "os"
)

func main() {
    if len(os.Args) != 3 {
        fmt.Fprintln(os.Stderr, "TEST-ONLY: ./ribbonhearts ORIGINAL_FOLDER OUTPUT_FOLDER")
        os.Exit(2)
    }
    report, err := InstallPatch(os.Args[1], os.Args[2], func(msg string, percent int) {fmt.Printf("%3d%% %s\n", percent, msg)})
    if err != nil {fmt.Fprintln(os.Stderr, "FAILED:", err); os.Exit(1)}
    fmt.Printf("SUCCESS: created %d, already installed %d\n", report.Created, report.Existing)
}
