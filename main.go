package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/viper"
)

var audioExtentions = []string{".mp3", ".wav", ".flac", ".ogg", ".aac", ".m4a", ".wma", ".aiff", ".au", ".opus"}
var imageExtentions = []string{
	".jpg", ".jpeg", ".png", ".webp", ".ico", ".gif", ".bmp", ".tiff", ".tif", ".svg", ".heic", ".heif", ".avif",

	".jfif", ".apng", ".psd", ".exr", ".tga", ".pdf", ".eps", ".djvu",

	".raw", ".cr2", ".nef", ".arw", ".dng", ".rw2", ".orf", ".sr2",

	".pbm", ".pgm", ".ppm", ".pnm", ".xpm", ".xbm", ".nef",
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
	configPath                           string
)

type FileToConvert struct {
	Path      string
	IsAudio   bool
	ConvertTo string
}

func init() {
	filesToConvertChan = make(chan FileToConvert)
	maxWorkers = runtime.NumCPU()
	switch runtime.GOOS {
	case "windows":
		configPath = filepath.Join(os.Getenv("APPDATA"), "GoConvertor")
	case "linux":
		homeDir, _ := os.UserHomeDir()
		configPath = filepath.Join(homeDir, ".config", "goconvertor")
	}

	LoadConfig(configPath)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--config" {
		OpenConfigMenu()
		return
	}

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

		ext := strings.ToLower(filepath.Ext(entry.Name()))
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
	var outputPath string

	switch isAudio {
	case true:
		outputPath = fmt.Sprintf("%s%s", path[:len(path)-len(filepath.Ext(path))], format)
		if viper.GetBool("save_converted_files_into_folder") {
			outputPath = filepath.Join(filepath.Dir(outputPath), "converted", filepath.Base(outputPath))
		}
		cmd = exec.Command("ffmpeg", "-i", path, outputPath)

	case false:
		outputPath = fmt.Sprintf("%s%s", path[:len(path)-len(filepath.Ext(path))], format)
		if viper.GetBool("save_converted_files_into_folder") {
			outputPath = filepath.Join(filepath.Dir(outputPath), "converted", filepath.Base(outputPath))
		}
		cmd = exec.Command("magick", path, outputPath)
	}
	if viper.GetBool("save_converted_files_into_folder") {
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
			fmt.Printf("Не удалось создать папку '%s': %v\n", outputDir, err)
			return
		}
	}
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Ошибка при конвертации файла '%s': %v\n", path, err)
	} else {
		fmt.Printf("Конвертация файла '%s' завершена успешно.\n", path) // TODO: Добавить удаление исходных файлов
		if viper.GetBool("delete_source_files") {
			os.Remove(path)
		}
		totalConverterFiles++
	}
}

func OpenConfigMenu() {
	options := []string{
		"Удалять исходные файлы после конвертации",
		"Сохранять конвертированные файлы в отдельную папку",
	}

	var selected []string
	var defaults []string

	if viper.GetBool("delete_source_files") {
		defaults = append(defaults, "Удалять исходные файлы после конвертации")
	}
	if viper.GetBool("save_converted_files_into_folder") {
		defaults = append(defaults, "Сохранять конвертированные файлы в отдельную папку")
	}

	prompt := &survey.MultiSelect{
		Message: "Выберите нужные опции:",
		Options: options,
		Default: defaults,
	}
	survey.AskOne(prompt, &selected)

	viper.Set("delete_source_files", slices.Contains(selected, "Удалять исходные файлы после конвертации"))
	viper.Set("save_converted_files_into_folder", slices.Contains(selected, "Сохранять конвертированные файлы в отдельную папку"))

	err := viper.WriteConfig()
	if err != nil {
		fmt.Println("Ошибка при записи конфигурационного файла:", err)
	}
}

func LoadConfig(path string) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(path)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("Конфигурационный файл не найден. Создаю новый...")
			viper.Set("delete_source_files", false)
			viper.Set("save_converted_files_into_folder", false)

			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				fmt.Println("Ошибка при создании папки для конфигурационного файла:", err)
				return
			}

			viper.WriteConfigAs(filepath.Join(path, "config.yaml"))
			return
		} else {
			fmt.Println("Ошибка при чтении конфигурационного файла:", err)
		}
	}
}
