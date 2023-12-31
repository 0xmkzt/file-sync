package main

import (
	"flag"
	"fmt"
	"go.uber.org/zap"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	//
	"file-sync/utils/log"
)

var (
	version = "1.0.0"

	sourceDir      = "/path/source_dir"
	targetDir      = "/path/target_dir"
	fileKeyPats    = make([]string, 10)
	defaultLogName = "run.log"
	logName        = defaultLogName

	baseFileName = "application.log"

	syncMap = sync.Map{}

	copyExpireTime   = time.Hour
	deleteExpireTime = time.Hour * 3

	logger *zap.Logger
)

func walkSyncMapForClear(key, value interface{}) bool {
	syncMap.Delete(key)

	return true
}

func walkSyncMapForPrint(key, value interface{}) bool {
	logger.Info(fmt.Sprintf("Load target file key, %v:%v", key, value))
	return true
}

func checkFileKey(fileKey string) bool {
	for _, pat := range fileKeyPats {
		if strings.HasPrefix(fileKey, pat) {
			return true
		}
	}
	return false
}

func copyFile(sourcePath, targetPath string) bool {
	logger.Info(fmt.Sprintf("Copy file, %v -> %v", sourcePath, targetPath))

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		logger.Error(fmt.Sprint("Open source file failed, ", err))
		return false
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		logger.Error(fmt.Sprint("Create target file failed, ", err))
		return false
	}
	defer targetFile.Close()

	targetFileStat, err := targetFile.Stat()
	if err != nil {
		logger.Error(fmt.Sprint("Create target file failed, ", err))
		return false
	}

	offset := targetFileStat.Size()
	_, err = sourceFile.Seek(offset, io.SeekStart)
	if err != nil {
		logger.Error(fmt.Sprintf("Seek(%v) source file failed, %v", offset, err))
		return false
	}

	n, err := io.Copy(targetFile, sourceFile)
	if err != nil {
		logger.Error(fmt.Sprint("Copy file error, ", err))
		return false
	}
	logger.Info(fmt.Sprintf("Copy stats, offset: %v, n: %v", offset, n))

	return true
}

func getTargetFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return nil
	}

	if !info.IsDir() {
		fileName := filepath.Base(path)
		if strings.HasSuffix(fileName, baseFileName) {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				syncMap.Store(fileName, stat.Size)
			}
		}
	}
	return nil
}

func copySourceFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if !info.IsDir() {
		fileName := filepath.Base(path)
		parentDirName := filepath.Base(filepath.Dir(path))
		stat, ok := info.Sys().(*syscall.Stat_t)
		if ok {
			fileSize := stat.Size
			fileKey := fmt.Sprintf("%v@%v@%v", parentDirName, stat.Ino, baseFileName)
			if !checkFileKey(fileKey) {
				return nil
			}

			_msg := fmt.Sprintf("source file(%v:%v)", fileKey, fileSize)

			toCopy := false
			if _fileSize, ok := syncMap.Load(fileKey); ok {
				_fileSize, _ := _fileSize.(int64)
				_msg = fmt.Sprintf("Exists target file(%v:%v), %v", fileKey, _fileSize, _msg)
				if fileSize > _fileSize {
					toCopy = true
					logger.Info(fmt.Sprintf("%v, size changed", _msg))
				} else if fileSize < _fileSize {
					logger.Info(fmt.Sprintf("%v, size anormal", _msg))
				}
			} else if fileName == baseFileName {
				modTime := info.ModTime()
				if modTime.Before(time.Now().Add(-copyExpireTime)) {
					return nil
				}

				toCopy = true
				logger.Info(fmt.Sprint("New ", _msg))
			}

			if toCopy && copyFile(path, filepath.Join(targetDir, fileKey)) {
				syncMap.Store(fileKey, fileSize)
			}

		} else {
			logger.Warn("Get source file failed")
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
		if strings.HasSuffix(fileName, baseFileName) {
			if err := os.Remove(path); err != nil {
				return err
			}
			syncMap.Delete(fileName)

			logger.Info(fmt.Sprintf("Delete file(%v): %v, %v(%v)", fileName, path, modTime, deleteExpireTime))
		}
	}

	return nil
}

func run() {
	logger.Info("Run sync...")

	syncMap.Range(walkSyncMapForClear)
	// Get target file
	if err := filepath.Walk(targetDir, getTargetFile); err != nil {
		logger.Error(fmt.Sprintf("Get target file failed, %v", err.Error()))
		return
	}
	syncMap.Range(walkSyncMapForPrint)

	// Copy source file
	if err := filepath.Walk(sourceDir, copySourceFile); err != nil {
		logger.Error(fmt.Sprintf("Copy source file failed, %v", err.Error()))
		return
	}

	// Delete target file
	if err := filepath.Walk(targetDir, deleteLocalFile); err != nil {
		logger.Error(fmt.Sprintf("Delete target file failed, %v", err.Error()))
		return
	}

	logger.Info("Run end")
}

func parseArgs() {
	var fileKeyPats_ string

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "[Version: %v] Usage of %s:\n\n", version, os.Args[0])
		flag.PrintDefaults()
	}

	flag.StringVar(&sourceDir, "source_dir", "", "")
	flag.StringVar(&targetDir, "target_dir", "", "")
	flag.StringVar(&fileKeyPats_, "file_key_pats", "", "")
	flag.StringVar(&logName, "log_name", "", "")
	flag.Parse()

	for _, path := range [2]string{sourceDir, targetDir} {
		if path == "" {
			fmt.Println("Please set valid dir")
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

	if logName == "" {
		logName = defaultLogName
	}

	fileKeyPats = strings.Split(fileKeyPats_, ",")

	fmt.Printf("================================= Parse args(Version:%v) %v =================================\n",
		version, time.Now())
	fmt.Println("source_dir =", sourceDir)
	fmt.Println("target_dir =", targetDir)
	fmt.Printf("file_key_pats = %v\n", fileKeyPats)
	fmt.Printf("log_name = %v\n", logName)
}

func main() {
	parseArgs()

	fmt.Println("File-Sync start...")

	logger = log.Init(logName)
	shutdown := false
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalChan
		fmt.Println("Shutdown by", sig)
		shutdown = true
	}()

	logger.Info("File-Sync start...")
	for {
		if shutdown {
			logger.Info("Shutdown ...")
			break
		}

		run()

		time.Sleep(time.Second * 1)
	}

	logger.Info("File-Sync end")
}
