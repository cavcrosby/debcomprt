package main

import "os"

var ProgDataDir string

func init() {
    ProgDataDir = os.Getenv("PROG_DATA_DIR")
}
