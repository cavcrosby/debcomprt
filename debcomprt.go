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

// A type used to store command flag argument values and argument values.
type cmdArgs struct {
	passthrough        bool
	quiet              bool
	helpFlagPassedIn   bool
	alias              string
	comprtIncludesPath string
	comprtConfigPath   string
	codeName           string
	target             string
	mirror             string
	passThroughFlags   []string
}

// A custom callback handler in the event improper cli
// flag/flag arguments/arguments are passed in.
var CustomOnUsageErrorFunc cli.OnUsageErrorFunc = func(context *cli.Context, err error, isSubcommand bool) error {
	cli.ShowAppHelp(context)
	log.Fatal(err)
	return err
}

// Copy the src file to dst. Any existing file will not be overwritten and will not
// copy file attributes.
func copy(src, dst string) error {
	// inspired from:
	// https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file#answer-21061062
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, fs.FileMode(0777))
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// Reverse the string array. Inspired by:
// https://stackoverflow.com/questions/28058278/how-do-i-reverse-a-slice-in-go
func reverse(arr *[]string) {
	for i, j := 0, len(*arr)-1; i < j; i, j = i+1, j-1 {
		(*arr)[i], (*arr)[j] = (*arr)[j], (*arr)[i]
	}
}

// Look to see if the string is in the string array.
func stringInArr(strArg string, arr []string) bool {
	for _, value := range arr {
		if value == strArg {
			return true
		}
	}
	return false
}

// Get required extra data to be used by the program.
func getProgData(args *cmdArgs) {
	progDataDir := appdirs.SiteDataDir(progname, "", "")
	comprtConfigsRepoPath := filepath.Join(progDataDir, comprtConfigsRepoName)

	if args.alias != defaultAlias {
		_, err := os.Stat(progDataDir)
		if errors.Is(err, fs.ErrNotExist) {
			os.MkdirAll(progDataDir, fs.FileMode(0766))
		} else if err != nil {
			log.Panic(err)
		}
		if _, err := os.Stat(comprtConfigsRepoPath); errors.Is(err, fs.ErrNotExist) {
			_, err := git.PlainClone(comprtConfigsRepoPath, false, &git.CloneOptions{
				URL: comprtConfigsRepoUrl,
			})
			if err != nil {
				log.Panic(err)
			}
		}
		args.comprtConfigPath = filepath.Join(comprtConfigsRepoPath, args.alias, comprtConfigFile)
		args.comprtIncludesPath = filepath.Join(comprtConfigsRepoPath, args.alias, args.comprtIncludesPath)
	}
}

// Read in the comprt includes file and adds the discovered packages into
// includePkgs.
func getComprtIncludes(includePkgs *[]string, args *cmdArgs) error {
	// inspired by:
	// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go/16615559#16615559
	file, err := os.Open(args.comprtIncludesPath)
	if err != nil {
		// the comprt includes is optional
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
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
func Chroot(target string) (func(returnDir string, root *os.File) error, error) {
	var fileSystemsToBind []string = []string{"/sys", "/proc", "/dev", "/dev/pts"}
	var fileSystemsUnmountBacklog []string = []string{}
	for _, fs := range fileSystemsToBind {
		if err := syscall.Mount(fs, filepath.Join(target, fs), "", syscall.MS_BIND, ""); err != nil {
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
		for _, fs := range fileSystemsToBind {
			var retries int
			for {
				err := syscall.Unmount(filepath.Join(target, fs), 0x0)
				if err == nil {
					break
				} else if retries == 1 {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " does not want to unmount, will try again later"}, ""))
					fileSystemsUnmountBacklog = append(fileSystemsUnmountBacklog, fs)
				} else if errors.Is(err, syscall.EBUSY) {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " is busy, trying again"}, ""))
					retries += 1
					time.Sleep(1 * time.Second)
				} else if errors.Is(err, syscall.EINVAL) {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " is not a mount point...this may be an issue"}, ""))
					break
				} else {
					fmt.Printf("%s: non-expected error thrown %d", progname, err)
					return err
				}

			}
		}

		// in the rare event that a filesystem is being stubborn to unmount
		for _, fs := range fileSystemsUnmountBacklog {
			var retries int
			for {
				err := syscall.Unmount(filepath.Join(target, fs), 0x0)
				if err == nil {
					break
				} else if retries == 1 {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " does not want to unmount...AGAIN"}, ""))
					fileSystemsUnmountBacklog = append(fileSystemsUnmountBacklog, fs)
				} else if errors.Is(err, syscall.EBUSY) {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " is busy...AGAIN, trying again"}, ""))
					retries += 1
					time.Sleep(2 * time.Second)
				} else if errors.Is(err, syscall.EINVAL) {
					fmt.Println(strings.Join([]string{progname, ": ", fs, " is not a mount point...this may be an issue"}, ""))
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

// Interprets the command arguments passed in. Saving particular flag/flag
// arguments of interest into 'args'.
func parseCmdArgs(args *cmdArgs) {
	var localOsArgs []string = os.Args

	// parses out flags to pass to debootstrap
	for index, value := range localOsArgs {
		if index < 1 {
			continue
		} else if value == "--" {
			break
		} else if stringInArr(value, []string{"-h", "-help", "--help"}) {
			args.helpFlagPassedIn = true
		} else if stringInArr(value, []string{"-passthrough", "--passthrough"}) {
			// index + 1 to ignoring iterating over passthrough flag
			for passthroughIndex, passthroughValue := range localOsArgs[index+1:] {
				if strings.HasPrefix(passthroughValue, "-") {
					args.passThroughFlags = append(args.passThroughFlags, passthroughValue)
				} else {
					// index + 1 to keep passthrough flag
					// index + passthroughIndex + 1 to truncate final flag argument
					localOsArgs = append(localOsArgs[:index+1], localOsArgs[index+passthroughIndex+1:]...)
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
				Destination: &args.alias,
			},
			&cli.BoolFlag{
				Name:        "passthrough",
				Value:       false,
				Usage:       "passes the rest of the flag/flag arguments to debootstrap (e.g. use --foo=bar format)",
				Destination: &args.passthrough,
			},
			&cli.BoolFlag{
				Name:        "quiet",
				Aliases:     []string{"q"},
				Value:       false,
				Usage:       "quiet (no output)",
				Destination: &args.quiet,
			},
			&cli.PathFlag{
				Name:        "includes-path",
				Aliases:     []string{"i"},
				Value:       args.comprtIncludesPath,
				Usage:       "alternative path to comprt includes file",
				Destination: &args.comprtIncludesPath,
			},
			&cli.PathFlag{
				Name:        "config-path",
				Aliases:     []string{"c"},
				Value:       args.comprtConfigPath,
				Usage:       "alternative path to comptr config file",
				Destination: &args.comprtConfigPath,
			},
		},
		Action: func(context *cli.Context) error {
			if context.NArg() < 1 { // CODENAME
				cli.ShowAppHelp(context)
				log.Fatal(errors.New("CODENAME argument is required"))
			}

			if context.NArg() < 2 { // TARGET
				cli.ShowAppHelp(context)
				log.Fatal(errors.New("TARGET argument is required"))
			} else if _, err := os.Stat(context.Args().Get(1)); errors.Is(err, fs.ErrNotExist) {
				log.Fatal(err)
			}

			if context.NArg() < 3 { // MIRROR
				if _, ok := defaultMirrorMappings[context.Args().Get(0)]; !ok {
					log.Fatal(errors.New("no default MIRROR could be determined"))
				}
				args.mirror = defaultMirrorMappings[context.Args().Get(0)]
			} else {
				args.mirror = context.Args().Get(2)
			}

			args.codeName = context.Args().Get(0)
			args.target = context.Args().Get(1)
			return nil
		},
	}
	app.Run(localOsArgs)
	// Because for some reason github.com/urfave/cli/v2@v2.3.0 does not have a way to
	// eject if a variant of 'help' is passed in!
	if args.helpFlagPassedIn {
		os.Exit(0)
	}
}

func main() {
	var includePkgs, debootstrap []string
	var targetComprtConfigPath string = filepath.Join("/", comprtConfigFile)
	args := &cmdArgs{ // sets defaults
		passthrough:        false,
		quiet:              false,
		comprtIncludesPath: filepath.Join(".", comprtIncludeFile),
		comprtConfigPath:   filepath.Join(".", comprtConfigFile),
	}
	parseCmdArgs(args)
	getProgData(args)

	debootstrap = append(debootstrap, "debootstrap")

	if err := getComprtIncludes(&includePkgs, args); err != nil {
		log.Fatal(err)
	}

	if includePkgs != nil {
		debootstrap = append(debootstrap, "--include="+strings.Join(includePkgs, ","))
	}

	if args.passThroughFlags != nil {
		debootstrap = append(debootstrap, args.passThroughFlags...)
	}

	debootstrap = append(debootstrap, args.codeName, args.target, args.mirror)

	debootstrapPath, err := exec.LookPath(debootstrap[0])
	if err != nil {
		log.Fatal(err)
	}

	if err := copy(args.comprtConfigPath, filepath.Join(args.target, targetComprtConfigPath)); err != nil {
		log.Fatal(err)
	}

	// inspired by:
	// https://stackoverflow.com/questions/39173430/how-to-print-the-realtime-output-of-running-child-process-in-go
	debootstrapCmd := exec.Command(debootstrapPath, debootstrap[1:]...)
	debootstrapCmd.Stdout = os.Stdout
	debootstrapCmd.Stderr = os.Stderr
	if err := debootstrapCmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := debootstrapCmd.Wait(); err != nil {
		log.Fatal(err)
	}

	previousDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	root, err := os.Open("/")
	if err != nil {
		log.Fatal(err)
	}
	defer root.Close()

	exitChroot, err := Chroot(args.target)
	if err != nil {
		log.Fatal(err)
	}

	shPath, err := exec.LookPath("sh")
	if err != nil {
		log.Fatal(err)
	}
	comprtConfigFileCmd := exec.Command(shPath, targetComprtConfigPath)
	comprtConfigFileCmd.Stdout = os.Stdout
	comprtConfigFileCmd.Stderr = os.Stderr
	if err := comprtConfigFileCmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := comprtConfigFileCmd.Wait(); err != nil {
		log.Fatal(err)
	}

	if err := exitChroot(previousDir, root); err != nil {
		log.Panic(err)
	}

	os.Exit(0)
}
