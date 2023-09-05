package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	//
	"file-sync/utils/log"
)

var (
	localPath  = "/path/local"
	targetPath = "/path/target"

	//
	targetFileName = "application.log"

	refreshKey = "_refresh_key"

	logger = log.GetLogger()

	syncMap = sync.Map{}

	copyExpireTime   = time.Hour * 1
	deleteExpireTime = time.Hour * 3
)

func walkSyncMapForDelete(key, value interface{}) bool {
	syncMap.Delete(key)

	logger.Info(fmt.Sprintf("Delete key from map, %v:%v", key, value))

	return true
}

func walkSyncMapForPrint(key, value interface{}) bool {
	logger.Info(fmt.Sprintf("Get file from local, %v,%v", key, value))

	return true
}

func copyFile(srcPath, dstPath string) bool {
	logger.Info(fmt.Sprintf("Copy file, %v -> %v", srcPath, dstPath))

	srcFile, err := os.Open(srcPath)
	if err != nil {
		logger.Info(fmt.Sprint("Open src file failed, ", err))
		return false
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		logger.Info(fmt.Sprint("Create dst file failed, ", err))
		return false
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		logger.Info(fmt.Sprint("Copy file failed, ", err))
		return false
	}

	return true
}

func getLocalFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return nil
	}

	if !info.IsDir() {
		fileName := filepath.Base(path)
		if strings.HasSuffix(fileName, targetFileName) {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				syncMap.Store(fileName, stat.Size)
			}
		}
	}

	return nil
}

func copyTargetFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if !info.IsDir() {
		fileName := filepath.Base(path)
		dirName := filepath.Base(filepath.Dir(path))
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok {
			fileSize := stat.Size
			newFileName := fmt.Sprintf("%v@%v@%v", dirName, stat.Ino, fileName)
			newPath := filepath.Join(localPath, newFileName)

			_msg := fmt.Sprintf("target file(%v,%v): %v", newFileName, fileSize, path)

			modTime := info.ModTime()
			if modTime.Before(time.Now().Add(-copyExpireTime)) {
				logger.Debug(fmt.Sprintf("Expired %v, %v(%v)", _msg, modTime, copyExpireTime))
				return nil
			}

			toCopy := false
			if _fileSize, ok := syncMap.Load(newFileName); ok {
				_msg = fmt.Sprint("Exists ", _msg)
				if _fileSize != fileSize {
					toCopy = true
					_msg = fmt.Sprintf("%v, File size changed", _msg)
				} else {
					_msg = fmt.Sprintf("%v, File size not change", _msg)
				}
				logger.Info(_msg)

			} else if fileName == targetFileName {
				toCopy = true
				logger.Info(fmt.Sprint("New ", _msg))

			} else {
				logger.Debug(fmt.Sprint("Ignore ", _msg))

			}

			if toCopy && copyFile(path, newPath) {
				syncMap.Store(newFileName, fileSize)
			}

		} else {
			logger.Info("Get target file failed")
		}
	}

	return nil
}

func deleteLocalFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	modTime := info.ModTime()
	if !modTime.Before(time.Now().Add(-deleteExpireTime)) {
		return nil
	}
	if !info.IsDir() {
		fileName := filepath.Base(path)
		if strings.HasSuffix(fileName, targetFileName) {
			if err := os.Remove(path); err != nil {
				return err
			}
			syncMap.Delete(fileName)

			logger.Info(fmt.Sprintf("Delete file(%v): %v, %v(%v)", fileName, path, modTime, deleteExpireTime))
		}
	}

	return nil
}

func toRefreshLocalFile() bool {
	newValue := time.Now().Second()
	refresh := false
	_, ok := syncMap.Load(refreshKey)
	if !ok {
		refresh = true
	} else if newValue%5 == 0 {
		refresh = true
	}

	if refresh {
		syncMap.Range(walkSyncMapForDelete)
		syncMap.Store(refreshKey, newValue)
	}

	return refresh
}

func run() {
	logger.Info("Run sync...")

	if toRefreshLocalFile() {
		// Get local file
		if err := filepath.Walk(localPath, getLocalFile); err != nil {
			logger.Error(fmt.Sprintf("Get local file failed, %v", err.Error()))
			return
		}
		syncMap.Range(walkSyncMapForPrint)
	}

	// Copy target file
	if err := filepath.Walk(targetPath, copyTargetFile); err != nil {
		logger.Error(fmt.Sprintf("Copy target file failed, %v", err.Error()))
		return
	}

	// Delete local file
	if err := filepath.Walk(localPath, deleteLocalFile); err != nil {
		logger.Error(fmt.Sprintf("Delete local file failed, %v", err.Error()))
		return
	}

	logger.Info("Run end")
}

func parseArgs() {

	flag.StringVar(&localPath, "localPath", "", "")
	flag.StringVar(&targetPath, "targetPath", "", "")
	flag.Parse()

	for _, path := range [2]string{localPath, targetPath} {
		if path == "" {
			fmt.Printf("Please set valid path\n\n")
			flag.PrintDefaults()
		} else if _, err := os.Stat(path); err == nil {
			continue
		} else if os.IsNotExist(err) {
			fmt.Printf("Path not exist, %v\n", path)
		} else {
			fmt.Println("Parse args failed. \n", err)
		}
		os.Exit(1)
	}
}

func main() {
	parseArgs()

	fmt.Println("localPath =", localPath)
	fmt.Println("targetPath =", targetPath)
	fmt.Println("Start ...")

	for {
		run()
		time.Sleep(time.Second * 1)
	}
}
