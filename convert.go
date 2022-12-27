package main

import "C"
import (
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Convert struct {
	Task             Task
	FilesConverted   []FileConverted
	ErrorAllowFormat []string
}

type FileConverted struct {
	Name      string
	FilePath  string
	CoverPath string
	CoverSize image.Point
}

func (c Convert) Run() []FileConverted {
	c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess", "start convert")

	c.FilesConverted = make([]FileConverted, 0)
	c.ErrorAllowFormat = make([]string, 0)

	// default bitrate for convert
	bitrate := 6

	for _, pathway := range c.Task.Files {
		fileConvertPath := pathway

		statFileConvert, err := os.Stat(fileConvertPath)
		if err != nil {
			log.Error(err)
			continue
		}

		if statFileConvert.Size() > 2e+9 {
			bitrate = 3
		}

		if !c.Task.IsAllowFormatForConvert(fileConvertPath) {
			c.ErrorAllowFormat = append(c.ErrorAllowFormat, path.Ext(fileConvertPath))
			continue
		}

		filaName := strings.TrimSuffix(path.Base(fileConvertPath), path.Ext(path.Base(fileConvertPath)))

		// create folder
		folderConvert, errCreat := c.CreateFolderConvert(filaName)
		if errCreat != nil {
			log.Warning(errCreat)
		}

		pathwayNewFiles := folderConvert + "/" + filaName
		fileCoverPath := pathwayNewFiles + ".jpg"
		fileConvertPathOut := pathwayNewFiles + ".mp4"

		timeTotal := c.TimeTotalRaw(fileConvertPath)

		err = c.execConvert(bitrate, timeTotal, filaName, fileConvertPath, fileConvertPathOut)
		if err != nil {
			c.Task.Send(tgbotapi.NewMessage(c.Task.Message.Chat.ID, "❗️ Video is bad - "+filaName))
			log.Error(err)
			continue
		}

		timeTotalAfter := c.TimeTotalRaw(fileConvertPathOut)
		if timeTotal.Second() != timeTotalAfter.Second() {
			mess := fmt.Sprintf("‼️ different time after convert, before %d sec - after %d sec",
				timeTotal.Second(), timeTotalAfter.Second())
			log.Info(mess)
			c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess", mess)
		}

		// create cover
		err = c.CreateCover(fileConvertPathOut, fileCoverPath, timeTotal)
		if err != nil {
			log.Error(err)
		}

		// set permit
		err = os.Chmod(fileConvertPathOut, os.ModePerm)
		if err != nil {
			log.Error(err)
		}
		err = os.Chmod(fileCoverPath, os.ModePerm)
		if err != nil {
			log.Error(err)
		}

		// get size
		sizeCover, err := c.GetSizeCover(fileCoverPath)
		if err != nil {
			log.Error(err)
			continue
		}

		c.FilesConverted = append(c.FilesConverted, FileConverted{filaName,
			fileConvertPathOut, fileCoverPath, sizeCover})
	}

	if len(c.ErrorAllowFormat) > 0 {
		c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess",
			fmt.Sprintf("warning, format not allowed %s", strings.Join(c.ErrorAllowFormat, " | ")))
	}

	return c.FilesConverted
}

func (c Convert) execConvert(rate int, timeTotal time.Time, fileName string, fileConvertPath string,
	fileConvertPathOut string) error {
	// todo checking the h264_nvenc is alive
	cv := "h264_nvenc"
	ffmpegPath := "./ffmpeg"
	if config.IsDev {
		cv = "h264"
		ffmpegPath = "ffmpeg"
	}

	prepareArgs := []string{
		"-protocol_whitelist", "file",
		"-v", "warning", "-hide_banner", "-stats",
		"-i", fileConvertPath,
		"-acodec", "aac",
		"-c:v", cv,
		//"-filter_complex", "scale=w='min(1920\\, iw*3/2):h=-1'",
		"-preset", "medium",
		"-ss", "00:00:00",
		"-t", "00:05:00",
		"-fs", "1990M",
		"-pix_fmt", "yuv420p",
		"-b:v", fmt.Sprintf("%dM", rate),
		"-maxrate", fmt.Sprintf("%dM", rate),
		"-bufsize", fmt.Sprintf("%dM", rate/2),
		// experimental
		//"-bf:v", "0",
		//"-profile:v", "high",
		"-y",
		"-f", "mp4",
		fileConvertPathOut}

	var args []string
	for _, pa := range prepareArgs {
		if c.Task.UserFromDB.Premium == 1 && (strings.Contains(pa, "-ss") ||
			strings.Contains(pa, "00:00:00") ||
			strings.Contains(pa, "-t") ||
			strings.Contains(pa, "00:05:00")) {
			continue
		}
		//if config.IsDev && strings.Contains(pa, "00:05:00") {
		//	if config.IsDev {
		//		args = append(args, "00:01:00")
		//	}
		//	continue
		//}
		args = append(args, pa)
	}

	cmd := exec.Command(ffmpegPath, args...)

	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}

	tmpLast := ""
	for {
		tmp := make([]byte, 1024)
		_, err := stdout.Read(tmp)
		if err != nil {
			break
		}
		tmpLast = string(tmp)

		regx := regexp.MustCompile(`time=(.*?) `)
		matches := regx.FindStringSubmatch(string(tmp))

		var timeLeft time.Time
		if len(matches) == 2 {
			timeLeft, _ = time.Parse("15:04:05,00", strings.Trim(matches[1], " "))
		} else {
			break
		}

		timeNull, _ := time.Parse("15:04:05", "00:00:00")

		PercentConvert, _ := strconv.ParseFloat(fmt.Sprintf("%.2f",
			100-(timeTotal.Sub(timeLeft).Seconds()/timeTotal.Sub(timeNull).Seconds())*100), 64)

		_, errEdit := c.Task.App.Bot.Send(tgbotapi.NewEditMessageText(c.Task.Message.Chat.ID, c.Task.MessageEditID,
			fmt.Sprintf("🌪 %s \n\n🔥 Convert progress: %.2f%%", fileName, PercentConvert)))

		if errEdit != nil {
			log.Warning(errEdit)
		}

		time.Sleep(2 * time.Second)
	}

	if err := cmd.Wait(); err != nil {
		log.Error(tmpLast)
		return err
	}

	return nil
}

func (c Convert) CreateFolderConvert(fileName string) (string, error) {
	folderConvert := config.DirBot + "/storage/" + c.Task.UniqueId("files-convert-"+
		fileName+"-"+strconv.FormatInt(c.Task.Message.From.ID, 10))
	err := os.Mkdir(folderConvert, os.ModePerm)
	if err != nil {
		return "", err
	}

	return folderConvert, nil
}

func (c Convert) CheckExistVideo() bool {
	existVideo := false
	for _, pathway := range c.Task.Files {
		if c.Task.IsAllowFormatForConvert(pathway) {
			existVideo = true
		}
	}

	return existVideo
}

func (c Convert) CreateCover(videoFile string, fileCoverPath string, timeTotal time.Time) error {
	timeCut := "00:00:30"
	if timeTotal.Second() <= 40 {
		timeCut = "00:00:01"
	}

	_, err := exec.Command("ffmpeg",
		"-protocol_whitelist", "file",
		"-i", videoFile,
		"-ss", timeCut,
		"-vframes", "1",
		"-y",
		fileCoverPath).Output()

	os.Chmod(fileCoverPath, os.ModePerm)

	return err
}

func (c Convert) GetSizeCover(fileCoverPath string) (image.Point, error) {
	existingImageFile, err := os.Open(fileCoverPath)
	if err != nil {
		return image.Point{X: 0, Y: 0}, err
	}
	defer existingImageFile.Close()

	_, _, err = image.Decode(existingImageFile)
	if err != nil {
		return image.Point{X: 0, Y: 0}, err
	}
	_, err = existingImageFile.Seek(0, 0)
	if err != nil {
		return image.Point{}, err
	}

	loadedImage, err := jpeg.Decode(existingImageFile)
	if err != nil {
		return image.Point{X: 0, Y: 0}, err
	}

	return loadedImage.Bounds().Size(), nil
}

func (c Convert) TimeTotalRaw(pathway string) time.Time {
	timeTotalRaw, err := exec.Command("ffprobe",
		"-protocol_whitelist", "file",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-sexagesimal",
		pathway).Output()
	if err != nil {
		log.Error(err)
	}

	parse, err := time.Parse("15:04:05,000000", strings.Trim(string(timeTotalRaw), "\n"))
	if err != nil {
		log.Error(err)
	}
	return parse
}
