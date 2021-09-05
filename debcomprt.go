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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cavcrosby/appdirs"
	"github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v2"
)

const (
	comprtConfigFile      = "comprtconfig"
	comprtConfigsRepoName = "comprtconfigs"
	comprtConfigsRepoUrl  = "https://github.com/cavcrosby/comprtconfigs"
	comprtIncludeFile     = "comprtinc.txt"
	defaultAlias          = "none"
	defaultDebianMirror   = "http://ftp.us.debian.org/debian/"
	defaultUbuntuMirror   = "http://archive.ubuntu.com/ubuntu/"
	progname              = "debcomprt"
)

var defaultMirrorMappings = map[string]string{
	"buster":  defaultDebianMirror,
	"focal":   defaultUbuntuMirror,
	"hirsute": defaultUbuntuMirror,
}

// A custom callback handler in the event improper cli
// flag/flag arguments/arguments are passed in.
var CustomOnUsageErrorFunc cli.OnUsageErrorFunc = func(context *cli.Context, err error, isSubcommand bool) error {
	cli.ShowAppHelp(context)
	log.Panic(err)
	return err
}

// A type used to store command flag argument values and argument values.
type cmdArgs struct {
	alias              string
	codeName           string
	comprtConfigPath   string
	comprtIncludesPath string
	helpFlagPassedIn   bool
	mirror             string
	passthrough        bool
	passThroughFlags   []string
	quiet              bool
	target             string
}

// Interprets the command arguments passed in. Saving particular flag/flag
// arguments of interest into 'cargs'.
func (cargs *cmdArgs) parseCmdArgs() {
	var localOsArgs []string = os.Args

	// parses out flags to pass to debootstrap
	for i, val := range localOsArgs {
		if i < 1 {
			continue
		} else if val == "--" {
			break
		} else if stringInArr(val, &[]string{"-h", "-help", "--help"}) {
			cargs.helpFlagPassedIn = true
		} else if stringInArr(val, &[]string{"-passthrough", "--passthrough"}) {
			// i + 1 to ignoring iterating over passthrough flag
			for passthroughIndex, passthroughValue := range localOsArgs[i+1:] {
				if strings.HasPrefix(passthroughValue, "-") {
					cargs.passThroughFlags = append(cargs.passThroughFlags, passthroughValue)
				} else {
					// i + 1 to keep passthrough flag
					// i + passthroughIndex + 1 to truncate final flag argument
					localOsArgs = append(localOsArgs[:i+1], localOsArgs[i+passthroughIndex+1:]...)
					break
				}
			}
			break
		}
	}

	app := &cli.App{
		Name:            progname,
		Usage:           "creates debian compartments, an undyling 'target' generated from debootstrap",
		UsageText:       "debcomprt [global options] CODENAME TARGET [MIRROR]",
		Description:     "[WARNING] this tool's cli is not fully POSIX compliant, so POSIX utility cli behavior may not always occur",
		HideHelpCommand: true,
		OnUsageError:    CustomOnUsageErrorFunc,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "alias",
				Aliases:     []string{"a"},
				Value:       defaultAlias,
				Usage:       fmt.Sprintf("use a particular comprt configuration from %v", comprtConfigsRepoUrl),
				Destination: &cargs.alias,
			},
			&cli.BoolFlag{
				Name:        "passthrough",
				Value:       false,
				Usage:       "passes the rest of the flag/flag arguments to debootstrap (e.g. use --foo=bar format)",
				Destination: &cargs.passthrough,
			},
			&cli.BoolFlag{
				Name:        "quiet",
				Aliases:     []string{"q"},
				Value:       false,
				Usage:       "quiet (no output)",
				Destination: &cargs.quiet,
			},
			&cli.PathFlag{
				Name:        "includes-path",
				Aliases:     []string{"i"},
				Value:       cargs.comprtIncludesPath,
				Usage:       "alternative path to comprt includes file",
				Destination: &cargs.comprtIncludesPath,
			},
			&cli.PathFlag{
				Name:        "config-path",
				Aliases:     []string{"c"},
				Value:       cargs.comprtConfigPath,
				Usage:       "alternative path to comptr config file",
				Destination: &cargs.comprtConfigPath,
			},
		},
		Action: func(context *cli.Context) error {
			if context.NArg() < 1 { // CODENAME
				cli.ShowAppHelp(context)
				log.Panic(errors.New("CODENAME argument is required"))
			}

			if context.NArg() < 2 { // TARGET
				cli.ShowAppHelp(context)
				log.Panic(errors.New("TARGET argument is required"))
			} else if _, err := os.Stat(context.Args().Get(1)); errors.Is(err, fs.ErrNotExist) {
				log.Panic(err)
			}

			if context.NArg() < 3 { // MIRROR
				if _, ok := defaultMirrorMappings[context.Args().Get(0)]; !ok {
					log.Panic(errors.New("no default MIRROR could be determined"))
				}
				cargs.mirror = defaultMirrorMappings[context.Args().Get(0)]
			} else {
				cargs.mirror = context.Args().Get(2)
			}

			cargs.codeName = context.Args().Get(0)
			cargs.target = context.Args().Get(1)
			return nil
		},
	}
	app.Run(localOsArgs)
	// Because for some reason github.com/urfave/cli/v2@v2.3.0 does not have a way to
	// eject if a variant of 'help' is passed in!
	if cargs.helpFlagPassedIn {
		os.Exit(0)
	}
}

// Copy the src file to dest. Any existing file will not be overwritten and will not
// copy file attributes.
func copy(src, dest string) error {
	// inspired from:
	// https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file#answer-21061062
	srcFd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	destFd, err := os.OpenFile(dest, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, fs.FileMode(0755))
	if err != nil {
		return err
	}
	defer destFd.Close()

	_, err = io.Copy(destFd, srcFd)
	if err != nil {
		return err
	}
	return destFd.Close()
}

// Reverse the string array. Inspired by:
// https://stackoverflow.com/questions/28058278/how-do-i-reverse-a-slice-in-go
func reverse(arr *[]string) {
	for i, j := 0, len(*arr)-1; i < j; i, j = i+1, j-1 {
		(*arr)[i], (*arr)[j] = (*arr)[j], (*arr)[i]
	}
}

// Look to see if the string is in the string array.
func stringInArr(strArg string, arr *[]string) bool {
	for _, val := range *arr {
		if val == strArg {
			return true
		}
	}
	return false
}

// Get required extra data to be used by the program.
func getProgData(cargs *cmdArgs) error {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	if cargs.alias != defaultAlias {
		_, err := os.Stat(progDataDir)
		if errors.Is(err, fs.ErrNotExist) {
			os.MkdirAll(progDataDir, fs.FileMode(0766))
		} else if err != nil {
			return err
		}
		if _, err := os.Stat(comprtConfigsRepoPath); errors.Is(err, fs.ErrNotExist) {
			_, err := git.PlainClone(comprtConfigsRepoPath, false, &git.CloneOptions{
				URL: comprtConfigsRepoUrl,
			})
			if err != nil {
				return err
			}
		}
		cargs.comprtConfigPath = filepath.Join(comprtConfigsRepoPath, cargs.alias, comprtConfigFile)
		cargs.comprtIncludesPath = filepath.Join(comprtConfigsRepoPath, cargs.alias, cargs.comprtIncludesPath)
	}
	return nil
}

// Read in the comprt includes file and adds the discovered packages into
// includePkgs.
func getComprtIncludes(includePkgs *[]string, cargs *cmdArgs) error {
	// inspired by:
	// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go/16615559#16615559
	file, err := os.Open(cargs.comprtIncludesPath)
	if err != nil {
		// the comprt includes is optional
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		*includePkgs = append(*includePkgs, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// Set the current process's root dir to target. A function to exit out
// of the chroot will be returned.
func Chroot(target string) (func(returnDir string, parentRootFd *os.File) error, error) {
	var fileSystemsToBind []string = []string{"/sys", "/proc", "/dev", "/dev/pts"}
	var fileSystemsUnmountBacklog []string = []string{}
	for _, filesys := range fileSystemsToBind {
		mountPoint := filepath.Join(target, filesys)
		if _, err := os.Stat(mountPoint); errors.Is(err, fs.ErrNotExist) {
			// DISCUSS(cavcrosby): determine if FileMode(s) should differ for each mount.
			os.Mkdir(mountPoint, fs.FileMode(0755))
		}
		if err := syscall.Mount(filesys, filepath.Join(target, filesys), "", syscall.MS_BIND, ""); err != nil {
			return nil, err
		}
	}

	if err := syscall.Chroot(target); err != nil {
		return nil, err
	}

	if err := syscall.Chdir("/"); err != nil { // makes sh happy, otherwise getcwd() for sh fails
		return nil, err
	}

	return func(returnDir string, root *os.File) error {
		if err := root.Chdir(); err != nil {
			return err
		}

		if err := syscall.Chroot("."); err != nil {
			return err
		}

		if err := os.Chdir(returnDir); err != nil {
			return err
		}

		// Unfortunately unmounting filesystems is not as simple when working in code.
		// It seems retrying to unmount the same filesystem previously attempted works
		// after a short sleep. Ordering of the filesystems matter, for reference:
		// https://unix.stackexchange.com/questions/61885/how-to-unmount-a-formerly-chrootd-filesystem#answer-234901
		//
		// MONITOR(cavcrosby): the syscall package is deprecated. At the time of writing, the replacement
		// package for Unix systems is still not at a stable version. So this will need to
		// be revisited at some point. Also for reference: golang.org/x/sys
		reverse(&fileSystemsToBind)
		for _, filesys := range fileSystemsToBind {
			var retries int
			for {
				err := syscall.Unmount(filepath.Join(target, filesys), 0x0)
				if err == nil {
					break
				} else if retries == 1 {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " does not want to unmount, will try again later"}, ""))
					fileSystemsUnmountBacklog = append(fileSystemsUnmountBacklog, filesys)
				} else if errors.Is(err, syscall.EBUSY) {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " is busy, trying again"}, ""))
					retries += 1
					time.Sleep(1 * time.Second)
				} else if errors.Is(err, syscall.EINVAL) {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " is not a mount point...this may be an issue"}, ""))
					break
				} else {
					fmt.Printf("%s: non-expected error thrown %d", progname, err)
					return err
				}

			}
		}

		// in the rare event that a filesystem is being stubborn to unmount
		for _, filesys := range fileSystemsUnmountBacklog {
			var retries int
			for {
				err := syscall.Unmount(filepath.Join(target, filesys), 0x0)
				if err == nil {
					break
				} else if retries == 1 {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " does not want to unmount...AGAIN"}, ""))
					return fmt.Errorf("%s: unable to unmount %v", progname, filesys)
				} else if errors.Is(err, syscall.EBUSY) {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " is busy...AGAIN, trying again"}, ""))
					retries += 1
					time.Sleep(2 * time.Second)
				} else if errors.Is(err, syscall.EINVAL) {
					fmt.Println(strings.Join([]string{progname, ": ", filesys, " is not a mount point...this may be an issue"}, ""))
					break
				} else {
					fmt.Printf("%s: non-expected error thrown %d", progname, err)
					return err
				}
			}
		}

		return nil
	}, nil
}

// Start the main program execution.
func main() {
	var includePkgs, debootstrapCmdArr []string
	cargs := &cmdArgs{ // sets defaults
		comprtConfigPath:   filepath.Join(".", comprtConfigFile),
		comprtIncludesPath: filepath.Join(".", comprtIncludeFile),
		passthrough:        false,
		quiet:              false,
	}
	cargs.parseCmdArgs()

	if err := getProgData(cargs); err != nil {
		log.Panic(err)
	}

	debootstrapPath, err := exec.LookPath("debootstrap")
	if err != nil {
		log.Panic(err)
	}
	debootstrapCmdArr = append(debootstrapCmdArr, debootstrapPath)

	if err := getComprtIncludes(&includePkgs, cargs); err != nil {
		log.Panic(err)
	}
	if includePkgs != nil {
		debootstrapCmdArr = append(debootstrapCmdArr, "--include="+strings.Join(includePkgs, ","))
	}
	if cargs.passThroughFlags != nil {
		debootstrapCmdArr = append(debootstrapCmdArr, cargs.passThroughFlags...)
	}
	// positional arguments
	debootstrapCmdArr = append(debootstrapCmdArr, cargs.codeName, cargs.target, cargs.mirror)

	if err := copy(cargs.comprtConfigPath, filepath.Join(cargs.target, comprtConfigFile)); err != nil {
		log.Panic(err)
	}

	// inspired by:
	// https://stackoverflow.com/questions/39173430/how-to-print-the-realtime-output-of-running-child-process-in-go
	debootstrapCmd := exec.Command(debootstrapPath, debootstrapCmdArr[1:]...)
	debootstrapCmd.Stdout = os.Stdout
	debootstrapCmd.Stderr = os.Stderr
	if err := debootstrapCmd.Start(); err != nil {
		log.Panic(err)
	}
	if err := debootstrapCmd.Wait(); err != nil {
		log.Panic(err)
	}

	previousDir, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}

	root, err := os.Open("/")
	if err != nil {
		log.Panic(err)
	}
	defer root.Close()

	exitChroot, err := Chroot(cargs.target)
	if err != nil {
		log.Panic(err)
	}

	shPath, err := exec.LookPath("sh")
	if err != nil {
		log.Panic(err)
	}
	comprtConfigFileCmd := exec.Command(shPath, filepath.Join("/", comprtConfigFile))
	comprtConfigFileCmd.Stdout = os.Stdout
	comprtConfigFileCmd.Stderr = os.Stderr
	if err := comprtConfigFileCmd.Start(); err != nil {
		log.Panic(err)
	}
	if err := comprtConfigFileCmd.Wait(); err != nil {
		log.Panic(err)
	}

	if err := exitChroot(previousDir, root); err != nil {
		log.Panic(err)
	}

	os.Exit(0)
}
