package main

import (
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"time"
)

const signAdvt = "\n\n@TorPurrBot - Download Torrent Video or File, YouTube, TikTok, Other"

func (t Task) SendVideo(file FileConverted) {
	if file.FilePath == "" {
		return
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		fmt.Sprintf("📲 "+t.Lang("Sending video")+" - %s \n\n🍿 "+
			t.Lang("Time upload to the telegram ~ 1-7 minutes"), file.Name)))
	t.App.SendLogToChannel(t.Message.From, "mess", "sending video")

	video := tgbotapi.NewVideo(t.Message.Chat.ID,
		tgbotapi.FilePath(file.FilePath))

	var urlHttp string
	if t.VideoUrlHttp != "" {
		urlHttp = "\n" + t.VideoUrlHttp
	}

	video.SupportsStreaming = true
	video.Caption = file.Name + urlHttp + signAdvt
	video.Thumb = tgbotapi.FilePath(file.CoverPath)
	video.Width = file.CoverSize.X
	video.Height = file.CoverSize.Y

	stopAction := false
	go func(stopAction *bool) {
		for {
			if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
				return
			}

			if *stopAction == true {
				return
			}

			_, _ = t.App.Bot.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_video"))

			time.Sleep(4 * time.Second)
		}
	}(&stopAction)

	sentVideo, err := t.App.Bot.Send(video)
	if err != nil {
		stopAction = true
		log.Error(err)

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			"😞 "+t.Lang("Something wrong... I will be fixing it")))

		t.App.SendLogToChannel(t.Message.From, "mess",
			fmt.Sprintf("video file send err\n\n%s", err))
		return
	} else {
		stopAction = true

		t.App.SendLogToChannel(t.Message.From, "video", fmt.Sprintf("video file - "+file.Name),
			sentVideo.Video.FileID)

		Cache.Add(Cache{Task: &t}, sentVideo.Video.FileID, sentVideo.Video.FileSize, file.FilePathNative)
	}
}

func (t Task) SendTorFile() {
	if t.File == "" {
		return
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		fmt.Sprintf("📲 "+t.Lang("Sending...")+" - %s \n\n⏰ "+
			t.Lang("Time upload to the telegram ~ 1-7 minutes"), t.Torrent.Name)))
	t.App.SendLogToChannel(t.Message.From, "mess", "sending file...")

	doc := tgbotapi.NewDocument(t.Message.Chat.ID,
		tgbotapi.FilePath(t.File))

	doc.Caption = t.Torrent.Name + signAdvt

	stopAction := false
	go func(stopAction *bool) {
		for {
			if *stopAction == true {
				return
			}

			_, _ = t.App.Bot.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_document"))

			time.Sleep(4 * time.Second)
		}
	}(&stopAction)

	sentDoc, err := t.App.Bot.Send(doc)
	if err != nil {
		log.Error(err)

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			"😞 "+t.Lang("Something wrong... I will be fixing it")))

		t.App.SendLogToChannel(t.Message.From, "mess", fmt.Sprintf("send file err\n\n %s", err))
	} else {
		var (
			fileIDStr string
			fileSize  int
		)
		if sentDoc.Document != nil {
			fileIDStr = sentDoc.Document.FileID
			fileSize = sentDoc.Document.FileSize
		} else {
			fileIDStr = sentDoc.Audio.FileID
			fileSize = sentDoc.Audio.FileSize
		}
		t.App.SendLogToChannel(t.Message.From, "doc", fmt.Sprintf("doc file - "+t.Torrent.Name), fileIDStr)

		Cache.Add(Cache{Task: &t}, fileIDStr, fileSize, t.Torrent.Name+".torrent")
	}

	stopAction = true
}
