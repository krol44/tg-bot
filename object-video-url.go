package main

import (
	"encoding/json"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func (o *ObjectVideoUrl) Download() bool {
	urlVideo := o.Task.Message.Text

	allowUrls := []string{
		"youtube.com",
		"youtu.be",
		"tiktok.com",
		"vk.com/video",
	}

	var urlsForSend []string
	urlsForSend = append(allowUrls, urlsForSend...)
	urlsForSend = append(urlsForSend, []string{
		"open.spotify.com/track",
		"open.spotify.com/album",
	}...)

	var allowUrl bool
	for _, val := range allowUrls {
		if strings.Contains(urlVideo, val) {
			allowUrl = true
		}
	}
	if !allowUrl {
		m := tgbotapi.NewMessage(o.Task.Message.Chat.ID,
			"❗️ "+o.Task.Lang("Not allowed url, I support only:")+"\n"+strings.Join(urlsForSend, "\n"))
		m.DisableWebPagePreview = true
		o.Task.Send(m)
		return false
	}

	sp := strings.Split(o.Task.Message.Text, "&")
	if len(sp) >= 1 {
		urlVideo = sp[0]
	}
	o.Task.DescriptionUrl = urlVideo

	_, err := url.ParseRequestURI(urlVideo)
	if err != nil {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")))
		log.Error(err)
		return false
	}

	o.Task.Alloc("video-url")

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
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")+" 1"))
		log.Error(err)
		return false
	}

	var infoVideo InfoYtDlp
	err = json.Unmarshal(out, &infoVideo)
	if err != nil {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")+" 2"))
		log.Error(err)
		return false
	}

	if infoVideo.FilesizeApprox == 0 {
		infoVideo.FilesizeApprox = infoVideo.Filesize
	}
	if infoVideo.FilesizeApprox == 0 {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")+" 3"))
		return false
	}

	protectedFlag = false

	o.Task.UrlIDForCache = infoVideo.ID
	cache := Cache{Task: o.Task}
	if !strings.Contains(o.Task.Message.Text, "+skip-cache-id") {
		if cache.TrySendThroughID() {
			return false
		}
	}

	cleanTitle := strings.ReplaceAll(infoVideo.FullTitle, "#", "")

	infoText := fmt.Sprintf("📺 "+o.Task.Lang("Video")+": %s", cleanTitle)
	o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, infoText))

	folder := config.DirBot + "/storage" + "/" + o.Task.UniqueId("files-video")

	quality := "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4] / bv*+ba/b"
	if strings.Contains(o.Task.Message.Text, "+quality") {
		quality = "bv*+ba/b"
	}

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
		"-f", quality,
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
			size, _ := o.Task.DirSize(folder)

			if sizeSave == size {
				cmd.Process.Kill()
				log.Warning("kill cmd download video url")
				o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID,
					"❗️ "+o.Task.Lang("Video url is bad")+" 4"))
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
			log.Debug(err)
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

		if percent == "0" {
			percent = "•• "
		}
		mess := fmt.Sprintf("🔽 %s \n\n🔥 "+o.Task.Lang("Download progress")+": %s%%", cleanTitle, percent)
		if o.Task.MessageTextLast != mess {
			o.Task.Send(tgbotapi.NewEditMessageText(o.Task.Message.Chat.ID, o.Task.MessageEditID, mess))
			o.Task.MessageTextLast = mess
		}

		if _, bo := o.Task.App.ChatsWork.StopTasks.Load(o.Task.Message.Chat.ID); bo {
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

	o.Task.File = filePath

	return true
}

func (o *ObjectVideoUrl) Convert() bool {
	var c = Convert{Task: o.Task, IsTorrent: false}

	if c.CheckExistVideo() {
		o.Task.FileConverted = c.Run()

		return true
	} else {
		return false
	}
}

func (o *ObjectVideoUrl) Send() bool {
	return o.Task.SendVideo()
}

func (o *ObjectVideoUrl) Clean() {
	o.Task.RemoveMessageEdit()
}
