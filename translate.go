package main

type Translate struct {
	Code string
}

func (t *Translate) Lang(str string) string {
	storage := map[string]map[string]string{
		"Write a message right here": {
			"ru": "Напишите сообщение прямо в бота",
		},
		"Premium is disabled": {
			"ru": "Премиум выключен",
		},
		"Premium is enabled": {
			"ru": "Премиум включен",
		},
		"Task stopped": {
			"ru": "Задача остановлена",
		},
		"Allowed only one task": {
			"ru": "Разрешена только одна задача",
		},
		"Just send me torrent file with the video files": {
			"ru": "Просто отправь мне торрент файл с видео файлами",
		},
		"Or send me youtube, tiktok url": {
			"ru": "Или отправь мне youtube, tiktok url",
		},
		"Example": {
			"ru": "Пример",
		},
		"Video url is bad": {
			"ru": "Видео url плохой",
		},
		"Video": {
			"ru": "Видео",
		},
		"Download progress": {
			"ru": "Прогресс скачивания",
		},
		"Torrent": {
			"ru": "Торрент",
		},
		"Torrent downloaded, wait next step": {
			"ru": "Торрент скачен, ожидайте следующий шаг",
		},
		"Progress": {
			"ru": "Прогресс",
		},
		"Speed": {
			"ru": "Скорость",
		},
		"Download is starting soon": {
			"ru": "Скачивание скоро начнется",
		},
		"Your queue": {
			"ru": "Ваша очередь",
		},
		"Only the first 5 minutes video is available and torrent in the zip archive don't available": {
			"ru": "Только первые 5 минут видео доступны, торрент файлы в zip архиве недоступны",
		},
		"To donate, for to improve the bot": {
			"ru": "Пожертвовать, чтобы улучшить бота",
		},
		"Write your telegram username in the body message. After donation, you will get full access for 30 days": {
			"ru": "Напишите telegram имя в тело сообщения. После пожертвования, Вы получите полный доступ на 30 дней",
		},
		"Convert is starting": {
			"ru": "Конвертируем",
		},
		"Video is bad": {
			"ru": "Плохое видео",
		},
		"Convert progress": {
			"ru": "Прогресс конверта",
		},
		"Sending video": {
			"ru": "Отправка видео",
		},
		"Time upload to the telegram ~ 1-7 minutes": {
			"ru": "Время загрузки в телеграм ~ 1-7 минут",
		},
		"Something wrong... We will be fixing it": {
			"ru": "Что-то случилось, мы уже знаем и будем чинить",
		},
		"Sending...": {
			"ru": "Отправка...",
		},
		"Available only for users who support us": {
			"ru": "Доступно только для пользователей, которые поддерживают нас",
		},
		"Choose a file, max size 2 GB": {
			"ru": "Выберите файл, максимальный размер 2 GB",
		},
		"File is bigger 2 GB": {
			"ru": "Файл больше 2 GB",
		},
		"Video format is not supported. Only": {
			"ru": "Видео формат не поддерживается. Только",
		},
	}

	if re, ok := storage[str][t.Code]; ok {
		return re
	}

	return str
}
