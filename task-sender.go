package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"time"
)

func (t Task) SendVideos(files []FileConverted) {
	for _, v := range files {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("📲 Sending video - %s \n\n🍿 Time upload to the telegram ~ 1-7 minutes",
				v.Name)))

		video := tgbotapi.NewVideo(t.Message.Chat.ID,
			tgbotapi.FilePath(v.FilePath))

		video.SupportsStreaming = true
		video.Caption = v.Name
		if t.UserFromDB.Premium == 0 {
			video.ProtectContent = true
		}
		video.Thumb = tgbotapi.FilePath(v.CoverPath)
		video.Width = v.CoverSize.X
		video.Height = v.CoverSize.Y

		stopAction := false
		go func(stopAction *bool) {
			for {
				if *stopAction == true {
					break
				}

				t.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_video"))

				time.Sleep(4 * time.Second)
			}
		}(&stopAction)

		sentVideo, err := t.App.Bot.Send(video)
		if err != nil {
			stopAction = true
			log.Error(err)

			t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
				"😞 Something wrong... We will be fixing it"))

			t.App.SendLogToChannel(t.Message.From.ID, "mess",
				fmt.Sprintf("video file send err\n\n%s", err))
			return
		} else {
			stopAction = true
			t.App.SendLogToChannel(t.Message.From.ID, "video", fmt.Sprintf("video file"),
				sentVideo.Video.FileID)
		}
	}

	t.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, t.MessageEditID))
}

func (t Task) SendTorFiles() {
	zipName := t.UniqueId(t.Torrent.Name) + ".zip"
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

	for _, pathway := range t.Files {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("🔥 Ziping - %s", pathway)))

		pathToZip := config.DirBot + "/torrent-client/" + pathway
		_, err := os.Stat(pathToZip)
		if err != nil {
			log.Error(err)
		}

		file, err := os.Open(pathToZip)
		if err != nil {
			log.Warning(err)
		}

		ctz, err := zipWriter.Create(pathway)
		if err != nil {
			log.Warning(err)
		}
		if _, err := io.Copy(ctz, file); err != nil {
			log.Warning(err)
		}

		file.Close()
	}

	zipWriter.Close()
	archive.Close()

	fiCh, err := os.Stat(pathZip)
	if err != nil {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("😞 Something wrong... We will be fixing it")))
		return
	}
	isBigFile := fiCh.Size() > 1999e6 // more 2gb

	if isBigFile {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("😔 Files in the torrent are too big, zip archive size available only no more than 2 gb")))
	} else {
		if t.UserFromDB.Premium == 0 {
			t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
				fmt.Sprintf("😔 You don't have a donation, file will not be sent")))

			return
		}

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("📲 Sending zip - %s \n\n⏰ Time upload to the telegram ~ 1-7 minutes",
				zipName)))

		doc := tgbotapi.NewDocument(t.Message.Chat.ID,
			tgbotapi.FilePath(pathZip))

		doc.Caption = t.Torrent.Name
		if t.UserFromDB.Premium == 0 {
			doc.ProtectContent = true
		}

		stopAction := false
		go func(stopAction *bool) {
			for {
				if *stopAction == true {
					break
				}

				t.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_document"))

				time.Sleep(4 * time.Second)
			}
		}(&stopAction)

		sentDoc, err := t.App.Bot.Send(doc)
		if err != nil {
			log.Error(err)
			t.App.SendLogToChannel(t.Message.From.ID,
				"mess", fmt.Sprintf("zip file send err\n\n%s", err))
		} else {
			t.App.SendLogToChannel(t.Message.From.ID,
				"doc", fmt.Sprintf("doc file"), sentDoc.Document.FileID)
		}

		stopAction = true

		t.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, t.MessageEditID))
	}
}
