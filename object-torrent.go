package main

import (
	"context"
	"fmt"
	"github.com/anacrolix/torrent"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"path"
	"strconv"
	"strings"
	"time"
)

func (o *ObjectTorrent) Download() bool {
	qn, _ := o.Task.App.ChatsWork.m.Load(o.Task.Message.MessageID)
	o.Task.App.SendLogToChannel(o.Task.Message.From, "mess",
		fmt.Sprintf("downloading torrent - %s | his turn: %d",
			o.Task.Message.Text, qn.(int)+1))

	o.Task.Alloc("torrent")

	o.Task.Torrent.Process = o.TorrentProcess

	var fileChosen *torrent.File
	for _, val := range o.Task.Torrent.Process.Files() {
		recoveryPath := val.Path() + " ~ " + strconv.FormatInt(val.Length()>>20, 10) + " MB"
		if strings.Contains(recoveryPath, o.Task.Message.Text) {
			if val.Length() > 1999e6 { // more 2 GB
				o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID,
					fmt.Sprintf("😔 "+o.Task.Lang("File is bigger 2 GB"))))
				return false
			}
			val.SetPriority(torrent.PiecePriorityNow)
			fileChosen = val
		} else {
			val.SetPriority(torrent.PiecePriorityNone)
		}
	}

	if fileChosen == nil {
		log.Error("file chosen is empty")
		return false
	}

	// if name not correct
	o.Task.Torrent.Process.SetDisplayName(o.Task.UniqueId("temp-torrent-name"))

	o.Task.Torrent.Process.SetMaxEstablishedConns(200)

	<-o.Task.Torrent.Process.GotInfo()

	o.Task.Torrent.Name = fileChosen.DisplayPath()

	infoText := fmt.Sprintf("🎈 "+o.Task.Lang("Torrent")+": %s", o.Task.Torrent.Name)

	o.Task.Send(tgbotapi.NewMessage(o.Task.Message.Chat.ID, infoText))

	ctx, cancelProgress := context.WithCancel(context.Background())
	go func(ctx context.Context, fileChosen *torrent.File) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if _, bo := o.Task.App.ChatsWork.StopTasks.Load(o.Task.Message.Chat.ID); bo {
					return
				}

				stat, percent := o.Task.StatDlTor(fileChosen)
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
				}(ctx, o.Task)
			}
			time.Sleep(time.Second)
		}
	}(ctx, fileChosen)

	o.Task.Torrent.Process.AllowDataDownload()

	for {
		if _, bo := o.Task.App.ChatsWork.StopTasks.Load(o.Task.Message.Chat.ID); bo {
			break
		}
		if fileChosen.FileInfo().Length == fileChosen.BytesCompleted() {
			break
		}
		time.Sleep(time.Second)
	}

	cancelProgress()

	if _, bo := o.Task.App.ChatsWork.StopTasks.Load(o.Task.Message.Chat.ID); bo {
		o.Task.Torrent.Process.Drop()
		return false
	}

	o.Task.Send(tgbotapi.NewEditMessageText(o.Task.Message.Chat.ID, o.Task.MessageEditID,
		"✅ "+o.Task.Lang("Torrent downloaded, wait next step")))

	pathway := path.Clean(config.DirBot + "/torrent-client/" + fileChosen.Path())

	cache := Cache{Task: o.Task}
	if cache.TrySend("video", pathway) {
		return false
	}
	if cache.TrySendThroughMd5(pathway) {
		return false
	}
	if cache.TrySend("doc", o.Task.Torrent.Name+".torrent") {
		return false
	}

	o.Task.File = pathway

	return true
}

func (o *ObjectTorrent) Convert() bool {
	var c = Convert{Task: o.Task, IsTorrent: true}

	if c.CheckExistVideo() {
		o.Task.FileConverted = c.Run()

		return true
	} else {
		return false
	}
}

func (o *ObjectTorrent) Send() bool {
	if path.Ext(o.Task.File) == ".flac" ||
		path.Ext(o.Task.File) == ".mp3" ||
		path.Ext(o.Task.File) == ".wav" {
		return o.Task.SendAudio()
	} else {
		return o.Task.SendDoc()
	}
}

func (o *ObjectTorrent) Clean() {
	o.Task.RemoveMessageEdit()
}
