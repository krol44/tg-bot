package main

import (
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

func (o *ObjectSpotify) Download() bool {
	urlAudio := o.Task.Message.Text

	sp := strings.Split(o.Task.Message.Text, "&")
	if len(sp) >= 1 {
		urlAudio = sp[0]
	}
	o.Task.DescriptionUrl = urlAudio

	_, err := url.ParseRequestURI(urlAudio)
	if err != nil {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Audio url is bad")))
		log.Error(err)
		return false
	}

	o.Task.Alloc("spotify")

	folder := config.DirBot + "/storage" + "/" + o.Task.UniqueId("files-audio")

	ffmpegPath := "./ffmpeg"
	if config.IsDev {
		ffmpegPath = "ffmpeg"
	}
	args := []string{
		"--simple-tui",
		"--ffmpeg", ffmpegPath,
		"--lyrics=musixmatch",
		urlAudio,
		"--output", fmt.Sprintf("%s/{track-number}. {artist} - {title} ({year}).{output-ext}", folder),
	}

	cmd := exec.Command("spotdl", args...)

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
				log.Warning("kill cmd download audio url")
				o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID,
					"❗️ "+o.Task.Lang("Audio url is bad")+" 4"))
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
		regx := regexp.MustCompile(`(.*?)Downloading`)
		matches := regx.FindStringSubmatch(lastResult)

		line := "Downloading..."
		if len(matches) == 2 {
			line = strings.TrimSpace(matches[0])
		}

		mess := "🔥 " + line
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

		time.Sleep(time.Second)
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

	var filesPath []string
	for _, file := range dir {
		path := folder + "/" + file.Name()
		filesPath = append(filesPath, path)
	}

	stopProtected = true

	o.Task.Files = filesPath

	return true
}

func (o *ObjectSpotify) Convert() bool {
	return true
}

func (o *ObjectSpotify) Send() bool {
	return o.Task.SendAudio()
}

func (o *ObjectSpotify) Clean() {
	o.Task.RemoveMessageEdit()
}
