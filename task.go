package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (t *Task) DownloadTorrentFiles() bool {
	file, _ := t.App.Bot.GetFile(tgbotapi.FileConfig{FileID: t.Message.Document.FileID})

	t.App.SendLogToChannel("doc", fmt.Sprintf("@%s (%d) - upload torrent file | ActiveRange: %d",
		t.Message.From.UserName, t.Message.From.ID, t.App.ActiveRange+1),
		t.Message.Document.FileID)

	var err error

	//t.TorrentProcess, _ := client.AddMagnet("magnet:?xt=urn:btih:....")
	t.TorrentProcess, err = t.App.TorClient.AddTorrentFromFile(config.TgPathLocal + "/" + config.BotToken +
		"/" + file.FilePath)
	if err != nil {
		log.Error(err)
	}

	t.TorrentProcess.SetDisplayName(UniqueId("temp-torrent-name"))
	t.TorrentProcess.SetMaxEstablishedConns(200)
	<-t.TorrentProcess.GotInfo()

	infoText := fmt.Sprintf("📺 Torrent: %s", t.TorrentProcess.Info().BestName())

	t.TorrentProcess.Info()

	if len(t.TorrentProcess.Info().Files) != 0 {
		var listFiles string
		for index, val := range t.TorrentProcess.Info().Files {
			if len(val.Path) == 0 {
				continue
			}
			if t.IsAllowFormat(val.Path[0]) {
				listFiles += fmt.Sprintf("%d. %s\n", index+1, val.Path[0])
			}
		}
		if listFiles != "" {
			infoText += fmt.Sprintf("\n\n📋 List of files:\n") + listFiles
		}
	}

	messInfo, _ := t.App.Bot.Send(tgbotapi.NewMessage(t.Message.Chat.ID, infoText))
	// pin
	pinChatInfoMess := tgbotapi.PinChatMessageConfig{
		ChatID:              messInfo.Chat.ID,
		MessageID:           messInfo.MessageID,
		DisableNotification: true,
	}
	if _, err := t.App.Bot.Request(pinChatInfoMess); err != nil {
		log.Error(err)
	}

	// edit message
	messStat, _ := t.App.Bot.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "🍀 Download is starting soon..."))
	t.MessageEdit = messStat.MessageID

	if t.UserFromDB.Premium == 0 {
		t.App.Bot.Send(tgbotapi.NewSticker(t.Message.Chat.ID,
			tgbotapi.FileID("CAACAgIAAxkBAAIEW2OcfHb7yPa6z59rHlFiTTUTkA3XAAJ-GQACHiDBS43V6msCr8MXKwQ")))
		messPremium := tgbotapi.NewMessage(t.Message.Chat.ID,
			`‼️ You don't have a donation for us, only the first 5 minutes video is available and zip don't available too

		<a href="url-donate">Help us, subscribe and service will be more fantastical</a> 🔥

		(Write your telegram username in the body message. After donation, you will access 30 days)`)
		messPremium.ParseMode = tgbotapi.ModeHTML
		t.App.Bot.Send(messPremium)
	}

	activeRangeUser := config.ActiveRangeUser
	if t.UserFromDB.Forward == 1 {
		activeRangeUser = 3
	}

	// user queue
	for {
		if t.App.RangeUser[t.Message.From.ID] < activeRangeUser {
			t.App.RangeUserMu.Lock()
			t.App.RangeUser[t.Message.From.ID]++
			t.App.RangeUserMu.Unlock()

			break
		}
		time.Sleep(time.Second)
	}

	// global queue
	for {
		if t.App.ActiveRange < config.ActiveRange {
			t.App.ActiveRangeMu.Lock()
			t.App.ActiveRange++
			t.App.ActiveRangeMu.Unlock()

			break
		}
		time.Sleep(time.Second)
	}

	time.Sleep(time.Second)

	// log
	t.App.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - start download",
		t.Message.From.UserName, t.Message.From.ID))

	go func() {
		for {
			stat, percent := t.statDl()
			if percent == 100 {
				break
			}

			t.App.Bot.Send(tgbotapi.NewEditMessageText(messStat.Chat.ID, messStat.MessageID, stat))
			time.Sleep(2 * time.Second)
		}
	}()

	t.TorrentProcess.DownloadAll()

	<-t.TorrentProcess.Complete.On()
	t.TorrentProcess.Drop()

	t.App.Bot.Send(tgbotapi.NewEditMessageText(messStat.Chat.ID, t.MessageEdit, "✅ Torrent downloaded"))

	t.Files = t.TorrentProcess.Files()

	if t.Message.Caption == "local" {
		for _, val := range t.Files {
			os.Rename(config.DirBot+"/torrent-client/"+val.Path(), "/temp-local/"+val.Path())
		}
		return false
	}

	return true
}

func (t *Task) IsAllowFormat(pathWay string) bool {
	for _, ext := range config.AllowVideoFormats {
		if ext == path.Ext(pathWay) {
			return true
		}
	}

	return false
}

func (t *Task) statDl() (string, float64) {
	if t.TorrentProcess.Info() == nil {
		return "", 0
	}

	currentProgress := t.TorrentProcess.BytesCompleted()
	downloadSpeed := humanize.Bytes(uint64(currentProgress-t.TorrentProgress)) + "/s"
	t.TorrentProgress = currentProgress

	complete := humanize.Bytes(uint64(currentProgress))
	size := humanize.Bytes(uint64(t.TorrentProcess.Info().TotalLength()))

	bytesWrittenData := t.TorrentProcess.Stats().BytesWrittenData
	uploadProgress := (&bytesWrittenData).Int64() - t.TorrentUploaded
	uploadSpeed := humanize.Bytes(uint64(uploadProgress)) + "/s"
	t.TorrentUploaded = uploadProgress

	ctlInfo := t.TorrentProcess.Info()
	var percentage float64
	if ctlInfo != nil {
		percentage = float64(t.TorrentProcess.BytesCompleted()) / float64(ctlInfo.TotalLength()) * 100
	}

	stat := fmt.Sprintf("🔥 Progress: \t%s / %s  %.2f%%\n\n🔽 Download speed: %s",
		complete, size, percentage, downloadSpeed)

	// todo
	if uploadProgress > 0 {
		stat += fmt.Sprintf("\n\n🔼 Upload speed: \t%s", uploadSpeed)
	}

	return stat, percentage
}

func (t *Task) CheckExistVideo() bool {
	existVideo := false
	for _, val := range t.Files {
		if t.IsAllowFormat(val.Path()) {
			existVideo = true
		}
	}

	return existVideo
}

func (t *Task) SendVideos() {
	sendErrorAllowFormat := make([]string, 0)
	for _, val := range t.Files {
		_, err := os.Stat(config.DirBot + "/torrent-client/" + val.Path())
		if err != nil {
			log.Error(err)
			continue
		}

		if !t.IsAllowFormat(val.Path()) {
			sendErrorAllowFormat = append(sendErrorAllowFormat, path.Ext(val.Path()))
			continue
		}

		t.FileConvertPath = config.DirBot + "/torrent-client/" + val.Path()
		t.FileName = strings.TrimSuffix(path.Base(val.Path()), path.Ext(path.Base(val.Path())))

		t.CreateFolderConvert()

		t.RunConvert()

		t.WaitSendFile()
	}

	if len(sendErrorAllowFormat) > 0 {
		t.App.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - warning, format not allowed %s",
			t.Message.From.UserName, t.Message.From.ID, strings.Join(sendErrorAllowFormat, " | ")))
	}
}

func (t *Task) SendFiles() {
	zipName := UniqueId(t.TorrentProcess.Info().BestName()) + ".zip"
	pathZip := config.DirBot + "/storage/" + zipName
	archive, err := os.Create(pathZip)
	if err != nil {
		log.Error(err)
		return
	}

	zipWriter := zip.NewWriter(archive)
	zipWriter.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})

	for _, val := range t.Files {
		t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
			fmt.Sprintf("🔥 Ziping - %s", val.Path())))

		pathToZip := config.DirBot + "/torrent-client/" + val.Path()
		_, err := os.Stat(pathToZip)
		if err != nil {
			log.Error(err)
			continue
		}

		file, err := os.Open(pathToZip)
		if err != nil {
			log.Warning(err)
			continue
		}

		ctz, err := zipWriter.Create(val.Path())
		if err != nil {
			log.Error(err)
			continue
		}
		if _, err := io.Copy(ctz, file); err != nil {
			log.Error(err)
			continue
		}

		file.Close()
	}

	zipWriter.Close()
	archive.Close()

	fiCh, _ := os.Stat(pathZip)
	isBigFile := fiCh.Size() > 1999e6 // more 2gb

	if isBigFile {
		t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
			fmt.Sprintf("😔 Files in the torrent are too big, zip archive size available only no more than 2 gb")))
	} else {
		if t.UserFromDB.Premium == 0 {
			t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
				fmt.Sprintf("😔 You don't have a donation, file not sent")))

			return
		}

		t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
			fmt.Sprintf("📲 Sending zip - %s \n\n⏰ Time upload to the telegram ~ 1-7 minutes",
				zipName)))

		doc := tgbotapi.NewDocument(t.Message.Chat.ID,
			tgbotapi.FilePath(pathZip))

		doc.Caption = t.TorrentProcess.Info().BestName()
		if t.UserFromDB.Forward == 0 {
			doc.ProtectContent = true
		}

		sentDoc, err := t.App.Bot.Send(doc)
		if err != nil {
			log.Error(err)
			t.App.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - zip file send err\n\n%s",
				t.Message.From.UserName, t.Message.From.ID, err))
		} else {
			t.App.SendLogToChannel("doc", fmt.Sprintf("@%s (%d) - doc file",
				t.Message.From.UserName, t.Message.From.ID), sentDoc.Document.FileID)
		}

		t.App.Bot.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, t.MessageEdit))
	}
}

func (t *Task) CreateFolderConvert() {
	t.FolderConvert = config.DirBot + "/storage/files-" +
		t.FileName + "-" + strconv.FormatInt(t.Message.From.ID, 10)
	err := os.Mkdir(t.FolderConvert, os.ModePerm)
	if err != nil {
		log.Warning(err)
	}
}

func (t *Task) CreateCover() *Task {
	t.FileCoverPath = t.FolderConvert + "/" + t.FileName + ".jpg"
	_, err := exec.Command("ffmpeg",
		"-protocol_whitelist", "file",
		"-i", t.FileConvertPath,
		"-ss", "00:00:30.000",
		"-vframes", "1",
		"-y",
		t.FileCoverPath).Output()

	if err != nil {
		log.Error(err)
	}

	return t
}

func (t *Task) GetSizeCover() (image.Point, error) {
	existingImageFile, err := os.Open(t.FileCoverPath)
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

func (t *Task) TimeTotalRaw() string {
	timeTotalRaw, err := exec.Command("ffprobe",
		"-protocol_whitelist", "file",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-sexagesimal",
		t.FileConvertPath).Output()
	if err != nil {
		log.Error(err)
	}

	return string(timeTotalRaw)
}

func (t *Task) RunConvert() {
	go func(t *Task) {
		t.App.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - start convert",
			t.Message.From.UserName, t.Message.From.ID))

		t.FileConvertPathOut = t.FolderConvert + "/" + t.FileName + ".mp4"

		presetConvert := "fast"
		if config.IsDev {
			presetConvert = "ultrafast"
		}

		prepareArgs := []string{
			"-protocol_whitelist", "file",
			"-v", "warning", "-hide_banner", "-stats",
			"-i", t.FileConvertPath,
			"-acodec", "mp2",
			"-vcodec", "h264",
			"-preset", presetConvert,
			"-ss", "00:00:00",
			"-t", "00:05:00",
			"-fs", "1990M",
			"-vf", "scale=iw/2:ih/2",
			"-y",
			t.FileConvertPathOut}

		fiCh, _ := os.Stat(t.FileConvertPath)

		isBigFile := fiCh.Size() > 4e9 // more 4gb

		var args []string
		for _, val := range prepareArgs {
			if t.UserFromDB.Premium == 1 && (strings.Contains(val, "-ss") ||
				strings.Contains(val, "00:00:00") ||
				strings.Contains(val, "-t") ||
				strings.Contains(val, "00:05:00")) {
				continue
			}
			if isBigFile == false && (strings.Contains(val, "-fs") || strings.Contains(val, "1990M") ||
				strings.Contains(val, "-vf") || strings.Contains(val, "scale=iw/2:ih/2")) {
				continue
			}

			args = append(args, val)
		}

		cmd := exec.Command("ffmpeg", args...)

		stdout, err := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout
		if err != nil {
			log.Error(err)
			return
		}
		if err = cmd.Start(); err != nil {
			log.Error(err)
			return
		}

		timeTotalRaw := t.TimeTotalRaw()
		for {
			tmp := make([]byte, 1024)
			_, err := stdout.Read(tmp)
			regx := regexp.MustCompile(`time=(.*?) `)
			matches := regx.FindStringSubmatch(string(tmp))

			var timeLeft time.Time
			if len(matches) == 2 {
				timeLeft, err = time.Parse("15:04:05,00", strings.Trim(matches[1], " "))
				if err != nil {
					log.Error(err)
				}
			} else {
				break
			}

			timeNull, _ := time.Parse("15:04:05", "00:00:00")
			timeTotal, err := time.Parse("15:04:05,000000", strings.Trim(timeTotalRaw, "\n"))
			if err != nil {
				log.Error(err)
			}

			t.PercentConvert, _ = strconv.ParseFloat(fmt.Sprintf("%.2f",
				100-(timeTotal.Sub(timeLeft).Seconds()/timeTotal.Sub(timeNull).Seconds())*100), 64)

			if err != nil {
				break
			}
		}

		if err := cmd.Wait(); err != nil {
			log.Error(err)
			return
		}

		t.PercentConvert = 100
	}(t)
}

func (t *Task) WaitSendFile() {
	for {
		t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
			fmt.Sprintf("🌪 Convert - %s \n\n🔥 Progress: %.2f%%", t.FileName, t.PercentConvert)))

		if t.PercentConvert == 100 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEdit,
		fmt.Sprintf("📲 Sending video - %s \n\n🍿 Time upload to the telegram ~ 1-7 minutes",
			t.FileName)))

	size, err := t.CreateCover().GetSizeCover()
	if err != nil {
		log.Error(err)
	}

	video := tgbotapi.NewVideo(t.Message.Chat.ID,
		tgbotapi.FilePath(t.FileConvertPathOut))

	video.SupportsStreaming = true
	video.Caption = t.FileName
	if t.UserFromDB.Forward == 0 {
		video.ProtectContent = true
	}
	video.Thumb = tgbotapi.FilePath(t.FileCoverPath)
	video.Width = size.X
	video.Height = size.Y

	sentVideo, err := t.App.Bot.Send(video)
	if err != nil {
		log.Error(err)
		t.App.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - video file send err\n\n%s",
			t.Message.From.UserName, t.Message.From.ID, err))
	} else {
		t.App.SendLogToChannel("video", fmt.Sprintf("@%s (%d) - video file",
			t.Message.From.UserName, t.Message.From.ID), sentVideo.Video.FileID)
	}

	t.App.Bot.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, t.MessageEdit))
}

func (t *Task) Cleaner() {
	t.App.LockForRemove.Add(1)

	log.Info("Folders cleaning...")

	pathConvert := config.DirBot + "/storage"
	dirs, _ := os.ReadDir(pathConvert)

	for _, val := range dirs {
		err := os.RemoveAll(pathConvert + "/" + val.Name())
		if err != nil {
			log.Error(err)
		}
	}

	for _, val := range t.Files {
		pathDir := path.Dir(val.Path())
		pathRemove := config.DirBot + "/torrent-client/" + path.Dir(val.Path())
		if pathDir == "." {
			pathRemove = config.DirBot + "/torrent-client/" + val.Path()
		}

		_, err := os.Stat(pathRemove)
		if err != nil {
			continue
		}

		err = os.RemoveAll(pathRemove)
		if err != nil {
			log.Error(err)
		}
	}

	t.App.LockForRemove.Done()
}

func UniqueId(prefix string) string {
	now := time.Now()
	sec := now.Unix()
	use := now.UnixNano() % 0x100000
	return fmt.Sprintf("%s-%08x%05x", prefix, sec, use)
}
