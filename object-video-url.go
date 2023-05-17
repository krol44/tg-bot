package main

import (
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path"
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
		"twitch.tv/videos",
		"twitch.tv/*****/clip",
		"rutube.ru/video",
		"instagram.com/reel",
		"coub.com/view",
	}

	var urlsForSend []string
	urlsForSend = append(allowUrls, urlsForSend...)
	urlsForSend = append(urlsForSend, []string{
		"open.spotify.com/track",
		"open.spotify.com/album",
	}...)

	var allowUrl bool
	for _, val := range allowUrls {
		if strings.Contains(urlVideo, "twitch.tv") && strings.Contains(urlVideo, "/clip") {
			allowUrl = true
		}
		if strings.Contains(urlVideo, val) {
			allowUrl = true
		}
	}
	if !allowUrl {
		uFs := strings.Replace(strings.Join(urlsForSend, "\n"), "instagram.com/reel", "", 1)
		m := tgbotapi.NewMessage(o.Task.Message.Chat.ID,
			"❗️ "+o.Task.Lang("Not allowed url, I support only:")+"\n"+uFs)
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

	if o.Task.Limit("video-url") {
		log.Debug("limit exceeded")
		return false
	}

	if !o.Task.Alloc("video-url") {
		o.Task.App.SendLogToChannel(o.Task.Message.From, "mess", "❗️ return - error alloc")
		return false
	}

	infoArgs := []string{"-j", "--socket-timeout", "10", urlVideo}
	if strings.Contains(o.Task.Message.Text, "instagram.com/reel") {
		infoArgs = append(infoArgs, []string{"--cookies", "instagram-cookies.txt"}...)
	}

	cmd := exec.Command("yt-dlp", infoArgs...)
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
		o.Task.App.SendLogToChannel(o.Task.Message.From, "mess", "❗️ Video url is bad 1")
		log.Warn(err)
		return false
	}

	var infoVideo InfoYtDlp
	err = json.Unmarshal(out, &infoVideo)
	if err != nil {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")+" 2"))
		log.Error(err)
		return false
	}

	if infoVideo.ID == "" {
		o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, "❗️ "+o.Task.Lang("Video url is bad")+" 3"))
		log.Error("not found id - " + urlVideo)
		return false
	}

	protectedFlag = false

	if _, isSlice := o.Task.GetTimeSlice(); isSlice {
		o.Task.Message.Text += " +skip-cache-id +quality"
	}

	u, err := url.Parse(urlVideo)
	if err != nil {
		log.Error(err)
		return false
	}

	o.Task.UrlIDForCache = strings.Split(strings.Replace(u.Host, "www.", "", 1), ".")[0] +
		"-" + infoVideo.ID
	cache := Cache{Task: o.Task}
	if !strings.Contains(o.Task.Message.Text, "+skip-cache-id") {
		if cache.TrySendThroughID() {
			return false
		}
	}

	cleanTitle := strings.ReplaceAll(infoVideo.FullTitle, "#", "")

	folder := config.DirBot + "/storage" + "/" + o.Task.UniqueId("files-video")

	quality := "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4] / bv*+ba/b"
	if strings.Contains(o.Task.Message.Text, "+quality") {
		quality = "bv*+ba/b"
	}

	if strings.Contains(o.Task.Message.Text, "coub.com/view") {
		quality = "bestvideo,bestaudio"
	}

	log.Debug(o.Task.Message.Text, " / ", quality)

	argsPre := []string{
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

	if strings.Contains(o.Task.Message.Text, "instagram.com/reel") {
		argsPre = append(argsPre, []string{"--cookies", "instagram-cookies.txt"}...)
	}

	var args []string
	for _, v := range argsPre {
		if strings.Contains(o.Task.Message.Text, "coub.com/view") && v == "-S" {
			args = append(args, "-k")
		}
		args = append(args, v)
	}

	cmd = exec.Command("yt-dlp", args...)
	defer func(c *exec.Cmd) {
		c.Process.Kill()
		if err := c.Wait(); err != nil {
			log.Info(err)
		}
		c.Process.Release()
	}(cmd)

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
	if err != nil {
		log.Error(err)
		return false
	}
	if err = cmd.Start(); err != nil {
		log.Error(err)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*20)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID,
				"😔 "+o.Task.Lang("Didn't have time to download")))
			o.Task.App.SendLogToChannel(o.Task.Message.From, "mess",
				"Didn't have time to download video url")
			return false
		default:
		}

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
		mess := fmt.Sprintf("🔽 %s \n\n🔥 "+o.Task.Lang("Download progress")+": %s%%",
			cleanTitle, percent)
		if o.Task.MessageTextLast != mess {
			o.Task.Send(tgbotapi.NewEditMessageText(o.Task.Message.Chat.ID, o.Task.MessageEditID, mess))
			o.Task.MessageTextLast = mess
		}
		if _, bo := o.Task.App.ChatsWork.StopTasks.Load(o.Task.Message.Chat.ID); bo {
			stopProtected = true
			return false
		}

		if percent == "100" {
			break
		}

		time.Sleep(2 * time.Second)
	}

	if strings.Contains(o.Task.Message.Text, "coub.com/view") {
		if !o.prepareCoub(folder) {
			return false
		}
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

	if !strings.Contains(o.Task.Message.Text, "+skip-cache-id") {
		if cache.TrySendThroughMd5(filePath) {
			return false
		}
	}

	o.Task.File = filePath

	return true
}

func (o *ObjectVideoUrl) prepareCoub(folder string) bool {
	if strings.Contains(o.Task.Message.Text, "coub.com/view") {
		dir, err := os.ReadDir(folder)
		if err != nil {
			log.Error(err)
			return false
		}

		var (
			as       int
			vs       int
			pathMp3  string
			pathMp4  string
			pathName string
		)
		c := Convert{}
		for _, file := range dir {
			if path.Ext(file.Name()) == ".mp3" {
				pathMp3 = folder + "/" + file.Name()
				asp := c.TimeTotalRaw(pathMp3)
				as = int(asp.Sub(time.Date(0000, 01, 01, 00, 00, 00, 0,
					time.UTC)).Seconds())
			}

			if path.Ext(file.Name()) == ".mp4" {
				pathMp4 = folder + "/" + file.Name()
				vs = c.TimeTotalRaw(pathMp4).Second()
				pathName = strings.Replace(path.Base(pathMp4), path.Ext(pathMp4), "", 1)
			}
		}

		if pathName == "" {
			return false
		}

		loop := fmt.Sprintf("%v", math.Floor(float64(as)/float64(vs)))

		ffmpegPath := "./ffmpeg"
		if config.IsDev {
			ffmpegPath = "ffmpeg"
		}
		_, err = exec.Command(ffmpegPath,
			"-v", "quiet", "-stream_loop", loop, "-t", fmt.Sprintf("%d", as), "-i", pathMp4, "-i", pathMp3,
			"-c", "copy", folder+"/"+pathName+"-coub.mp4").Output()
		if err != nil {
			log.Warn(err)
			return false
		}

		err = os.Remove(pathMp3)
		if err != nil {
			log.Error(err)
			return false
		}
		err = os.Remove(pathMp4)
		if err != nil {
			log.Error(err)
			return false
		}
	}

	return true
}

func (o *ObjectVideoUrl) Convert() bool {
	var c = Convert{Task: o.Task, IsTorrent: false}

	if c.Task.IsAllowFormatForConvert(c.Task.File) {
		o.Task.FileConverted = c.Run()

		return true
	} else {
		return false
	}
}

func (o *ObjectVideoUrl) Send() bool {
	return o.Task.SendVideo(false)
}

func (o *ObjectVideoUrl) Clean() {
	o.Task.RemoveMessageEdit()
}
