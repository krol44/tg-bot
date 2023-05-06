package main

import (
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"strings"
	"time"
)

const signAdvt = "\n\n@TorPurrBot - Download Torrent, YouTube, Spotify, TikTok, Other"

func (t *Task) SendVideo() bool {
	file := t.FileConverted
	if file.FilePath == "" {
		return false
	}

	fileInfo, err := os.Stat(file.FilePath)
	if err != nil {
		log.Error(err)
	}
	if fileInfo.Size() > 1999e6 {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("File is bigger 2 GB")))
		t.App.SendLogToChannel(t.Message.From, "mess", "File is bigger 2 GB")
		return false
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		fmt.Sprintf("📲 "+t.Lang("Sending video")+" - %s \n\n🍿 "+
			t.Lang("Time upload to the telegram ~ 1-7 minutes"),
			file.Name)))
	t.App.SendLogToChannel(t.Message.From, "mess", "sending video")

	video := tgbotapi.NewVideo(t.Message.Chat.ID,
		tgbotapi.FilePath(file.FilePath))

	var urlHttp string
	if t.DescriptionUrl != "" {
		urlHttp = "\n" + t.DescriptionUrl
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

		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "😞 "+t.Lang("Something wrong... I will be fixing it")))

		t.App.SendLogToChannel(t.Message.From, "mess",
			fmt.Sprintf("video file send err\n\n%s", err))
		return false
	} else {
		stopAction = true

		t.App.SendLogToChannel(t.Message.From, "video", fmt.Sprintf("video file - "+file.Name),
			sentVideo.Video.FileID)

		Cache.Add(Cache{Task: t}, sentVideo.Video.FileID, sentVideo.Video.FileSize, file.FilePathNative)
	}

	return true
}

func (t *Task) SendDoc() bool {
	if t.File == "" {
		return false
	}

	fileInfo, err := os.Stat(t.File)
	if err != nil {
		log.Error(err)
	}
	if fileInfo.Size() > 1999e6 {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "❗️ "+t.Lang("File is bigger 2 GB")))
		t.App.SendLogToChannel(t.Message.From, "mess", "File is bigger 2 GB")
		return false
	}

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID,
		fmt.Sprintf("📲 "+t.Lang("Sending doc")+" - %s \n\n⏰ "+
			t.Lang("Time upload to the telegram ~ 1-7 minutes"), t.Torrent.Name)))
	t.App.SendLogToChannel(t.Message.From, "mess", "sending doc")

	doc := tgbotapi.NewDocument(t.Message.Chat.ID, tgbotapi.FilePath(t.File))
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

		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "😞 "+t.Lang("Something wrong... I will be fixing it")))

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
		t.App.SendLogToChannel(t.Message.From, "doc",
			fmt.Sprintf("doc file - "+t.Torrent.Name), fileIDStr)

		Cache.Add(Cache{Task: t}, fileIDStr, fileSize, t.File)
	}

	stopAction = true

	return true
}

func chunkSlice[T any](items []T, chunkSize int) (chunks [][]T) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}
	return append(chunks, items)
}

type CacheLog struct {
	From      *tgbotapi.User
	FileID    string
	FromCache bool
}

func (t *Task) SendAudio() bool {
	if len(t.Files) == 0 {
		return false
	}

	var logs []CacheLog

	t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, "📲 "+t.Lang("Sending audio")+"\n\n⏰ "+
		t.Lang("Time upload to the telegram ~ 1-7 minutes")))
	t.App.SendLogToChannel(t.Message.From, "mess", "sending audio")

	stopAction := false
	go func(stopAction *bool) {
		for {
			if *stopAction == true {
				return
			}

			_, _ = t.App.Bot.Send(tgbotapi.NewChatAction(t.Message.Chat.ID, "upload_audio"))

			time.Sleep(4 * time.Second)
		}
	}(&stopAction)

	for _, chuck := range chunkSlice(t.Files, 10) {
		var filesNameForCache []string
		var files []interface{}

		for i, val := range chuck {
			fileID := Cache.GetFileIdThroughMd5(Cache{Task: t}, val)
			name := strings.TrimSuffix(path.Base(val), path.Ext(path.Base(val)))
			if fileID != "" {
				log.Debug("add from cache")
				tgFileID := tgbotapi.NewInputMediaAudio(tgbotapi.FileID(fileID))
				tgFileID.Caption = name + "\n"
				if i == len(chuck)-1 {
					tgFileID.Caption += "\n" + t.DescriptionUrl + signAdvt
				}
				files = append(files, tgFileID)
			} else {
				log.Debug("add from file")
				tgFilePath := tgbotapi.NewInputMediaAudio(tgbotapi.FilePath(val))
				tgFilePath.Caption = name + "\n"
				if i == len(chuck)-1 {
					tgFilePath.Caption = "\n" + t.DescriptionUrl + signAdvt
				}
				files = append(files, tgFilePath)
				filesNameForCache = append(filesNameForCache, val)
			}
		}

		sentAudio, err := t.App.Bot.SendMediaGroup(tgbotapi.NewMediaGroup(t.Message.Chat.ID, files))
		if err != nil {
			log.Error(err)

			t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "😞 "+t.Lang("Something wrong... I will be fixing it")))

			t.App.SendLogToChannel(t.Message.From, "mess", fmt.Sprintf("send audio err\n\n %s", err))
		} else {
			for _, st := range sentAudio {
				var pathForSave string
				for _, fn := range filesNameForCache {
					if strings.Contains(fn, st.Audio.FileName) {
						pathForSave = fn
					}
				}

				fileIDStr := st.Audio.FileID
				fileSize := st.Audio.FileSize
				if pathForSave != "" {
					Cache.Add(Cache{Task: t}, fileIDStr, fileSize, pathForSave)
					logs = append(logs, CacheLog{From: t.Message.From, FileID: fileIDStr, FromCache: false})
				} else {
					logs = append(logs, CacheLog{From: t.Message.From, FileID: fileIDStr, FromCache: true})
				}
			}
		}
	}

	stopAction = true

	go func() {
		for _, s := range logs {
			var ic string
			if s.FromCache {
				ic = " from cache"
			}
			t.App.SendLogToChannel(s.From, "doc", "audio file"+ic, s.FileID)
			time.Sleep(time.Second * 2)
		}
	}()

	return true
}
