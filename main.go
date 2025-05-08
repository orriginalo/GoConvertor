package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

var audioExtentions = []string{".mp3", ".wav", ".flac", ".ogg", ".aac", ".m4a", ".wma", ".aiff", ".au", ".opus"}
var imageExtentions = []string{
	".jpg", ".jpeg", ".png", ".webp", ".ico", ".gif", ".bmp", ".tiff", ".tif", ".svg", ".heic", ".heif", ".avif",

	".jfif", ".apng", ".psd", ".exr", ".tga", ".pdf", ".eps", ".djvu",

	".raw", ".cr2", ".nef", ".arw", ".dng", ".rw2", ".orf", ".sr2",

	".pbm", ".pgm", ".ppm", ".pnm", ".xpm", ".xbm", ".NEF",
}

var (
	hasAudio, hasImage, hasFolders       bool
	dirPath                              string
	targetAudioFormat, targetImageFormat string
	convertRecursively                   bool
	filesToConvertChan                   chan FileToConvert
	filesToConvert                       []FileToConvert
	totalConverterFiles                  int
	maxWorkers                           int
)

type FileToConvert struct {
	Path      string
	IsAudio   bool
	ConvertTo string
}

func init() {
	filesToConvertChan = make(chan FileToConvert)
	maxWorkers = runtime.NumCPU()
}

func main() {
	survey.AskOne(&survey.Input{
		Message: "Укажите путь к директории с файлами:",
		Default: ".",
	}, &dirPath)

	if stat, err := os.Stat(dirPath); err != nil || !stat.IsDir() {
		fmt.Println("Указанный путь не является директорией или не существует.")
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Println("Ошибка при чтении директории:", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			hasFolders = true
			continue
		}

		ext := filepath.Ext(entry.Name())
		for _, audioExt := range audioExtentions {
			if ext == audioExt {
				hasAudio = true
				filesToConvert = append(filesToConvert, FileToConvert{Path: filepath.Join(dirPath, entry.Name()), IsAudio: true})
			}
		}
		for _, imageExt := range imageExtentions {
			if ext == imageExt {
				hasImage = true
				filesToConvert = append(filesToConvert, FileToConvert{Path: filepath.Join(dirPath, entry.Name()), IsAudio: false})
			}
		}
	}

	if !hasAudio && !hasImage {
		fmt.Println("Не найдено ни одного файла с аудио или изображениями.")
		return
	}

	if hasAudio {
		survey.AskOne(&survey.Select{
			Message: "В какой формат конвертировать аудио?",
			Options: audioExtentions,
		}, &targetAudioFormat)
	}
	if hasImage {
		survey.AskOne(&survey.Select{
			Message: "В какой формат конвертировать изображения?",
			Options: imageExtentions,
		}, &targetImageFormat)
	}

	for i := range filesToConvert {
		if filesToConvert[i].IsAudio {
			filesToConvert[i].ConvertTo = targetAudioFormat
		} else {
			filesToConvert[i].ConvertTo = targetImageFormat
		}
	}

	if hasFolders {
		survey.AskOne(&survey.Confirm{
			Message: "Конвертировать рекурсивно?",
		}, &convertRecursively)
	}

	fmt.Println()
	fmt.Println("Конвертация началась...")

	var wg sync.WaitGroup

	start := time.Now()
	ProcessFiles(filesToConvertChan, &wg)

	for _, f := range filesToConvert {
		if filepath.Ext(f.Path) != f.ConvertTo {
			filesToConvertChan <- f
		}
	}

	close(filesToConvertChan)

	wg.Wait()

	end := time.Now()
	elapsed := end.Sub(start).Seconds()
	fmt.Println()
	fmt.Println("Конвертация файлов завершена")
	fmt.Printf("Всего конвертировано %d файлов за %.1f секунд\n", totalConverterFiles, elapsed)
}

func ProcessFiles(filesToConvert chan FileToConvert, wg *sync.WaitGroup) {
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range filesToConvertChan {
				ConvertFile(f.Path, f.ConvertTo, f.IsAudio, i)
			}
		}()
	}
}

func ConvertFile(path string, format string, isAudio bool, workerNum int) {
	var cmd *exec.Cmd

	switch isAudio {
	case true:
		outputPath := fmt.Sprintf("%s%s", path[:len(path)-len(filepath.Ext(path))], format)
		cmd = exec.Command("ffmpeg", "-i", path, outputPath)

	case false:
		outputPath := fmt.Sprintf("%s%s", path[:len(path)-len(filepath.Ext(path))], format)
		cmd = exec.Command("magick", path, outputPath)
	}

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Ошибка при конвертации файла '%s': %v\n", path, err)
	} else {
		fmt.Printf("Конвертация файла '%s' завершена успешно.\n", path)
		totalConverterFiles++
	}
}
