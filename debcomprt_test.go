package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const (
	tempDir = "debcomprt"
)

var pkgs []string = []string{"autoconf", "git", "wget"}

// createTestFile creates a test file that is solely meant for testing. This file
// should be created on the intentions of allowing any test to access it.
func createTestFile(filePath, contents string) error {
	err := ioutil.WriteFile(filePath, []byte(contents), fs.FileMode(0777))
	if err != nil {
		return err
	}

	return nil
}

func TestCopy(t *testing.T) {
	// inspired by:
	// https://stackoverflow.com/questions/29505089/how-can-i-compare-two-files-in-golang#answer-29528747
	var filePath1, filePath2, fileContents string
	tempDirPath, err := ioutil.TempDir("", tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	fileContents = "hello\nthere!\n"
	filePath1 = filepath.Join(tempDirPath, "foo")
	filePath2 = filepath.Join(tempDirPath, "bar")

	if err := createTestFile(filePath1, fileContents); err != nil {
		t.Error(err)
	}
	if err := copy(filePath1, filePath2); err != nil {
		t.Error(err)
	}

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
}

func TestGetComprtIncludes(t *testing.T) {
	var includePkgs []string
	tempDirPath, err := ioutil.TempDir("", tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	pkgsByteString := []byte(strings.Join(pkgs, "\n"))
	args := &cmdArgs{
		comprtIncludesPath: filepath.Join(tempDirPath, comprtIncludeFile),
		comprtConfigPath:   filepath.Join(tempDirPath, comprtConfigFile),
	}

	if err := createTestFile(args.comprtIncludesPath, strings.Join(pkgs, "\n")); err != nil {
		t.Error(err)
	}

	if getComprtIncludes(&includePkgs, args); !bytes.Equal([]byte(strings.Join(includePkgs, "\n")), pkgsByteString) {
		t.Errorf("found the following packages \n%s", strings.Join(includePkgs, "\n"))
	}
}

func TestIntegration(t *testing.T) {
	var codename, target, chrootTestFilePath, comprtConfigFileContents string
	tempDirPath, err := ioutil.TempDir(".", "_"+tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	codename = "buster"
	target = filepath.Join(tempDirPath, "testChroot")
	chrootTestFilePath = filepath.Join("foo")
	comprtConfigFileContents = fmt.Sprintf(`#!/bin/sh

touch %s
	`, chrootTestFilePath)

	args := &cmdArgs{
		comprtIncludesPath: filepath.Join(tempDirPath, comprtIncludeFile),
		comprtConfigPath:   filepath.Join(tempDirPath, comprtConfigFile),
	}

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	if err := os.Mkdir(target, fs.FileMode(0777)); err != nil {
		t.Error(err)
	}
	if err := createTestFile(args.comprtIncludesPath, strings.Join(pkgs, "\n")); err != nil {
		t.Error(err)
	}
	if err := createTestFile(args.comprtConfigPath, comprtConfigFileContents); err != nil {
		t.Error(err)
	}

	debcomprtCmd := exec.Command("debcomprt", "--includes-path", args.comprtIncludesPath, "--config-path", args.comprtConfigPath, codename, target)
	if testing.Verbose() {
		debcomprtCmd.Stdout = os.Stdout
		debcomprtCmd.Stderr = os.Stderr
	}
	if err := debcomprtCmd.Start(); err != nil {
		t.Error(err)
	}
	if err := debcomprtCmd.Wait(); err != nil {
		t.Error(err)
	}

	// returning back to the residing directory before entering the chroot,
	// for reference:
	// https://devsidestory.com/exit-from-a-chroot-with-golang/
	pwd, err := os.Getwd()
	if err != nil {
		t.Error(err)
	}
	fdPwd, err := os.Open(pwd)
	if err != nil {
		t.Error(err)
	}
	defer fdPwd.Close()

	// runs checks on test chroot env
	if err := syscall.Chroot(target); err != nil {
		t.Error(err)
	}
	if err := syscall.Chdir("/"); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(chrootTestFilePath); errors.Is(err, fs.ErrNotExist) {
		t.Error(err)
	}

	for _, pkg := range pkgs {
		dpkgQueryCmd := exec.Command("dpkg-query", "--show", pkg)
		if testing.Verbose() {
			dpkgQueryCmd.Stdout = os.Stdout
			dpkgQueryCmd.Stderr = os.Stderr
		}

		if err := dpkgQueryCmd.Start(); err != nil {
			t.Error(err)
		}
		if err := dpkgQueryCmd.Wait(); err != nil {
			t.Error(err)
		}
	}

	fdPwd.Chdir()
	if err := syscall.Chroot("."); err != nil {
		t.Error(err)
	}
}
