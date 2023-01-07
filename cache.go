package main

import (
	"crypto/md5"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"strings"
	"time"
)

type Cache struct {
	Task *Task
}
type CacheRow struct {
	id             int    `db:"id"`
	Caption        string `db:"caption"`
	NativePathFile string `db:"native_path_file"`
	NativeMd5Sum   string `db:"native_md_5_sum"`
	VideoUrlId     string `db:"video_url_id"`
	TgFromID       string `db:"tg_from_id"`
	TgFileID       string `db:"tg_file_id"`
	TgFileSize     int    `db:"tg_file_size"`
	DateCreate     string `db:"date_create"`
}

func (c Cache) Add(typeCache string, tgFileId string, tgFileSize int, NativeFilePath string) {
	if typeCache == "video" {
		timeTotal := Convert.TimeTotalRaw(Convert{}, NativeFilePath)
		s := timeTotal.Sub(time.Date(0000, 01, 01, 00, 00, 00, 0, time.UTC)).Seconds()
		if c.Task.UserFromDB.Premium == 0 && s > 280 {
			return
		}
	}

	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	caption := strings.TrimSuffix(path.Base(NativeFilePath), path.Ext(path.Base(NativeFilePath)))

	_, err := c.Task.App.DB.Exec(`INSERT INTO cache
		(caption, native_path_file, native_md5_sum, video_url_id, tg_from_id, tg_file_id, tg_file_size, date_create)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		caption, NativeFilePath, md5Sum, c.Task.VideoUrlID, c.Task.Message.From.ID, tgFileId, tgFileSize)
	if err != nil {
		log.Error(err)
	}
}

func (c Cache) TrySend(typeSome string, pathway string) bool {
	var row CacheRow
	err := c.Task.App.DB.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_path_file = ? ORDER BY id DESC", pathway)
	if err != nil {
		return false
	}

	if typeSome == "video" {
		sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
		sob.Caption = row.Caption + signAdvt

		_, err := c.Task.App.Bot.Send(sob)
		if err != nil {
			return false
		}
		c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess",
			"video sent from cache - "+row.Caption)
	}
	if typeSome == "doc" {
		sob := tgbotapi.NewDocument(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
		sob.Caption = row.Caption + signAdvt
		_, err := c.Task.App.Bot.Send(sob)
		if err != nil {
			return false
		}

		c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess",
			"doc sent from cache - "+row.Caption)
	}

	return true
}

func (c Cache) TrySendThroughMd5(NativeFilePath string) bool {
	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	if md5Sum == "" {
		return false
	}

	var row CacheRow
	err := c.Task.App.DB.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_md5_sum = ? ORDER BY id DESC", md5Sum)
	if err != nil {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess",
		"video sent from cache md5 - "+row.Caption)

	return true
}

func (c Cache) TrySendThroughId() bool {
	var row CacheRow
	err := c.Task.App.DB.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE video_url_id = ? ORDER BY id DESC", c.Task.VideoUrlID)
	if err != nil {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From.ID, "mess",
		"video sent from cache video url id - "+row.Caption)

	return true
}
