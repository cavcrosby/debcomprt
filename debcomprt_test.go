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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/cavcrosby/appdirs"
)

const (
	tempDir = "debcomprt"
)

var (
	testPkgs                       = []string{"autoconf", "git", "wget"}
	testCodeCame                   = "buster"
	testComprtConfigFileChrootPath = filepath.Join("foo")
)

var testComprtConfigFileContents = fmt.Sprintf(`#!/bin/sh

touch %s
	`, testComprtConfigFileChrootPath)

// createTestFile creates a test file that is solely meant for testing. This file
// is created on the intentions of allowing anything to access it.
func createTestFile(fPath, contents string) error {
	if err := ioutil.WriteFile(
		fPath,
		[]byte(contents),
		ModeFile|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_W|OS_GROUP_X|OS_OTH_R|OS_OTH_W|OS_OTH_X),
	); err != nil {
		return err
	}

	return nil
}

// Get a file's system status.
func stat(fPath string, stat *syscall.Stat_t) error {
	fileInfo, err := os.Stat(fPath)
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

// Setup the program's data directory. Ensure any validation/checking is done here.
func setupProgDataDir() (string, error) {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	if _, err := os.Stat(progDataDir); errors.Is(err, fs.ErrNotExist) {
		os.MkdirAll(progDataDir, os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_X|OS_OTH_R|OS_OTH_X))
	} else if err != nil {
		return "", err
	}

	return progDataDir, nil
}

func TestCopy(t *testing.T) {
	// inspired by:
	// https://stackoverflow.com/questions/29505089/how-can-i-compare-two-files-in-golang#answer-29528747
	tempDirPath, err := os.MkdirTemp("", "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	var filePath1, filePath2, fileContents string
	fileContents = "hello\nthere!\n"
	filePath1 = filepath.Join(tempDirPath, "foo")
	filePath2 = filepath.Join(tempDirPath, "bar")

	if err := createTestFile(filePath1, fileContents); err != nil {
		t.Fatal(err)
	}
	if err := copy(filePath1, filePath2); err != nil {
		t.Fatal(err)
	}

	file1, err := ioutil.ReadFile(filePath1)
	if err != nil {
		t.Fatal(err)
	}
	file2, err := ioutil.ReadFile(filePath2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(file1, file2) {
		t.Fatalf("%s is not the same as %s", filePath1, filePath2)
	}
}

func TestCopyDestAlreadyExists(t *testing.T) {
	tempDirPath, err := os.MkdirTemp("", "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	var filePath1, filePath2, fileContents string
	fileContents = "hello\nthere!\n"
	filePath1 = filepath.Join(tempDirPath, "foo")
	filePath2 = filepath.Join(tempDirPath, "bar")

	if err := createTestFile(filePath1, fileContents); err != nil {
		t.Fatal(err)
	}
	if err := copy(filePath1, filePath2); err != nil {
		t.Fatal(err)
	}

	if err := copy(filePath1, filePath2); err == nil {
		t.Fatal("dest was overwritten with second call to copy!")
	} else if !errors.Is(err, syscall.EEXIST) {
		t.Fatalf("a non-expected error has occurred: %d", err)
	}
}

func TestGetProgData(t *testing.T) {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	pconfs := &progConfigs{
		alias: "altaria",
	}

	if err := getProgData(pconfs.alias, false, pconfs); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(progDataDir)

	if _, err := os.Stat(progDataDir); errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := os.Stat(comprtConfigsRepoPath); errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestGetComprtIncludes(t *testing.T) {
	tempDirPath, err := os.MkdirTemp("", "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	pconfs := &progConfigs{
		comprtConfigPath:   filepath.Join(tempDirPath, comprtConfigFile),
		comprtIncludesPath: filepath.Join(tempDirPath, comprtIncludeFile),
	}

	if err := createTestFile(pconfs.comprtIncludesPath, strings.Join(testPkgs, "\n")); err != nil {
		t.Fatal(err)
	}

	var includePkgs []string
	pkgsByteString := []byte(strings.Join(testPkgs, "\n"))
	getComprtIncludes(&includePkgs, pconfs.comprtIncludesPath)

	if !bytes.Equal([]byte(strings.Join(includePkgs, "\n")), pkgsByteString) {
		t.Fatalf("found the following packages \n%s", strings.Join(includePkgs, "\n"))
	}
}

func TestChroot(t *testing.T) {
	tempDirPath, err := os.MkdirTemp("", "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	root, err := os.Open("/")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	var parentRootStat *syscall.Stat_t = &syscall.Stat_t{}
	if err := stat("/", parentRootStat); err != nil {
		t.Fatal(err)
	}

	// For reference on determining if the process is in a chroot:
	// https://unix.stackexchange.com/questions/14345/how-do-i-tell-im-running-in-a-chroot
	exitChroot, errs := Chroot(tempDirPath)
	if errs != nil {
		t.Fatal(errs)
	}
	defer func() {
		if err := exitChroot(); err != nil {
			t.Fatal(err)
		}

		var rootStat2 *syscall.Stat_t = &syscall.Stat_t{}
		if err := stat("/", rootStat2); err != nil {
			t.Fatal(err)
		}

		if rootStat2.Ino != parentRootStat.Ino {
			t.Fatal("was unable to exit chroot")
		}
	}()

	var rootStat *syscall.Stat_t = &syscall.Stat_t{}
	if err := stat("/", rootStat); err != nil {
		t.Fatal(err)
	}

	if rootStat.Ino == parentRootStat.Ino {
		t.Fatal("was unable to chroot into target")
	}
}

func TestCreateCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	progDataDir, err := setupProgDataDir()
	if err != nil {
		t.Fatal(err)
	}

	tempDirPath, err := os.MkdirTemp(progDataDir, "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	var testTarget string = filepath.Join(tempDirPath, "testChroot")
	if err := os.Mkdir(
		testTarget,
		os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_W|OS_GROUP_X|OS_OTH_R|OS_OTH_W|OS_OTH_X),
	); err != nil {
		t.Fatal(err)
	}

	var comprtIncludesPath string = filepath.Join(tempDirPath, comprtIncludeFile)
	if err := createTestFile(comprtIncludesPath, strings.Join(testPkgs, "\n")); err != nil {
		t.Fatal(err)
	}

	var comprtConfigPath string = filepath.Join(tempDirPath, comprtConfigFile)
	if err := createTestFile(comprtConfigPath, testComprtConfigFileContents); err != nil {
		t.Fatal(err)
	}

	debcomprtCmd := exec.Command("debcomprt", "create", "--includes-path", comprtIncludesPath, "--config-path", comprtConfigPath, testCodeCame, testTarget)
	if testing.Verbose() {
		debcomprtCmd.Stdout = os.Stdout
		debcomprtCmd.Stderr = os.Stderr
	}
	if err := debcomprtCmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := debcomprtCmd.Wait(); err != nil {
		t.Fatal(err)
	}

	exitChroot, errs := Chroot(testTarget)
	if errs != nil {
		t.Fatal(errs)
	}
	defer func() {
		if err := exitChroot(); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := os.Stat(testComprtConfigFileChrootPath); errors.Is(err, fs.ErrNotExist) {
		t.Error(err)
	}

	for _, pkg := range testPkgs {
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
}

func TestChrootCommandIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	progDataDir, err := setupProgDataDir()
	if err != nil {
		t.Fatal(err)
	}

	tempDirPath, err := os.MkdirTemp(progDataDir, "_"+tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirPath)

	var testTarget string = filepath.Join(tempDirPath, "testChroot")
	pconfs := &progConfigs{
		comprtConfigPath: filepath.Join(tempDirPath, comprtConfigFile),
		target:           testTarget,
	}

	if err := os.Mkdir(
		pconfs.target,
		os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_W|OS_GROUP_X|OS_OTH_R|OS_OTH_W|OS_OTH_X),
	); err != nil {
		t.Fatal(err)
	}

	if err := createTestFile(pconfs.comprtConfigPath, testComprtConfigFileContents); err != nil {
		t.Fatal(err)
	}

	var debootstrapCmdArr []string
	createDebootstrapArgList(
		&debootstrapCmdArr,
		nil,
		"",
		testCodeCame,
		pconfs.target,
		defaultMirrorMappings[testCodeCame],
	)
	if errs := createComprt(pconfs.comprtConfigPath, pconfs.target, noAlias, "", false, &debootstrapCmdArr); errs != nil {
		t.Fatal(errs)
	}

	debcomprtCmd := exec.Command("debcomprt", "chroot", testTarget)
	if testing.Verbose() {
		debcomprtCmd.Stderr = os.Stderr
	}
	debcomprtCmdStdin, err := debcomprtCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	debcomprtCmdStdout, err := debcomprtCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := debcomprtCmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(debcomprtCmdStdin, "id --user\n"); err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(debcomprtCmdStdout)
	debcomprtOut, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if debcomprtOut == "" {
		t.Fatal("unable to get effective uid of shell running in chroot")
	}

	uid, err := strconv.Atoi(strings.TrimSuffix(debcomprtOut, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if uid != defaultComprtUid {
		t.Fatal("was unable to chroot into target with the default comprt uid!")
	}
	// needed to send EOF to the interactive shell
	debcomprtCmdStdin.Close()

	if err := debcomprtCmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
