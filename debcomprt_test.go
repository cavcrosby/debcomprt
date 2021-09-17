// Copyright 2021 Conner Crosby
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/cavcrosby/appdirs"
)

const (
	tempDir = "debcomprt"
)

var pkgs []string = []string{"autoconf", "git", "wget"}

// createTestFile creates a test file that is solely meant for testing. This file
// should be created on the intentions of allowing any test to access it.
func createTestFile(filePath, contents string) error {
	err := ioutil.WriteFile(
		filePath,
		[]byte(contents),
		ModeFile|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_W|OS_GROUP_X|OS_OTH_R|OS_OTH_W|OS_OTH_X),
	)
	if err != nil {
		return err
	}

	return nil
}

// Get a file's system status.
func stat(filePath string, stat *syscall.Stat_t) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	// Could have also used https://pkg.go.dev/syscall#Stat but syscall is deprecated.
	// MONITOR(cavcrosby): still means this implementation will need to be revisited.
	//
	// inspired by: https://stackoverflow.com/questions/28339240/get-file-inode-in-go
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("Not a %v", reflect.TypeOf(stat))
	}
	// fileStat should not contain further pointers, though this may change depending
	// on the implementation. For reference: https://pkg.go.dev/syscall#Stat_t
	*stat = *fileStat
	return nil
}

func TestCopy(t *testing.T) {
	// inspired by:
	// https://stackoverflow.com/questions/29505089/how-can-i-compare-two-files-in-golang#answer-29528747
	tempDirPath, err := ioutil.TempDir("", tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	var filePath1, filePath2, fileContents string
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

func TestGetProgData(t *testing.T) {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	cargs := &cmdArgs{
		alias: "altaria",
	}

	if err := getProgData(cargs); err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(progDataDir)

	if _, err := os.Stat(progDataDir); errors.Is(err, fs.ErrNotExist) {
		t.Error(err)
	}

	if _, err := os.Stat(comprtConfigsRepoPath); errors.Is(err, fs.ErrNotExist) {
		t.Error(err)
	}
}

func TestGetComprtIncludes(t *testing.T) {
	tempDirPath, err := ioutil.TempDir("", "_"+tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	pkgsByteString := []byte(strings.Join(pkgs, "\n"))
	cargs := &cmdArgs{
		comprtConfigPath:   filepath.Join(tempDirPath, comprtConfigFile),
		comprtIncludesPath: filepath.Join(tempDirPath, comprtIncludeFile),
	}

	if err := createTestFile(cargs.comprtIncludesPath, strings.Join(pkgs, "\n")); err != nil {
		t.Error(err)
	}

	var includePkgs []string
	if getComprtIncludes(&includePkgs, cargs); !bytes.Equal([]byte(strings.Join(includePkgs, "\n")), pkgsByteString) {
		t.Errorf("found the following packages \n%s", strings.Join(includePkgs, "\n"))
	}
}

func TestChroot(t *testing.T) {
	// e.g. /tmp/${tempDir} on Unix systems
	tempDirPath, err := ioutil.TempDir("", "_"+tempDir)
	if err != nil {
		// DISCUSS(cavcrosby): determine if t.Errors at any point should just exit the prog.
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	root, err := os.Open("/")
	if err != nil {
		t.Error(err)
	}
	defer root.Close()

	var parentRootStat *syscall.Stat_t = &syscall.Stat_t{}
	parentStatErr := stat("/", parentRootStat)
	if parentStatErr != nil {
		t.Error(err)
	}

	exitChroot, err := Chroot(tempDirPath)
	if err != nil {
		t.Error(err)
	}

	var rootStat *syscall.Stat_t = &syscall.Stat_t{}
	statErr := stat("/", rootStat)
	if statErr != nil {
		t.Error(err)
	}

	if rootStat.Ino == parentRootStat.Ino {
		t.Error("was unable to chroot into target")
	}

	if err := exitChroot("/", root); err != nil {
		t.Error(err)
	}

	var rootStat2 *syscall.Stat_t = &syscall.Stat_t{}
	statErr2 := stat("/", rootStat2)
	if statErr2 != nil {
		t.Error(err)
	}

	if rootStat2.Ino != parentRootStat.Ino {
		t.Error("was unable to exit chroot")
	}
}

func TestIntegration(t *testing.T) {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	_, err := os.Stat(progDataDir)
	if errors.Is(err, fs.ErrNotExist) {
		os.MkdirAll(progDataDir, os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_X|OS_OTH_R|OS_OTH_X))
	} else if err != nil {
		t.Error(err)
	}

	tempDirPath, err := os.MkdirTemp(progDataDir, "_"+tempDir)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tempDirPath)

	var codename, target, chrootTestFilePath, comprtConfigFileContents string
	codename = "buster"
	target = filepath.Join(tempDirPath, "testChroot")
	chrootTestFilePath = filepath.Join("foo")
	comprtConfigFileContents = fmt.Sprintf(`#!/bin/sh

touch %s
	`, chrootTestFilePath)

	cargs := &cmdArgs{
		comprtConfigPath:   filepath.Join(tempDirPath, comprtConfigFile),
		comprtIncludesPath: filepath.Join(tempDirPath, comprtIncludeFile),
	}

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	mkTargetErr := os.Mkdir(
		target,
		os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_W|OS_GROUP_X|OS_OTH_R|OS_OTH_W|OS_OTH_X),
	)
	if mkTargetErr != nil {
		t.Error(mkTargetErr)
	}

	if err := createTestFile(cargs.comprtIncludesPath, strings.Join(pkgs, "\n")); err != nil {
		t.Error(err)
	}
	if err := createTestFile(cargs.comprtConfigPath, comprtConfigFileContents); err != nil {
		t.Error(err)
	}

	debcomprtCmd := exec.Command("debcomprt", "--includes-path", cargs.comprtIncludesPath, "--config-path", cargs.comprtConfigPath, codename, target)
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
