package main

import (
	"fmt"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

func main() {
	app := Run()
	go app.ObserverQueue()

	for update := range app.BotUpdates {
		if update.Message != nil {
			// Logs sqlite
			app.Logs(update.Message)

			if update.Message.Text == "/start" {
				app.InitUser(update.Message)
			}

			// long time
			if (update.Message.Document != nil && update.Message.Document.MimeType == "application/x-bittorrent") ||
				strings.Contains(update.Message.Text, "youtube.com") ||
				strings.Contains(update.Message.Text, "youtu.be") ||
				strings.Contains(update.Message.Text, "tiktok.com") {
				app.Queue <- struct{ Message *tgbotapi.Message }{Message: update.Message}
			}
		}

		if update.ChannelPost != nil && update.ChannelPost.Chat.ID == config.ChatIdChannelLog {
			sp := strings.Split(update.ChannelPost.Text, " ")

			if len(sp) == 2 && sp[0] == "/premium" {
				user := struct {
					Name    string `db:"name"`
					Premium int    `db:"premium"`
				}{}
				_ = app.DB.Get(&user, "SELECT name, premium FROM users WHERE telegram_id = ?", sp[1])

				premium := 0
				premiumText := "disabled"
				if user.Premium == 0 {
					premium = 1
					premiumText = "enabled"
				}
				_, err := app.DB.Exec("UPDATE users SET premium = ? WHERE telegram_id = ?", premium, sp[1])
				if err != nil {
					log.Error(err)
				}

				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(int64(whoId), "mess",
						fmt.Sprintf("premium is %s", premiumText))
				}
			}
		}
	}
}
