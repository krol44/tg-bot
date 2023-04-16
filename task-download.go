package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anacrolix/torrent"
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

func (t *Task) DownloadVideoUrl() bool {
	urlVideo := t.Message.Text

	sp := strings.Split(t.Message.Text, "&")
	if len(sp) >= 1 {
		urlVideo = sp[0]
	}
	t.VideoUrlHttp = urlVideo

	_, err := url.ParseRequestURI(urlVideo)
	if err != nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")))
		log.Error(err)
		return false
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
		return false
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
		return false
	}

	if infoVideo.FilesizeApprox == 0 {
		infoVideo.FilesizeApprox = infoVideo.Filesize
	}
	if infoVideo.FilesizeApprox == 0 {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("Video url is bad")+" 3"))
		return false
	}

	protectedFlag = false

	t.VideoUrlID = infoVideo.ID
	cache := Cache{Task: t}
	if cache.TrySendThroughID() {
		return false
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
		return false
	}
	if err = cmd.Start(); err != nil {
		log.Error(err)
		return false
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

			return false
		}

		if percent == "100" {
			break
		}

		time.Sleep(2 * time.Second)
	}

	if err := cmd.Wait(); err != nil {
		log.Error(err)
		return false
	}

	dir, err := os.ReadDir(folder)
	if err != nil {
		log.Error(err)
		return false
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

	if cache.TrySendThroughMd5(filePath) {
		return false
	}

	t.File = filePath

	return true
}

func (t *Task) DownloadTorrentFile(torrentProcess *torrent.Torrent) bool {
	t.Alloc("torrent")

	t.Torrent.Process = torrentProcess

	var fileChosen *torrent.File
	for _, val := range t.Torrent.Process.Files() {
		if strings.Contains(t.Message.Text, val.DisplayPath()) {
			if val.Length() > 1999e6 { // more 2 GB
				t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, fmt.Sprintf("😔 "+t.Lang("File is bigger 2 GB"))))
				return false
			}
			val.SetPriority(torrent.PiecePriorityNow)
			fileChosen = val
		} else {
			val.SetPriority(torrent.PiecePriorityNone)
		}
	}

	// if name not correct
	t.Torrent.Process.SetDisplayName(t.UniqueId("temp-torrent-name"))

	t.Torrent.Process.SetMaxEstablishedConns(200)

	<-t.Torrent.Process.GotInfo()

	t.Torrent.Name = fileChosen.DisplayPath()

	infoText := fmt.Sprintf("🎈 "+t.Lang("Torrent")+": %s", t.Torrent.Name)

	messInfo := t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, infoText))
	// pin
	pinChatInfoMess := tgbotapi.PinChatMessageConfig{
		ChatID:              messInfo.Chat.ID,
		MessageID:           messInfo.MessageID,
		DisableNotification: true,
	}

	_, err := t.App.Bot.Request(pinChatInfoMess)
	if err != nil {
		log.Warning(err)
	}

	ctx, cancelProgress := context.WithCancel(context.Background())
	go func(ctx context.Context, fileChosen *torrent.File) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
					return
				}

				stat, percent := t.statDlTor(fileChosen)
				if percent == 100 {
					return
				}

				go func(ctx context.Context, t *Task) {
					select {
					case <-ctx.Done():
						return
					default:
						if time.Now().Second()%2 == 0 && t.MessageTextLast != stat {
							t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, stat))
							t.MessageTextLast = stat
						}
					}
				}(ctx, t)
			}
			time.Sleep(time.Second)
		}
	}(ctx, fileChosen)

	t.Torrent.Process.AllowDataDownload()

	for {
		if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
			break
		}
		if fileChosen.FileInfo().Length == fileChosen.BytesCompleted() {
			break
		}
		time.Sleep(time.Second)
	}

	cancelProgress()

	if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
		t.Torrent.Process.Drop()
		return false
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		"✅ "+t.Lang("Torrent downloaded, wait next step")))

	pathway := path.Clean(config.DirBot + "/torrent-client/" + fileChosen.Path())

	cache := Cache{Task: t}
	if cache.TrySend("video", pathway) {
		return false
	}
	if cache.TrySendThroughMd5(pathway) {
		return false
	}
	if cache.TrySend("doc", t.Torrent.Name+".torrent") {
		return false
	}

	t.File = pathway

	return true
}

func (t *Task) statDlTor(fileChosen *torrent.File) (string, float64) {
	if t.Torrent.Process.Info() == nil {
		return "", 0
	}

	currentProgress := t.Torrent.Process.BytesCompleted()
	downloadSpeed := humanize.Bytes(uint64(currentProgress-t.Torrent.Progress)) + "/s"
	t.Torrent.Progress = currentProgress

	ctlInfo := fileChosen.FileInfo().Length
	complete := humanize.Bytes(uint64(fileChosen.BytesCompleted()))
	size := humanize.Bytes(uint64(ctlInfo))
	var percentage float64
	if ctlInfo != 0 {
		percentage = float64(fileChosen.BytesCompleted()) / float64(ctlInfo) * 100
	}

	stat := fmt.Sprintf(
		"🔥 "+t.Lang("Progress")+": \t%s / %s  %.2f%%\n\n🔽 "+t.Lang("Speed")+
			": %s (Act. peers %d / Total %d)",
		complete, size, percentage, downloadSpeed, t.Torrent.Process.Stats().ActivePeers,
		t.Torrent.Process.Stats().TotalPeers)

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
