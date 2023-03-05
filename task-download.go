package main

import (
	"encoding/json"
	"fmt"
	"github.com/dustin/go-humanize"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func (t *Task) DownloadVideoUrl() []string {
	urlVideo := t.Message.Text

	sp := strings.Split(t.Message.Text, "&")
	if len(sp) >= 1 {
		urlVideo = sp[0]
	}

	_, err := url.ParseRequestURI(urlVideo)
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")))
		log.Error(err)
		return nil
	}

	t.Alloc("video-url")

	cmd := exec.Command("yt-dlp", "-j", "--socket-timeout", "10", urlVideo)
	// protected
	protectedFlag := true
	go func(cmd *exec.Cmd, protectedFlag *bool) {
		time.Sleep(10 * time.Second)
		if *protectedFlag == true {
			log.Warning("get info video url kill process")
			cmd.Process.Kill()
		}
	}(cmd, &protectedFlag)

	out, err := cmd.Output()

	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")+" 1"))
		log.Error(err)
		return nil
	}

	var infoVideo struct {
		ID             string `json:"id"`
		FullTitle      string `json:"fulltitle"`
		FilesizeApprox int    `json:"filesize_approx"`
		Filesize       int    `json:"filesize"`
		Filename       string `json:"_filename"`
	}
	err = json.Unmarshal(out, &infoVideo)
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")+" 2"))
		log.Error(err)
		return nil
	}

	if infoVideo.FilesizeApprox == 0 {
		infoVideo.FilesizeApprox = infoVideo.Filesize
	}
	if infoVideo.FilesizeApprox == 0 {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")+" 3"))
		return nil
	}

	protectedFlag = false

	t.VideoUrlID = infoVideo.ID
	cache := Cache{Task: t}
	if cache.TrySendThroughId() {
		return []string{}
	}

	cleanTitle := strings.ReplaceAll(infoVideo.FullTitle, "#", "")

	infoText := fmt.Sprintf("📺 "+t.Lang("Video")+": %s", cleanTitle)
	messInfo := t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, infoText))
	// pin
	pinChatInfoMess := tgbotapi.PinChatMessageConfig{
		ChatID:              messInfo.Chat.ID,
		MessageID:           messInfo.MessageID,
		DisableNotification: true,
	}
	if _, err = t.App.Bot.Request(pinChatInfoMess); err != nil {
		log.Warning(err)
	}

	folder := config.DirBot + "/storage" + "/" + t.UniqueId("files-video")

	args := []string{
		"--bidi-workaround",
		"--socket-timeout", "10",
		"--newline",
		//"-q", "--progress",
		"--no-playlist",
		"--no-colors",
		//"--ignore-errors", "--no-warnings",
		//"--write-thumbnail", "--convert-thumbnails", "jpg",
		"--sponsorblock-mark", "all",
		"-f", "bv+ba/b",
		"-S", "filesize:1990M",
		"-o", fmt.Sprintf("%s/%%(title).100s - %%(upload_date)s.%%(ext)s", folder),
		urlVideo,
	}

	cmd = exec.Command("yt-dlp", args...)

	stopProtected := false
	go func(cmd *exec.Cmd, folder string, stopProtected *bool) {
		var sizeSave int64
		for {
			time.Sleep(60 * time.Second)

			if *stopProtected {
				break
			}
			size, _ := t.DirSize(folder)

			if sizeSave == size {
				cmd.Process.Kill()
				log.Warning("kill cmd download video url")
				t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")+" 4"))
				break
			}
			sizeSave = size
		}
	}(cmd, folder, &stopProtected)

	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	defer stdout.Close()
	if err != nil {
		log.Error(err)
		return nil
	}
	if err = cmd.Start(); err != nil {
		log.Error(err)
		return nil
	}

	for {
		tmp := make([]byte, 1024*400)
		_, err := stdout.Read(tmp)
		if err != nil {
			log.Warn(err)
			break
		}

		ls := strings.Split(string(tmp), "\n")
		var lastResult string
		if len(ls) > 1 {
			lastResult = ls[len(ls)-2]
		}

		regx := regexp.MustCompile(`\[download\](.*?)%`)
		matches := regx.FindStringSubmatch(lastResult)

		var percent = "0"
		if len(matches) == 2 {
			percent = strings.TrimSpace(matches[1])
		}

		mess := fmt.Sprintf("🔽 %s \n\n🔥 "+t.Lang("Download progress")+": %s%%", cleanTitle, percent)
		if t.MessageTextLast != mess {
			t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, mess))
			t.MessageTextLast = mess
		}

		if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
			warn := cmd.Process.Kill()
			if warn != nil {
				log.Warn(warn)
			}

			stopProtected = true

			return nil
		}

		if percent == "100" {
			break
		}

		time.Sleep(2 * time.Second)
	}

	if err := cmd.Wait(); err != nil {
		log.Error(err)
		return nil
	}

	dir, err := os.ReadDir(folder)
	if err != nil {
		log.Error(err)
		return nil
	}

	var filePath string
	for _, file := range dir {
		oldFilePath := folder + "/" + file.Name()
		filePath = strings.ReplaceAll(oldFilePath, "#", "")

		err := os.Rename(oldFilePath, filePath)
		if err != nil {
			log.Error(err)
		}

		break
	}

	stopProtected = true

	t.Files = []string{filePath}
	if cache.TrySendThroughMd5(filePath) {
		t.Files = []string{}
	}

	return t.Files
}

func (t *Task) DownloadTorrentFiles() []string {
	isMagnet := strings.Contains(t.Message.Text, "magnet:?xt=")

	var (
		file tgbotapi.File
		err  error
	)

	qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
	if !isMagnet {
		file, err = t.App.Bot.GetFile(tgbotapi.FileConfig{FileID: t.Message.Document.FileID})

		if err != nil {
			log.Warning(err)
			return nil
		}
		// log
		t.App.SendLogToChannel(t.Message.From.ID, "doc",
			fmt.Sprintf("upload torrent file | his turn: %d", qn.(int)+1), t.Message.Document.FileID)
	} else {
		// log
		t.App.SendLogToChannel(t.Message.From.ID, "mess",
			fmt.Sprintf("torrent magnet | his turn: %d", qn.(int)+1))
	}

	t.Alloc("torrent")

	if !isMagnet {
		t.Torrent.Process, err = t.App.TorClient.AddTorrentFromFile(config.TgPathLocal + "/" + config.BotToken +
			"/" + file.FilePath)
	} else {
		t.Torrent.Process, err = t.App.TorClient.AddMagnet(t.Message.Text)
	}
	if err != nil {
		log.Error(err)
		return nil
	}

	// if name not correct
	t.Torrent.Process.SetDisplayName(t.UniqueId("temp-torrent-name"))

	t.Torrent.Process.SetMaxEstablishedConns(200)

	<-t.Torrent.Process.GotInfo()

	t.Torrent.Name = t.Torrent.Process.Info().BestName()

	infoText := fmt.Sprintf("🎈 "+t.Lang("Torrent")+": %s", t.Torrent.Name)

	t.Torrent.Process.Info()

	if len(t.Torrent.Process.Info().Files) != 0 {
		var listFiles string
		for _, val := range t.Torrent.Process.Info().Files {
			if len(val.Path) == 0 {
				continue
			}
			if t.IsAllowFormatForConvert(val.Path[0]) {
				listFiles += fmt.Sprintf("%s\n", val.Path[0])
			}
		}
		if listFiles != "" {
			infoText += fmt.Sprintf("\n\n📋 "+t.Lang("List of files")+":\n") + listFiles
		}
	}

	messInfo := t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, infoText))
	// pin
	pinChatInfoMess := tgbotapi.PinChatMessageConfig{
		ChatID:              messInfo.Chat.ID,
		MessageID:           messInfo.MessageID,
		DisableNotification: true,
	}
	if _, errPin := t.App.Bot.Request(pinChatInfoMess); err != nil {
		log.Warning(errPin)
	}

	go func() {
		for {
			stat, percent := t.statDlTor()
			if percent == 100 {
				break
			}

			if time.Now().Second()%2 == 0 && t.MessageTextLast != stat {
				t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, stat))
				t.MessageTextLast = stat
			}
			time.Sleep(time.Second)

			if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
				t.Torrent.Process.Complete.SetBool(true)
				break
			}
		}
	}()

	t.Torrent.Process.DownloadAll()

	<-t.Torrent.Process.Complete.On()
	t.Torrent.Process.Drop()

	if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
		return nil
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		"✅ "+t.Lang("Torrent downloaded, wait next step")))

	cache := Cache{Task: t}
	for _, torFile := range t.Torrent.Process.Files() {
		pathway := path.Clean(config.DirBot + "/torrent-client/" + torFile.Path())

		fiCh, _ := os.Stat(pathway)
		if fiCh.Size() > 2e+9 {
			t.Send(tgbotapi.NewMessage(t.Message.Chat.ID,
				fmt.Sprintf("❗ "+t.Lang("%s - File is more 2gb, will be skipped"), path.Base(pathway))))
			time.Sleep(time.Second * 2)
			continue
		}

		if cache.TrySend("video", pathway) {
			continue
		}

		if cache.TrySendThroughMd5(pathway) {
			continue
		}

		t.Files = append(t.Files, pathway)
	}

	return t.Files
}

func (t *Task) statDlTor() (string, float64) {
	if t.Torrent.Process.Info() == nil {
		return "", 0
	}

	currentProgress := t.Torrent.Process.BytesCompleted()
	downloadSpeed := humanize.Bytes(uint64(currentProgress-t.Torrent.TorrentProgress)) + "/s"
	t.Torrent.TorrentProgress = currentProgress

	complete := humanize.Bytes(uint64(currentProgress))
	size := humanize.Bytes(uint64(t.Torrent.Process.Info().TotalLength()))

	bytesWrittenData := t.Torrent.Process.Stats().BytesWrittenData
	uploadProgress := (&bytesWrittenData).Int64() - t.Torrent.Uploaded
	uploadSpeed := humanize.Bytes(uint64(uploadProgress)) + "/s"
	t.Torrent.Uploaded = uploadProgress

	ctlInfo := t.Torrent.Process.Info()
	var percentage float64
	if ctlInfo != nil {
		percentage = float64(t.Torrent.Process.BytesCompleted()) / float64(ctlInfo.TotalLength()) * 100
	}

	stat := fmt.Sprintf(
		"🔥 "+t.Lang("Progress")+": \t%s / %s  %.2f%%\n\n🔽 "+t.Lang("Download speed")+": %s",
		complete, size, percentage, downloadSpeed)

	// if it needs
	if uploadProgress > 0 {
		stat += fmt.Sprintf("\n\n🔼 "+t.Lang("Upload speed")+": \t%s", uploadSpeed)
	}

	return stat, percentage
}

func (Task) DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}
