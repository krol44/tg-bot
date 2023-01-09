package main

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"time"
)

const signAdvt = "\n\n@TorPurrBot - download and convert\n Torrent, youtube, tiktok, other"

func (t Task) SendVideos(files []FileConverted) {
	for _, v := range files {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("📲 "+t.Lang("Sending video")+" - %s \n\n🍿 "+
				t.Lang("Time upload to the telegram ~ 1-7 minutes"), v.Name)))

		video := tgbotapi.NewVideo(t.Message.Chat.ID,
			tgbotapi.FilePath(v.FilePath))

		video.SupportsStreaming = true
		video.Caption = v.Name + signAdvt
		if t.UserFromDB.Premium == 0 {
			video.ProtectContent = true
		}
		video.Thumb = tgbotapi.FilePath(v.CoverPath)
		video.Width = v.CoverSize.X
		video.Height = v.CoverSize.Y

		stopAction := false
		go func(stopAction *bool) {
			for {
				if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
					return
				}

				if *stopAction == true {
					break
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
				"😞 "+t.Lang("Something wrong... We will be fixing it")))

			t.App.SendLogToChannel(t.Message.From.ID, "mess",
				fmt.Sprintf("video file send err\n\n%s", err))
			return
		} else {
			stopAction = true

			t.App.SendLogToChannel(t.Message.From.ID, "video", fmt.Sprintf("video file - "+v.Name),
				sentVideo.Video.FileID)

			Cache.Add(Cache{Task: &t}, "video", sentVideo.Video.FileID, sentVideo.Video.FileSize, v.FilePathNative)
		}
	}
}

func (t Task) SendTorFiles() {
	cache := Cache{Task: &t}
	if cache.TrySend("doc", t.Torrent.Name+".torrent") {
		return
	}

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
		if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
			return
		}

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("🔥 Ziping - %s", pathway)))

		_, err := os.Stat(pathway)
		if err != nil {
			log.Error(err)
		}

		file, err := os.Open(pathway)
		if err != nil {
			log.Warning(err)
		}

		ctz, err := zipWriter.Create(strings.TrimLeft(pathway, config.DirBot+"/torrent-client/"))
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
			fmt.Sprintf("😞 "+t.Lang("Something wrong... We will be fixing it"))))
		return
	}
	isBigFile := fiCh.Size() > 1999e6 // more 2gb

	if isBigFile {
		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("😔 "+
				t.Lang("Files in the torrent are too big, zip archive size available only no more than 2 gb"))))
	} else {
		if t.UserFromDB.Premium == 0 {
			t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
				fmt.Sprintf("😔 "+t.Lang("File not sent"))))

			return
		}

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
			fmt.Sprintf("📲 "+t.Lang("Sending zip")+" - %s \n\n⏰ "+
				t.Lang("Time upload to the telegram ~ 1-7 minutes"), zipName)))

		doc := tgbotapi.NewDocument(t.Message.Chat.ID,
			tgbotapi.FilePath(pathZip))

		doc.Caption = t.Torrent.Name + signAdvt
		if t.UserFromDB.Premium == 0 {
			doc.ProtectContent = true
		}

		stopAction := false
		go func(stopAction *bool) {
			for {
				if *stopAction == true {
					break
				}

				_, _ = t.App.Bot.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_document"))

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
				"doc", fmt.Sprintf("doc file - "+t.Torrent.Name), sentDoc.Document.FileID)

			cache.Add("doc", sentDoc.Document.FileID, sentDoc.Document.FileSize, t.Torrent.Name+".torrent")
		}

		stopAction = true
	}
}
