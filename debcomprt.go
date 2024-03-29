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

//go:generate go run -mod=vendor github.com/cavcrosby/genruntime-vars
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
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v2"
)

const (
	comprtConfigFile      = "comprtconfig"
	comprtConfigsRepoName = "comprtconfigs"
	comprtConfigsRepoUrl  = "https://github.com/cavcrosby/comprtconfigs"
	comprtIncludeFile     = "comprtinc"

	// Derived uid based on debian's package policy for uids/gids. For reference:
	// https://www.debian.org/doc/debian-policy/ch-opersys.html#uid-and-gid-classes
	//
	// The uid serves as a default user to 'login in as' when choosing to chroot into
	// a comprt.
	defaultComprtUid      = 1224
	defaultComprtUserName = "debcomprt"

	defaultDebianMirror = "http://ftp.us.debian.org/debian/"
	defaultUbuntuMirror = "http://archive.ubuntu.com/ubuntu/"
	rootUid             = 0
	noAlias             = "none"
	progname            = "debcomprt"
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

// A custom callback handler in the event improper cli flag/flag
// arguments/arguments are passed in.
var CustomOnUsageErrorFunc cli.OnUsageErrorFunc = func(context *cli.Context, err error, isSubcommand bool) error {
	cli.ShowAppHelp(context)
	log.Panic(err)
	return err
}

// A type used to store command flag argument values and argument values.
type progConfigs struct {
	alias              string
	codeName           string
	command            string
	comprtConfigPath   string
	comprtIncludesPath string
	cryptPassword      string
	helpFlagPassedIn   bool
	mirror             string
	passthrough        bool
	passThroughFlags   []string
	preprocessAliases  bool
	quiet              bool
	target             string
}

// Interpret the command arguments passed in. Saving particular flag/flag
// arguments of interest into 'pconfs'.
func (pconfs *progConfigs) parseCmdArgs() {
	var localOsArgs []string = os.Args

	for i, val := range localOsArgs {
		if i < 1 {
			continue
		} else if val == "--" {
			break
		} else if stringsInArr([]string{"-a", "-alias", "--alias"}, &localOsArgs) &&
			stringsInArr([]string{"-p", "-crypt-password", "--crypt-password"}, &localOsArgs) {
			log.Panic(errors.New("--crypt-password cannot be used with --alias"))
		} else if stringsInArr([]string{"-a", "-alias", "--alias"}, &localOsArgs) &&
			stringsInArr([]string{"-c", "-config-path", "--config-path"}, &localOsArgs) {
			log.Panic(errors.New("--config-path cannot be used with --alias"))
		} else if stringInArr(val, &[]string{"-h", "-help", "--help"}) {
			pconfs.helpFlagPassedIn = true
		} else if stringInArr(val, &[]string{"-passthrough", "--passthrough"}) {
			// i + 1 to ignoring iterating over passthrough flag
			for passthroughIndex, passthroughValue := range localOsArgs[i+1:] {
				if strings.HasPrefix(passthroughValue, "-") {
					pconfs.passThroughFlags = append(pconfs.passThroughFlags, passthroughValue)
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
				log.Panic(fmt.Errorf("%v is not a properly formatted env var", envVar))
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

					pconfs.command = context.Command.Name
					pconfs.target = context.Args().Get(0)
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
						Destination: &pconfs.alias,
					},
					&cli.BoolFlag{
						Name:        "alias-envvar",
						Value:       false,
						Aliases:     []string{"e"},
						Usage:       "preprocess all the aliases files by evaluating these env vars (ex. <flag> foo=bar <flag> bar=baz)",
						Destination: &pconfs.preprocessAliases,
					},
					&cli.BoolFlag{
						Name:        "passthrough",
						Value:       false,
						Usage:       "pass the rest of the flag/flag arguments to debootstrap (e.g. use --foo=bar flag format)",
						Destination: &pconfs.passthrough,
					},
					&cli.BoolFlag{
						Name:        "quiet",
						Aliases:     []string{"q"},
						Value:       false,
						Usage:       "quiet (no output)",
						Destination: &pconfs.quiet,
					},
					&cli.PathFlag{
						Name:        "includes-path",
						Aliases:     []string{"i"},
						Value:       pconfs.comprtIncludesPath,
						Usage:       "alternative `PATH` to comprt includes file",
						Destination: &pconfs.comprtIncludesPath,
					},
					&cli.PathFlag{
						Name:        "config-path",
						Aliases:     []string{"c"},
						Value:       pconfs.comprtConfigPath,
						Usage:       "alternative `PATH` to comptr config file",
						Destination: &pconfs.comprtConfigPath,
					},
					&cli.StringFlag{
						Name:        "crypt-password",
						Aliases:     []string{"p"},
						Value:       "",
						Usage:       fmt.Sprintf("set a password for the default comprt user: %v", defaultComprtUserName),
						Destination: &pconfs.cryptPassword,
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
						pconfs.mirror = defaultMirrorMappings[context.Args().Get(0)]
					} else {
						pconfs.mirror = context.Args().Get(2)
					}

					pconfs.command = context.Command.Name
					pconfs.codeName = context.Args().Get(0)
					pconfs.target = context.Args().Get(1)
					return nil
				},
			},
		},
		Action: func(context *cli.Context) error {
			if context.NArg() < 1 || context.Command.Name == "" {
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
	if pconfs.helpFlagPassedIn {
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

	if _, err := io.Copy(destFd, srcFd); err != nil {
		return err
	}

	return nil
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

// Look to see if a string from the strArgs is in the string array.
func stringsInArr(strArgs []string, arr *[]string) bool {
	for _, val := range strArgs {
		if stringInArr(val, arr) {
			return true
		}
	}

	return false
}

// Look in a file that has some form of standardized file format
// (e.g. /etc/passwd, /etc/os-release) and locate a 'field' among
// the rows based on a regex for another field. Fields are a sequence
// of characters separated by a field separator (or a character). Field
// indexes start at 0.
func locateField(fPath string, fieldSepRegex *regexp.Regexp, matchIndex, returnIndex int, matchRegex *regexp.Regexp) (string, error) {
	file, err := os.Open(fPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var allFields int = -1
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := fieldSepRegex.Split(scanner.Text(), allFields)
		if len(fields) <= matchIndex {
			continue
		} else if matchRegex.FindStringIndex(fields[matchIndex]) != nil {
			return fields[returnIndex], nil
		}
	}

	return "", nil
}

// Get required extra data to be used by the program.
func getProgData(alias string, preprocessAliases bool, pconfs *progConfigs) error {
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	if alias != noAlias {
		if _, err := os.Stat(progDataDir); errors.Is(err, fs.ErrNotExist) {
			os.MkdirAll(progDataDir, os.ModeDir|(OS_USER_R|OS_USER_W|OS_USER_X|OS_GROUP_R|OS_GROUP_X|OS_OTH_R|OS_OTH_X))
		} else if err != nil {
			return err
		}

		if _, err := os.Stat(comprtConfigsRepoPath); errors.Is(err, fs.ErrNotExist) {
			if _, err := git.PlainClone(comprtConfigsRepoPath, false, &git.CloneOptions{
				URL: comprtConfigsRepoUrl,
			}); err != nil {
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

		if preprocessAliases {
			makePath, err := exec.LookPath("make")
			if err != nil {
				log.Panic(err)
			}

			makeCmd := exec.Command(makePath, "PREPROCESS_ALIASES=1", alias)
			makeCmd.Dir = comprtConfigsRepoPath
			if _, err := makeCmd.Output(); err != nil {
				log.Panic(err)
			}
		}

		pconfs.comprtConfigPath = filepath.Join(comprtConfigsRepoPath, alias, comprtConfigFile)
		pconfs.comprtIncludesPath = filepath.Join(comprtConfigsRepoPath, alias, comprtIncludeFile)
	}

	return nil
}

// Read in the comprt includes file and adds the discovered packages into
// includePkgs.
func getComprtIncludes(includePkgs *[]string, comprtIncludesPath string) error {
	// inspired by:
	// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go/16615559#16615559
	file, err := os.Open(comprtIncludesPath)
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

// Mount filesystems found on devices to their respective location(s) on the
// target. As if the process had chooted to the target.
func mountChrootFileSystems(devicesToMount []string, target string) ([]string, error) {
	var fileSystemsMounted []string
	for _, filesys := range devicesToMount {
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
			default:
				fileMode = os.ModeDir | (OS_USER_R | OS_USER_W | OS_USER_X | OS_GROUP_R | OS_GROUP_X | OS_OTH_R | OS_OTH_X)
			}
			if err := os.Mkdir(
				mountPoint,
				fileMode,
			); err != nil {
				return fileSystemsMounted, err
			}
		}
		if err := syscall.Mount(filesys, filepath.Join(target, filesys), "", syscall.MS_BIND, ""); err != nil {
			return fileSystemsMounted, err
		}
		fileSystemsMounted = append(fileSystemsMounted, filesys)
	}
	return fileSystemsMounted, nil
}

// Unmount filesystems found on devices starting in the tree hierarchy of the target.
func unMountChrootFileSystems(devicesToMount []string, target string) error {
	// Unfortunately unmounting filesystems is not as simple when working in code.
	// It seems retrying to unmount the same filesystem previously attempted works
	// after a short sleep. Ordering of the filesystems matter, for reference:
	// https://unix.stackexchange.com/questions/61885/how-to-unmount-a-formerly-chrootd-filesystem#answer-234901
	//
	// MONITOR(cavcrosby): the syscall package is deprecated. At the time of writing, the replacement
	// package for Unix systems is still not at a stable version. So this will need to
	// be revisited at some point. Also for reference: golang.org/x/sys
	reverse(&devicesToMount)
	var fileSystemsUnmountBacklog []string = []string{}
	for _, filesys := range devicesToMount {
		var retries int
		for {
			// DISCUSS(cavcrosby): would using golang's logging package be beneficial? Its
			// either that, or just using the schmorgesborg of io utilities.
			//
			// Even with --quiet implemented, in some cases like the below, output should
			// still go to where an operator will see it.
			err := syscall.Unmount(filepath.Join(target, filesys), 0x0)
			if err == nil {
				break
			} else if retries == 1 {
				// inspired by:
				// https://stackoverflow.com/questions/35615839/how-to-merge-multiple-strings-and-int-into-a-single-string#answer-35624701
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
}

// Set the current process's root dir to target. A function to exit out
// of the chroot will be returned.
func Chroot(target string) (f func() error, errs []error) {
	// Returning back to the residing directory before entering the chroot.
	// For reference:
	// https://devsidestory.com/exit-from-a-chroot-with-golang/
	returnDir, err := os.Getwd()
	if err != nil {
		return nil, append(errs, err)
	}

	root, err := os.Open("/")
	if err != nil {
		return nil, append(errs, err)
	}

	var devicesToMount []string = []string{"/sys", "/proc", "/dev", "/dev/pts"}
	fileSystemsMounted, err := mountChrootFileSystems(devicesToMount, target)
	defer func() {
		if errs != nil {
			root.Close()
			if err := unMountChrootFileSystems(fileSystemsMounted, target); err != nil {
				errs = append(errs, err)
			}
		}
	}()
	if err != nil {
		return nil, append(errs, err)
	}

	if err := syscall.Chroot(target); err != nil {
		return nil, append(errs, err)
	}

	if err := syscall.Chdir("/"); err != nil { // makes sh happy, otherwise getcwd() for sh fails
		return nil, append(errs, err)
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

		if err := unMountChrootFileSystems(devicesToMount, target); err != nil {
			root.Close()
			return err
		}

		return nil
	}, nil
}

// Create the debootstrap arg list to be used elsewhere.
func createDebootstrapArgList(args *[]string, passThroughFlags *[]string, comprtIncludesPath, codeName, target, mirror string) error {
	var includePkgs []string
	if err := getComprtIncludes(&includePkgs, comprtIncludesPath); err != nil {
		return err
	}

	if includePkgs != nil {
		*args = append(*args, "--include="+strings.Join(includePkgs, ","))
	}
	if passThroughFlags != nil {
		*args = append(*args, *passThroughFlags...)
	}
	*args = append(*args, codeName, target, mirror)

	return nil
}

// Provide an interactive shell into the comprt.
func runInteractiveChroot(target string) (errs []error) {
	var uidRegex *regexp.Regexp = regexp.MustCompile(strconv.Itoa(defaultComprtUid))
	var loginNameIndex, uidIndex int = 0, 2
	defaultComprtUsername, err := locateField(
		filepath.Join(target, "/etc/passwd"),
		regexp.MustCompile(":"),
		uidIndex,
		loginNameIndex,
		uidRegex,
	)
	if err != nil {
		errs = append(errs, err)
		return
	}

	exitChroot, errs := Chroot(target)
	if errs != nil {
		errs = append(errs, errs...)
		return
	}
	defer func() {
		if err := exitChroot(); err != nil {
			errs = append(errs, err)
		}
	}()

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		errs = append(errs, err)
		return
	}

	suPath, err := exec.LookPath("su")
	if err != nil {
		errs = append(errs, err)
		return
	}

	bashCmd := exec.Command(suPath, "--shell", bashPath, "--login", defaultComprtUsername)
	bashCmd.Stdin = os.Stdin
	bashCmd.Stdout = os.Stdout
	bashCmd.Stderr = os.Stderr
	if err := bashCmd.Start(); err != nil {
		errs = append(errs, err)
		return
	}
	if err := bashCmd.Wait(); err != nil {
		errs = append(errs, err)
		return
	}

	return nil
}

// Create a debian comprt.
func createComprt(comprtConfigPath, target, alias, cryptPassword string, quiet bool, debootstrapCmdArr *[]string) (errs []error) {
	debootstrapPath, err := exec.LookPath("debootstrap")
	if err != nil {
		errs = append(errs, err)
		return
	}

	if err := copy(comprtConfigPath, filepath.Join(target, comprtConfigFile)); err != nil {
		errs = append(errs, err)
		return
	}

	// inspired by:
	// https://stackoverflow.com/questions/39173430/how-to-print-the-realtime-output-of-running-child-process-in-go
	debootstrapCmd := exec.Command(debootstrapPath, *debootstrapCmdArr...)
	if !quiet {
		debootstrapCmd.Stdout = os.Stdout
		debootstrapCmd.Stderr = os.Stderr
	}
	if err := debootstrapCmd.Start(); err != nil {
		errs = append(errs, err)
		return
	}
	if err := debootstrapCmd.Wait(); err != nil {
		errs = append(errs, err)
		return
	}

	exitChroot, errs := Chroot(target)
	if errs != nil {
		errs = append(errs, errs...)
		return
	}
	defer func() {
		if err := exitChroot(); err != nil {
			errs = append(errs, err)
		}
	}()

	shPath, err := exec.LookPath("sh")
	if err != nil {
		errs = append(errs, err)
		return
	}

	comprtConfigFileCmd := exec.Command(shPath, filepath.Join("/", comprtConfigFile))
	if !quiet {
		comprtConfigFileCmd.Stdout = os.Stdout
		comprtConfigFileCmd.Stderr = os.Stderr
	}
	if err := comprtConfigFileCmd.Start(); err != nil {
		errs = append(errs, err)
		return
	}
	if err := comprtConfigFileCmd.Wait(); err != nil {
		errs = append(errs, err)
		return
	}

	if alias == noAlias {
		groupAddPath, err := exec.LookPath("groupadd")
		if err != nil {
			errs = append(errs, err)
			return
		}

		groupAddCmd := exec.Command(
			groupAddPath,
			"--gid",
			strconv.Itoa(defaultComprtUid),
			defaultComprtUserName,
		)
		if !quiet {
			groupAddCmd.Stdout = os.Stdout
			groupAddCmd.Stderr = os.Stderr
		}
		if err := groupAddCmd.Start(); err != nil {
			errs = append(errs, err)
			return
		}
		if err := groupAddCmd.Wait(); err != nil {
			errs = append(errs, err)
			return
		}

		userAddPath, err := exec.LookPath("useradd")
		if err != nil {
			errs = append(errs, err)
			return
		}

		// DISCUSS(cavcrosby): it might be fun to reimplement the creation of the default
		// user and group using the more primitive system calls for Unix/Linux. I would
		// like to circle around at some point and look into this.
		userAddCmd := exec.Command(
			userAddPath,
			"--create-home",
			"--home-dir",
			"/home/debcomprt",
			"--uid",
			strconv.Itoa(defaultComprtUid),
			"--gid",
			strconv.Itoa(defaultComprtUid),
			"--shell",
			"/bin/bash",
			defaultComprtUserName,
			"--password",
			cryptPassword,
		)
		if !quiet {
			userAddCmd.Stdout = os.Stdout
			userAddCmd.Stderr = os.Stderr
		}
		if err := userAddCmd.Start(); err != nil {
			errs = append(errs, err)
			return
		}
		if err := userAddCmd.Wait(); err != nil {
			errs = append(errs, err)
			return
		}
	}

	return nil
}

// Start the main program execution.
func main() {
	pconfs := &progConfigs{ // sets defaults
		comprtConfigPath:   filepath.Join(".", comprtConfigFile),
		comprtIncludesPath: filepath.Join(".", comprtIncludeFile),
	}
	pconfs.parseCmdArgs()

	user, err := user.Current()
	if err != nil {
		log.Panic(err)
	}
	if user.Uid != strconv.Itoa(rootUid) {
		log.Panic(strings.Join([]string{progname, ": must be ran as root!"}, ""))
	}

	switch pconfs.command {
	case "chroot":
		// DISCUSS(cavcrosby): chrooting allows for the filesystem to be virtualized in that, the running
		// process will believe it is running in its own private filesystem. I would like
		// to extend this in the future to the process tree as well. That said, some
		// investigation has already been done to look into this.
		//
		// Virtualizing the process tree will require creating processes inside a new
		// PID namespace and mounting a new instance of /proc from a process inside the
		// new PID namespace.
		//
		// While the above would not be technically to hard to implement, it does come
		// with caveats. Looking mainly at the PID namespace man page (link below), any new
		// process created in this PID namespace will be labeled as the 'init' process
		// for the new namespace. Thus, some form of 'init' software would probably need
		// to be run vs just using a shell instance. Otherwise, if the shell instance
		// exited, then all processes in the PID namespace will be killed by the kernel.
		// https://man7.org/linux/man-pages/man7/pid_namespaces.7.html,
		//
		// To add, systemd processes cannot be controlled in a chroot. Thus, more research
		// would need to be done if this feat would be desired to attempt. For reference:
		// https://superuser.com/questions/688733/start-a-systemd-service-inside-chroot-from-a-non-systemd-based-rootfs

		if errs := runInteractiveChroot(pconfs.target); errs != nil {
			log.Panic(errs)
		}
	case "create":
		if err := getProgData(pconfs.alias, pconfs.preprocessAliases, pconfs); err != nil {
			log.Panic(err)
		}

		var debootstrapCmdArr []string
		createDebootstrapArgList(
			&debootstrapCmdArr,
			&pconfs.passThroughFlags,
			pconfs.comprtIncludesPath,
			pconfs.codeName,
			pconfs.target,
			pconfs.mirror,
		)
		if errs := createComprt(
			pconfs.comprtConfigPath,
			pconfs.target,
			pconfs.alias,
			pconfs.cryptPassword,
			pconfs.quiet,
			&debootstrapCmdArr,
		); errs != nil {
			log.Panic(errs)
		}
	}

	os.Exit(0)
}
