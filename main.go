package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jpillora/opts"
	"github.com/pkg/errors"
)

type app struct {
	Mount  string `opts:"help=location of iOS device mount"`
	Target string `opts:"help=target directory to put photos and videos"`
	Delete bool   `opts:"help=delete original files"`
	DryRun bool   `opts:"short=n,help=show operations but dont perform them"`
}

func (a *app) exec(cmd string, args []string) error {
	if a.DryRun {
		fmt.Print(cmd)
		for _, arg := range args {
			fmt.Print(" ", arg)
		}
		fmt.Println()
		return nil
	}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%s", out)
	}
	return nil
}

func (a *app) cpOrMv(src, target string) error {
	if a.Delete {
		return a.exec("mv", []string{"--no-clobber", src, target})
	} else {
		return a.exec("cp", []string{"--no-clobber", src, target})
	}
}

func (a *app) dcim() error {
	root := filepath.Join(a.Mount, "DCIM")
	return filepath.Walk(
		root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.Wrapf(err, "for file %q", path)
			}
			if info.IsDir() {
				if path == root || strings.HasSuffix(path, "APPLE") {
					return nil
				}
				fmt.Fprintf(os.Stderr, "Skipping: %s\n", path)
				return filepath.SkipDir
			}
			target := filepath.Join(a.Target, filepath.Base(path))
			return a.cpOrMv(path, target)
		})
}

func (a *app) mutations() error {
	dcim := filepath.Join(a.Mount, "PhotoData/Mutations/DCIM")
	dir, err := os.Open(dcim)
	if err != nil {
		return errors.WithStack(err)
	}
	names, err := dir.Readdirnames(0)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, name := range names {
		if !strings.HasSuffix(name, "APPLE") {
			continue
		}
		if err := a.mutationsDir(filepath.Join(dcim, name)); err != nil {
			return err
		}
	}
	return nil
}

type mutSrcTarget struct {
	src, target string
}

func (a *app) mutSrcTarget(dir string, ext string) (*mutSrcTarget, error) {
	src := filepath.Join(dir, "Adjustments/FullSizeRender."+ext)
	if _, err := os.Stat(src); err != nil {
		return nil, nil
	}
	name := filepath.Base(dir)
	target := filepath.Join(a.Target, fmt.Sprintf("%s.%s", name, strings.ToUpper(ext)))
	i := 0
	for {
		i++
		if _, err := os.Stat(target); err == nil {
			//TODO: if they're identical, then overwrite it
			newName := fmt.Sprintf("%s-v%d", name, i)
			target = filepath.Join(a.Target, fmt.Sprintf("%s.%s", newName, strings.ToUpper(ext)))
		} else {
			break
		}
	}
	return &mutSrcTarget{
		src:    src,
		target: target,
	}, nil
}

func (a *app) mutationsDir(dcim string) error {
	dir, err := os.Open(dcim)
	if err != nil {
		return errors.WithStack(err)
	}
	names, err := dir.Readdirnames(0)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, name := range names {
		nameDir := filepath.Join(dcim, name)
		srcTarget, err := a.mutSrcTarget(nameDir, "jpg")
		if err != nil {
			return err
		}
		if srcTarget == nil {
			srcTarget, err = a.mutSrcTarget(nameDir, "mov")
			if err != nil {
				return err
			}
		}
		if srcTarget == nil {
			//return errors.Errorf("unexpected mutation entry without any FullSizeRender: %s", nameDir)
			continue
		}
		if err := a.cpOrMv(srcTarget.src, srcTarget.target); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) run() error {
	if a.Mount == "" || a.Target == "" {
		return errors.New("mount and target must be specified")
	}
	if !strings.HasSuffix(a.Target, "/") {
		a.Target = a.Target + "/"
	}
	if err := a.dcim(); err != nil {
		return err
	}
	if err := a.mutations(); err != nil {
		return err
	}
	// delete j0nx?
	return nil
}

func defaultTarget() string {
	return filepath.Join(
		os.Getenv("HOME"), "Dropbox", "Pictures",
		fmt.Sprintf("%s-phone", time.Now().Format("2006-01-02")))
}

func main() {
	var a = app{
		Target: defaultTarget(),
	}
	opts.Parse(&a)
	if err := a.run(); err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
