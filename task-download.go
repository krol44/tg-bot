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
	"regexp"
	"strings"
	"time"
)

func (t *Task) DownloadYoutube() []string {
	urlVideo := t.Message.Text

	sp := strings.Split(t.Message.Text, "&")
	if len(sp) >= 1 {
		urlVideo = sp[0]
	}

	_, err := url.ParseRequestURI(urlVideo)
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ Youtube url is bad"))
		log.Error(err)
		return nil
	}

	t.Alloc("youtube")

	cmd := exec.Command("yt-dlp", "-j", "--socket-timeout", "10", urlVideo)
	// protected
	protectedFlag := true
	go func(cmd *exec.Cmd, protectedFlag *bool) {
		time.Sleep(5 * time.Second)
		if *protectedFlag == true {
			log.Warning("kill youtube process")
			cmd.Process.Kill()
		}
	}(cmd, &protectedFlag)

	out, err := cmd.Output()
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ Youtube video is bad"))
		log.Error(err)
		return nil
	}

	var infoVideo struct {
		FullTitle      string `json:"fulltitle"`
		FilesizeApprox int    `json:"filesize_approx"`
		Filename       string `json:"_filename"`
	}
	err = json.Unmarshal(out, &infoVideo)
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ Youtube video is bad"))
		log.Error(err)
		return nil
	}

	if infoVideo.FilesizeApprox == 0 {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ Youtube video is bad"))
		return nil
	}

	protectedFlag = false

	infoText := fmt.Sprintf("📺 Youtube: %s", infoVideo.FullTitle)
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

	name := path.Clean(strings.TrimSuffix(infoVideo.Filename, path.Ext(infoVideo.Filename)))
	folder := config.DirBot + "/storage" + "/" + t.UniqueId("files-youtube")

	args := []string{
		"--socket-timeout", "10",
		"--newline",
		"-q", "--progress",
		"--no-playlist",
		"--no-colors",
		"--ignore-errors", "--no-warnings",
		//"--write-thumbnail", "--convert-thumbnails", "jpg",
		"--sponsorblock-mark", "all",
		"-f", "bv+ba/b",
		"-o", fmt.Sprintf("%s/%%(title)s - %%(upload_date)s", folder),
		urlVideo,
	}

	cmd = exec.Command("yt-dlp", args...)

	go func(cmd *exec.Cmd, folder string) {
		var sizeSave int64
		for {
			time.Sleep(60 * time.Second)
			stat, _ := os.Stat(folder)
			log.Warning(sizeSave == stat.Size(), sizeSave, stat.Size())
			if sizeSave == stat.Size() {
				cmd.Process.Kill()
				log.Warning("kill cmd dl youtube")
				t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ Youtube video is bad"))
				break
			}
			sizeSave = stat.Size()
		}
	}(cmd, folder)

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

		ls := strings.Split(string(tmp), "\n")
		var lastResult string
		if len(ls) > 1 {
			lastResult = ls[len(ls)-2]
		}

		regx := regexp.MustCompile(`\[download\](.*?)%`)
		matches := regx.FindStringSubmatch(lastResult)

		var percent string
		if len(matches) == 2 {
			percent = strings.TrimSpace(matches[1])
		}
		_, err = t.App.Bot.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("🔽 %s \n\n🔥 Download progress: %s%%", name, percent)))
		if err != nil {
			log.Warning(err)
			break
		}
		if percent == "100" {
			break
		}

		if err != nil {
			log.Warning(err)
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
		filePath = file.Name()
		break
	}

	t.Files = []string{folder + "/" + filePath}
	return t.Files
}

func (t *Task) DownloadTorrentFiles() []string {
	// todo t.Torrent.Process, _ := client.AddMagnet("magnet:?xt=urn:btih:....")
	file, err := t.App.Bot.GetFile(tgbotapi.FileConfig{FileID: t.Message.Document.FileID})

	if err != nil {
		log.Warning(err)
		return nil
	}

	// log
	qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
	t.App.SendLogToChannel(t.Message.From.ID, "doc",
		fmt.Sprintf("upload torrent file | His turn: %d", qn.(int)+1), t.Message.Document.FileID)

	t.Alloc("torrent")

	t.Torrent.Process, err = t.App.TorClient.AddTorrentFromFile(config.TgPathLocal + "/" + config.BotToken +
		"/" + file.FilePath)
	if err != nil {
		log.Error(err)
		return nil
	}

	// if name not correct
	t.Torrent.Process.SetDisplayName(t.UniqueId("temp-torrent-name"))

	t.Torrent.Process.SetMaxEstablishedConns(200)

	<-t.Torrent.Process.GotInfo()

	t.Torrent.Name = t.Torrent.Process.Info().BestName()

	infoText := fmt.Sprintf("🎈 Torrent: %s", t.Torrent.Name)

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
			infoText += fmt.Sprintf("\n\n📋 List of files:\n") + listFiles
		}
	}

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

	go func() {
		for {
			stat, percent := t.statDlTor()
			if percent == 100 {
				break
			}

			if time.Now().Second()%2 == 0 {
				t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, stat))
			}
			time.Sleep(time.Second)
		}
	}()

	t.Torrent.Process.DownloadAll()

	<-t.Torrent.Process.Complete.On()
	t.Torrent.Process.Drop()

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, "✅ Torrent downloaded"))

	// todo if files are big, do something - t.Torrent.Process.Files()
	//fiCh, _ := os.Stat(file)
	//isBigFile := fiCh.Size() > 4e9 // more 4gb
	for _, torFile := range t.Torrent.Process.Files() {
		t.Files = append(t.Files, path.Clean(config.DirBot+"/torrent-client/"+torFile.Path()))
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

	stat := fmt.Sprintf("🔥 Progress: \t%s / %s  %.2f%%\n\n🔽 Download speed: %s",
		complete, size, percentage, downloadSpeed)

	// if it needs
	if uploadProgress > 0 {
		stat += fmt.Sprintf("\n\n🔼 Upload speed: \t%s", uploadSpeed)
	}

	return stat, percentage
}
