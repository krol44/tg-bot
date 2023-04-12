package main

import (
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	log.Info("TorPurrBot is running...")

	app := Run()
	go app.ObserverQueue()

	for update := range app.BotUpdates {
		if update.Message != nil {
			// logs sqlite
			app.Logs(update.Message)

			if update.Message.Text != "" {
				app.SendLogToChannel(update.Message.From.ID, "mess", "Send message: "+update.Message.Text)
			}

			if app.IsBlockUser(update.Message.From.ID) {
				continue
			}

			app.Queue <- struct{ Message *tgbotapi.Message }{Message: update.Message}

		}

		if update.ChannelPost != nil && update.ChannelPost.Chat.ID == config.ChatIdChannelLog {
			sp := strings.Split(update.ChannelPost.Text, " ")

			if len(sp) == 2 && sp[0] == "/premium" {
				var userFromDB User
				_ = app.DB.Get(&userFromDB, "SELECT name, premium, language_code FROM users WHERE telegram_id = ?",
					sp[1])

				tr := Translate{Code: userFromDB.LanguageCode}
				premium := 0
				premiumText := tr.Lang("Premium is disabled") + " 😔"
				if userFromDB.Premium == 0 {
					premium = 1
					premiumText = tr.Lang("Premium is enabled") + " 🎉"
				}
				_, err := app.DB.Exec("UPDATE users SET premium = ? WHERE telegram_id = ?", premium, sp[1])
				if err != nil {
					log.Error(err)
				}

				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(int64(whoId), "mess", premiumText)
					_, _ = app.Bot.Send(tgbotapi.NewMessage(int64(whoId), premiumText))
				}
			}

			if update.ChannelPost.ReplyToMessage != nil {
				regx := regexp.MustCompile(` \((.*?)\) `)
				matches := regx.FindStringSubmatch(update.ChannelPost.ReplyToMessage.Text)

				if len(matches) == 2 {
					replayChatId, _ := strconv.Atoi(matches[1])
					_, err := app.Bot.Send(tgbotapi.NewMessage(int64(replayChatId),
						"Support: "+update.ChannelPost.Text))
					if err != nil {
						log.Error(err)
					}
				}
			}
		}
	}
}
