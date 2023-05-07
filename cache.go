package main

import (
	"crypto/md5"
	"database/sql"
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

func (c Cache) Add(tgFileId string, tgFileSize int, nativeFilePath string) {
	var md5Sum string
	if file, err := os.ReadFile(nativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	caption := strings.TrimSuffix(path.Base(nativeFilePath), path.Ext(path.Base(nativeFilePath)))

	var urlHttp string
	if c.Task.DescriptionUrl != "" {
		urlHttp = "\n" + c.Task.DescriptionUrl
	}

	if _, isSlice := c.Task.GetTimeSlice(); isSlice {
		nativeFilePath = ""
		md5Sum = ""
		c.Task.UrlIDForCache = "no"
	}

	_, err := Postgres.Exec(`INSERT INTO cache
		(caption, native_path_file, native_md5_sum, video_url_id, tg_from_id, tg_file_id, tg_file_size, date_create)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		caption+urlHttp, nativeFilePath, md5Sum, c.Task.UrlIDForCache, c.Task.Message.From.ID,
		tgFileId, tgFileSize, time.Now())
	if err != nil {
		log.Error(err)
	}
}

func (c Cache) TrySend(typeSome string, pathway string) bool {
	var row CacheRow
	err := Postgres.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_path_file = $1 ORDER BY id DESC", pathway)
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
		c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
			"video sent from cache - "+row.Caption)
	}
	if typeSome == "doc" {
		sob := tgbotapi.NewDocument(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
		sob.Caption = row.Caption + signAdvt
		_, err := c.Task.App.Bot.Send(sob)
		if err != nil {
			return false
		}

		c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
			"doc sent from cache - "+row.Caption)
	}

	return true
}

func (c Cache) GetFileIdThroughMd5(NativeFilePath string) string {
	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	if md5Sum == "" {
		return ""
	}

	var row CacheRow
	err := Postgres.Get(&row,
		"SELECT tg_file_id FROM cache WHERE native_md5_sum = $1 ORDER BY id DESC", md5Sum)
	if err != nil {
		return ""
	}

	return row.TgFileID
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
	err := Postgres.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_md5_sum = $1 ORDER BY id DESC", md5Sum)
	if err != nil {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
		"video sent from cache md5 - "+row.Caption)

	return true
}

func (c Cache) TrySendThroughID() bool {
	var row CacheRow
	err := Postgres.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE video_url_id = $1 ORDER BY id DESC", c.Task.UrlIDForCache)
	if err != nil && err != sql.ErrNoRows {
		log.Error(err)
		return false
	}

	if row.TgFileID == "" {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		log.Error(err)
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
		"video sent from cache video url id - "+row.Caption)

	return true
}
