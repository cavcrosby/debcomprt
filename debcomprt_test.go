package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

var pkgs []string = []string{"autoconf", "git", "wget"}

// createTestFile creates a test file that is solely meant for testing. This file
// should be created on the intentions of allowing any test to access it.
func createTestFile(fileName, contents string) error {
	err := ioutil.WriteFile(filepath.Join(".", fileName), []byte(contents), fs.FileMode(0777))

	if err != nil {
		return err
	}

	return nil
}

func TestCopy(t *testing.T) {
	// inspired by:
	// https://stackoverflow.com/questions/29505089/how-can-i-compare-two-files-in-golang#answer-29528747
	var filePath1, filePath2, fileContents string
	fileContents = "hello\nthere!\n"
	filePath1 = filepath.Join(".", "foo")
	filePath2 = filepath.Join(".", "bar")

	createTestFile(filePath1, fileContents)
	copy(filePath1, filePath2)

	file1, err := ioutil.ReadFile(filePath1)

	if err != nil {
		t.Error(err)
	}

	file2, err := ioutil.ReadFile(filePath2)

	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(file1, file2) {
		t.Errorf("%s is not the same as %s", filePath1, filePath2)
	}

	os.Remove(filePath1)
	os.Remove(filePath2)
}

func TestGetComprtIncludes(t *testing.T) {
	var includePkgs []string
	pkgsByteString := []byte(strings.Join(pkgs, "\n"))
	args := &cmdArgs{ // sets defaults
		comprtIncludesPath: filepath.Join(".", comprtIncludeFile),
		comprtConfigPath:   filepath.Join(".", comprtConfigFile),
	}

	err := createTestFile(args.comprtIncludesPath, strings.Join(pkgs, "\n"))
	if err != nil {
		t.Error(err)
	}

	if getComprtIncludes(&includePkgs, args); !bytes.Equal([]byte(strings.Join(includePkgs, "\n")), pkgsByteString) {
		t.Errorf("found the following packages \n%s", strings.Join(includePkgs, "\n"))
	}

	os.Remove(args.comprtIncludesPath)
}

func TestIntegration(t *testing.T) {
	var codename, target, chrootTestFilePath string
	codename = "buster"
	target = "testdir"
	chrootTestFilePath = filepath.Join("/", "foo")
	var comprtConfigFileContents string = fmt.Sprintf(`#!/bin/sh

touch %s
	`, chrootTestFilePath)

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	os.Mkdir(target, fs.FileMode(0777))
	createTestFile(comprtIncludeFile, strings.Join(pkgs, "\n"))
	createTestFile(comprtConfigFile, comprtConfigFileContents)

	debcomprtCmd := exec.Command("debcomprt", codename, target)
	if testing.Verbose() {
		debcomprtCmd.Stdout = os.Stdout
		debcomprtCmd.Stderr = os.Stderr
	}

	if err := debcomprtCmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := debcomprtCmd.Wait(); err != nil {
		log.Fatal(err)
	}

	if err := syscall.Chroot(target); err != nil {
		t.Error(err)
	}
	if err := syscall.Chdir("/"); err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(chrootTestFilePath); errors.Is(err, fs.ErrNotExist) {
		t.Error(err)
	}

	// TODO(cavcrosby): add checks to make sure the packages included were installed
	// (e.g. running dpkg-query on each pkg name) in the chroot env.

	os.Remove(comprtIncludeFile)
	os.Remove(comprtConfigFile)
}
