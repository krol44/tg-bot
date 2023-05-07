package main

import (
	"github.com/anacrolix/torrent"
	"time"
)

type ObjectHandler interface {
	Download() bool
	Convert() bool
	Send() bool
	Clean()
}

type ObjectVideoUrl struct {
	Task *Task
}

type ObjectTorrent struct {
	Task           *Task
	TorrentProcess *torrent.Torrent
}

type ObjectSpotify struct {
	Task *Task
}

type User struct {
	TelegramID   int64     `db:"telegram_id"`
	DateCreate   time.Time `db:"date_create"`
	Name         string    `db:"name"`
	Premium      int       `db:"premium"`
	SentAd       int       `db:"sent_ad"`
	Block        int       `db:"block"`
	BlockWhy     string    `db:"block_why"`
	LanguageCode string    `db:"language_code"`
}

type CacheRow struct {
	id             int       `db:"id"`
	Caption        string    `db:"caption"`
	NativePathFile string    `db:"native_path_file"`
	NativeMd5Sum   string    `db:"native_md5_sum"`
	VideoUrlId     string    `db:"video_url_id"`
	TgFromID       string    `db:"tg_from_id"`
	TgFileID       string    `db:"tg_file_id"`
	TgFileSize     int       `db:"tg_file_size"`
	DateCreate     time.Time `db:"date_create"`
}

type InfoYtDlp struct {
	ID             string `json:"id"`
	FullTitle      string `json:"fulltitle"`
	FilesizeApprox int    `json:"filesize_approx"`
	Filesize       int    `json:"filesize"`
	Filename       string `json:"_filename"`
	Formats        []struct {
		Ext              string `json:"ext" gorm:"column:ext"`
		Vcodec           string `json:"vcodec" gorm:"column:vcodec"`
		AudioExt         string `json:"audio_ext" gorm:"column:audio_ext"`
		VideoExt         string `json:"video_ext" gorm:"column:video_ext"`
		Preference       int    `json:"preference" gorm:"column:preference"`
		Format           string `json:"format" gorm:"column:format"`
		SourcePreference int    `json:"source_preference" gorm:"column:source_preference"`
		Filesize         int    `json:"filesize" gorm:"column:filesize"`
		DynamicRange     string `json:"dynamic_range" gorm:"column:dynamic_range"`
		Resolution       string `json:"resolution" gorm:"column:resolution"`
		Url              string `json:"url" gorm:"column:url"`
		Protocol         string `json:"protocol" gorm:"column:protocol"`
		FormatNote       string `json:"format_note" gorm:"column:format_note"`
		Acodec           string `json:"acodec" gorm:"column:acodec"`
		Width            int    `json:"width" gorm:"column:width"`
		FormatID         string `json:"format_id" gorm:"column:format_id"`
		Height           int    `json:"height" gorm:"column:height"`
	} `json:"formats" gorm:"column:formats"`
}
