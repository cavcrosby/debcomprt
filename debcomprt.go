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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cavcrosby/appdirs"
	"github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v2"
)

// TODO(cavcrosby): Remove comprtIncludeFile extension, as the other program
// specific files do not have a extension.

const (
	comprtConfigFile      = "comprtconfig"
	comprtConfigsRepoName = "comprtconfigs"
	comprtConfigsRepoUrl  = "https://github.com/cavcrosby/comprtconfigs"
	comprtIncludeFile     = "comprtinc.txt"
	defaultComprtUid      = 1224
	defaultDebianMirror   = "http://ftp.us.debian.org/debian/"
	defaultUbuntuMirror   = "http://archive.ubuntu.com/ubuntu/"
	noAlias               = "none"
	progname              = "debcomprt"
)

// inspired by:
// https://stackoverflow.com/questions/28969455/how-to-properly-instantiate-os-filemode
const (
	ModeFile       = 0x0
	OS_READ        = 04
	OS_WRITE       = 02
	OS_EX          = 01
	OS_USER_SHIFT  = 6
	OS_GROUP_SHIFT = 3
	OS_OTH_SHIFT   = 0
	OS_USER_R      = OS_READ << OS_USER_SHIFT
	OS_USER_W      = OS_WRITE << OS_USER_SHIFT
	OS_USER_X      = OS_EX << OS_USER_SHIFT
	OS_USER_RW     = OS_USER_R | OS_USER_W
	OS_USER_RWX    = OS_USER_RW | OS_USER_X
	OS_GROUP_R     = OS_READ << OS_GROUP_SHIFT
	OS_GROUP_W     = OS_WRITE << OS_GROUP_SHIFT
	OS_GROUP_X     = OS_EX << OS_GROUP_SHIFT
	OS_GROUP_RW    = OS_GROUP_R | OS_GROUP_W
	OS_GROUP_RWX   = OS_GROUP_RW | OS_GROUP_X
	OS_OTH_R       = OS_READ << OS_OTH_SHIFT
	OS_OTH_W       = OS_WRITE << OS_OTH_SHIFT
	OS_OTH_X       = OS_EX << OS_OTH_SHIFT
	OS_OTH_RW      = OS_OTH_R | OS_OTH_W
	OS_OTH_RWX     = OS_OTH_RW | OS_OTH_X
)

var (
	reFindEnvVar = regexp.MustCompile(`(?P<name>^[a-zA-Z_]\w*)=(?P<value>.+)`)
)

// Mappings of codenames to respective a package repository.
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
	command            string
	comprtConfigPath   string
	comprtIncludesPath string
	helpFlagPassedIn   bool
	mirror             string
	passthrough        bool
	passThroughFlags   []string
	preprocessAliases  bool
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
		} else if stringInArr(val, &[]string{"-e", "-alias-envvar", "--alias-envvar"}) {
			var envVar string = localOsArgs[i+1]

			if reFindEnvVar.FindStringIndex(envVar) == nil {
				log.Panic(errors.New("an env var was not properly formatted"))
			}
			envVarArr := reFindEnvVar.FindStringSubmatch(envVar)
			envVarName, envVarValue := envVarArr[1], envVarArr[2]
			os.Setenv(envVarName, envVarValue)
			// i + 1 to keep alias-envvar flag
			// i + 2 only to truncate the flag's argument
			localOsArgs = append(localOsArgs[:i+1], localOsArgs[i+2:]...)
		}
	}
	os.Setenv("DEBCOMPRT_DEFAULT_LOGIN_UID", strconv.Itoa(defaultComprtUid))

	// TODO(cavcrosby): Pretty sure --quiet was never implemented in any part of the
	// code!
	app := &cli.App{
		Name:            progname,
		Usage:           "manages debian compartments (comprt), an underlying 'target' generated from debootstrap",
		UsageText:       "debcomprt [global options] [command] CODENAME TARGET [MIRROR]",
		Description:     "[WARNING] this tool's cli is not fully POSIX compliant, so POSIX utility cli behavior may not always occur",
		HideHelpCommand: true,
		OnUsageError:    CustomOnUsageErrorFunc,
		Commands: []*cli.Command{
			{
				Name:      "chroot",
				Usage:     "chroots into a debian compartment",
				UsageText: "debcomprt [options] create TARGET",
				Action: func(context *cli.Context) error {
					if context.NArg() < 1 { // TARGET
						cli.ShowAppHelp(context)
						log.Panic(errors.New("TARGET argument is required"))
					} else if _, err := os.Stat(context.Args().Get(0)); errors.Is(err, fs.ErrNotExist) {
						log.Panic(err)
					}

					cargs.command = context.Command.Name
					cargs.target = context.Args().Get(0)
					return nil
				},
			},
			{
				Name:      "create",
				Usage:     "creates a debian compartment",
				UsageText: "debcomprt [options] create CODENAME TARGET [MIRROR]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "alias",
						Aliases:     []string{"a"},
						Value:       noAlias,
						Usage:       fmt.Sprintf("use a particular comprt configuration from %v", comprtConfigsRepoUrl),
						Destination: &cargs.alias,
					},
					&cli.BoolFlag{
						Name:        "alias-envvar",
						Aliases:     []string{"e"},
						Usage:       "preprocess all the aliases files by evaluating these env vars (ex. <flag> foo=bar <flag> bar=baz)",
						Destination: &cargs.preprocessAliases,
					},
					&cli.BoolFlag{
						Name:        "passthrough",
						Value:       false,
						Usage:       "pass the rest of the flag/flag arguments to debootstrap (e.g. use --foo=bar flag format)",
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
						Usage:       "alternative `PATH` to comprt includes file",
						Destination: &cargs.comprtIncludesPath,
					},
					&cli.PathFlag{
						Name:        "config-path",
						Aliases:     []string{"c"},
						Value:       cargs.comprtConfigPath,
						Usage:       "alternative `PATH` to comptr config file",
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

					cargs.command = context.Command.Name
					cargs.codeName = context.Args().Get(0)
					cargs.target = context.Args().Get(1)
					return nil
				},
			},
		},
		Action: func(context *cli.Context) error {
			if context.NArg() < 1 {
				cli.ShowAppHelp(context)
				os.Exit(1)
			}

			// this shouldn't get here as long as a subcommand is required!
			return nil
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
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

	destFd, err := os.OpenFile(
		dest,
		syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY,
		ModeFile|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_X|OS_OTH_R|OS_OTH_X),
	)
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

// Look in a file that has some form of standardized file format
// (e.g. /etc/passwd, /etc/os-release) and locate a 'field' among
// the rows based on a regex for another field. Fields are a sequence
// of elements separated by a field separator.
func locateField(filePath, fieldSep string, matchIndex, returnIndex int, matchRegex *regexp.Regexp) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), fieldSep)
		if matchRegex.FindStringIndex(fields[matchIndex]) != nil {
			return fields[returnIndex], nil
		}
	}
	return "", nil
}

// Get required extra data to be used by the program.
func getProgData(cargs *cmdArgs) error {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	if cargs.alias != noAlias {
		_, err := os.Stat(progDataDir)
		if errors.Is(err, fs.ErrNotExist) {
			os.MkdirAll(progDataDir, os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_X|OS_OTH_R|OS_OTH_X))
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
		} else {
			var pullOpts git.PullOptions = git.PullOptions{RemoteName: "origin"}
			comprtRepo, err := git.PlainOpen(comprtConfigsRepoPath)
			if err != nil {
				return err
			}

			gitWorkingDir, err := comprtRepo.Worktree()
			if err != nil {
				return err
			}
			gitWorkingDir.Pull(&pullOpts)
		}

		if cargs.preprocessAliases {
			makePath, err := exec.LookPath("make")
			if err != nil {
				log.Panic(err)
			}

			makeCmd := exec.Command(makePath, "PREPROCESS_ALIASES=1", cargs.alias)
			makeCmd.Dir = comprtConfigsRepoPath
			if _, err := makeCmd.Output(); err != nil {
				log.Panic(err)
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
func Chroot(target string) (func() error, error) {
	var fileSystemsToBind []string = []string{"/sys", "/proc", "/dev", "/dev/pts"}
	returnDir, err := os.Getwd()
	if err != nil {
		log.Panic(err)
	}

	root, err := os.Open("/")
	if err != nil {
		log.Panic(err)
	}

	for _, filesys := range fileSystemsToBind {
		mountPoint := filepath.Join(target, filesys)
		if _, err := os.Stat(mountPoint); errors.Is(err, fs.ErrNotExist) {
			var fileMode fs.FileMode
			// filemode for /sys based on current workstation (same for /proc except url) and
			// https://askubuntu.com/questions/341939/why-cant-i-create-a-directory-in-sys
			switch filesys {
			case "/sys":
				fileMode = os.ModeDir | (OS_USER_R | OS_USER_X | OS_GROUP_R | OS_GROUP_X | OS_OTH_R | OS_OTH_X)
			case "/proc":
				fileMode = os.ModeDir | (OS_USER_R | OS_USER_X | OS_GROUP_R | OS_GROUP_X | OS_OTH_R | OS_OTH_X)
			case "/dev":
				fileMode = os.ModeDir | (OS_USER_R | OS_USER_W | OS_USER_X | OS_GROUP_R | OS_GROUP_X | OS_OTH_R | OS_OTH_X)
			case "/dev/pts":
				fileMode = os.ModeDir | (OS_USER_R | OS_USER_W | OS_USER_X | OS_GROUP_R | OS_GROUP_X | OS_OTH_R | OS_OTH_X)
			}
			os.Mkdir(
				mountPoint,
				fileMode,
			)
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

	return func() error {
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
		var fileSystemsUnmountBacklog []string = []string{}
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

// Provide an interactive shell into the comprt.
func runInteractiveChroot(target string) error {
	var uidRegex *regexp.Regexp = regexp.MustCompile(strconv.Itoa(defaultComprtUid))
	defaultComprtUsername, err := locateField(filepath.Join(target, "/etc/passwd"), ":", 2, 0, uidRegex)
	if err != nil{
		return err
	}
	
	exitChroot, err := Chroot(target)
	if err != nil {
		return err
	}

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return err
	}

	suPath, err := exec.LookPath("su")
	if err != nil {
		return err
	}

	bashCmd := exec.Command(suPath, "--shell", bashPath, "--login", defaultComprtUsername)
	bashCmd.Stdin = os.Stdin
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr
	if err := bashCmd.Start(); err != nil {
		return err
	}
	if err := bashCmd.Wait(); err != nil {
		return err
	}

	if err := exitChroot(); err != nil {
		return err
	}
	return nil
}

// Create a debian comprt.
func createComprt(cargs *cmdArgs) error {
	if err := getProgData(cargs); err != nil {
		return err
	}

	debootstrapPath, err := exec.LookPath("debootstrap")
	if err != nil {
		return err
	}

	var includePkgs, debootstrapCmdArr []string
	debootstrapCmdArr = append([]string{debootstrapPath}, debootstrapCmdArr...)

	if err := getComprtIncludes(&includePkgs, cargs); err != nil {
		return err
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
		return err
	}

	// inspired by:
	// https://stackoverflow.com/questions/39173430/how-to-print-the-realtime-output-of-running-child-process-in-go
	debootstrapCmd := exec.Command(debootstrapPath, debootstrapCmdArr[1:]...)
	debootstrapCmd.Stdout = os.Stdout
	debootstrapCmd.Stderr = os.Stderr
	if err := debootstrapCmd.Start(); err != nil {
		return err
	}
	if err := debootstrapCmd.Wait(); err != nil {
		return err
	}

	exitChroot, err := Chroot(cargs.target)
	if err != nil {
		return err
	}

	shPath, err := exec.LookPath("sh")
	if err != nil {
		return err
	}
	comprtConfigFileCmd := exec.Command(shPath, filepath.Join("/", comprtConfigFile))
	comprtConfigFileCmd.Stdout = os.Stdout
	comprtConfigFileCmd.Stderr = os.Stderr
	if err := comprtConfigFileCmd.Start(); err != nil {
		return err
	}
	if err := comprtConfigFileCmd.Wait(); err != nil {
		return err
	}

	if err := exitChroot(); err != nil {
		return err
	}
	return nil
}

// Start the main program execution.
func main() {
	cargs := &cmdArgs{ // sets defaults
		comprtConfigPath:   filepath.Join(".", comprtConfigFile),
		comprtIncludesPath: filepath.Join(".", comprtIncludeFile),
		passthrough:        false,
		quiet:              false,
	}
	cargs.parseCmdArgs()

	switch cargs.command {
	case "chroot":
		if err := runInteractiveChroot(cargs.target); err != nil {
			log.Panic(err)
		}
	case "create":
		if err := createComprt(cargs); err != nil {
			log.Panic(err)
		}
	}

	os.Exit(0)
}
