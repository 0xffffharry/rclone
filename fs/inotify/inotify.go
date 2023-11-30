package inotify

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/fs/sync"
	"github.com/rclone/rclone/fs/walk"
)

func Inotify(ctx context.Context, fdst, fsrc fs.Fs, srcFileName string, createEmptySrcDirs bool) error {
	if srcFileName == "" {
		features := fsrc.Features()
		if !features.IsLocal {
			return fmt.Errorf("source backend only support local backend")
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if srcFileName != "" {
		err = watcher.Add(srcFileName)
		if err != nil {
			return err
		}
	} else {
		tree, err := walk.NewDirTree(ctx, fsrc, "", false, -1)
		if err != nil {
			return err
		}
		rootDir := fsrc.Root()
		for _, dir := range tree.Dirs() {
			dir = filepath.Join(rootDir, dir)
			err = watcher.Add(dir)
			if err != nil {
				return err
			}
		}
	}
	go func() {
		signalChan := make(chan os.Signal)
		defer close(signalChan)
		signal.Notify(signalChan, syscall.SIGQUIT, syscall.SIGSTOP, syscall.SIGKILL, os.Interrupt, os.Kill)
		select {
		case <-ctx.Done():
		case <-signalChan:
			cancel()
		}
	}()
	callChan := make(chan struct{}, 1)
	callChanCloseFlag := &atomic.Int64{}
	callChanCloseFlag.Add(1)
	defer func() {
		if callChanCloseFlag.Add(-1) == 0 {
			close(callChan)
		}
	}()
	go func() {
		callChanCloseFlag.Add(1)
		defer func() {
			if callChanCloseFlag.Add(-1) == 0 {
				close(callChan)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-watcher.Events:
				fs.Debugf(nil, "events: name: %s, operate: %s", event.Name, event.Op.String())
				switch {
				case event.Op.Has(fsnotify.Create):
					fsInfo, configName, fsPath, config, err := fs.ConfigFs(event.Name)
					if err != nil {
						fs.Errorf(nil, "failed to read path: %s", err)
						continue
					}
					stat, _ := os.Stat(fsPath)
					if err == nil && stat.IsDir() {
						f, err := fsInfo.NewFs(ctx, configName, fsPath, config)
						if err != nil {
							fs.Errorf(nil, "failed to load path: %s", err)
							continue
						}
						tree, err := walk.NewDirTree(ctx, f, "", false, -1)
						if err != nil {
							fs.Errorf(nil, "failed to load path(build tree): %s", err)
							continue
						}
						for _, dir := range tree.Dirs() {
							dir = filepath.Join(fsPath, dir)
							err = watcher.Add(dir)
							if err != nil {
								fs.Errorf(nil, "failed to load path(add to watcher): %s", event.Name)
							}
						}
					} else {
						err = watcher.Add(fsPath)
						if err != nil {
							fs.Errorf(nil, "failed to add file: %s", event.Name)
						}
					}
				case event.Op.Has(fsnotify.Write):
				case event.Op.Has(fsnotify.Remove):
					fallthrough
				case event.Op.Has(fsnotify.Rename):
					watcher.Remove(event.Name)
				}
				select {
				case callChan <- struct{}{}:
				default:
				}
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
		case <-callChan:
			fs.Logf(nil, "%s", "syncing...")
			err := syncDo(ctx, fdst, fsrc, srcFileName, createEmptySrcDirs)
			if err != nil {
				fs.Errorf(nil, "failed to sync: %s", err)
			}
			fs.Logf(nil, "%s", "sync done")
			continue
		}
		break
	}
	return nil
}

func syncDo(ctx context.Context, fdst, fsrc fs.Fs, srcFileName string, createEmptySrcDirs bool) error {
	if srcFileName == "" {
		return sync.Sync(ctx, fdst, fsrc, createEmptySrcDirs)
	}
	return operations.CopyFile(ctx, fdst, fsrc, srcFileName, srcFileName)
}
