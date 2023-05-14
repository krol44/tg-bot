package main

type Translate struct {
	Code string
}

func (t *Translate) Lang(str string) string {
	storage := map[string]map[string]string{
		"What is happened? Write me right here": {
			"ru": "Что случилось? Напиши мне прямо здесь",
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
		"Just send me torrent file with the video files or files": {
			"ru": "Просто отправь мне торрент файл с видео файлами или файлами",
		},
		"And also you can send me": {
			"ru": "А также вы можете прислать мне",
		},
		"Or send me YouTube, TikTok url, examples below": {
			"ru": "Или отправь мне YouTube, TikTok url, примеры ниже",
		},
		"Example": {
			"ru": "Пример",
		},
		"Video url is bad": {
			"ru": "Видео url плохой",
		},
		"Audio url is bad": {
			"ru": "Аудио url плохой",
		},
		"Download progress": {
			"ru": "Прогресс скачивания",
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
		"Something wrong... I will be fixing it": {
			"ru": "Что-то случилось, буду чинить",
		},
		"Sending audio": {
			"ru": "Отправка аудио",
		},
		"Sending doc": {
			"ru": "Отправка файла",
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
		"Getting data from torrent, please wait": {
			"ru": "Получение данных из торрента, ожидайте",
		},
		"No data in the torrent file or magnet link, no seeds to get info": {
			"ru": "Нет данных в торрент файле или magnet link, нет сидиров, чтобы получить информацию",
		},
		"Not allowed url, I support only": {
			"ru": "Недопустимый url, поддерживаю только",
		},
		"I am so appreciative of you for using bot! %s Please, share below a message with your friends. Thank you!": {
			"ru": "Я так благодарен тебе за использование бота!" +
				" %s Пожалуйста, поделитесь приведенным ниже сообщением со своими друзьями. Спасибо!",
		},
		"limit exceeded, try again in 24 hours": {
			"ru": "лимит превышен, повторите попытку через 24 часа",
		},
		"Didn't have time to download, maximum 30 minutes": {
			"ru": "Не успел скачать, максимум 30 минут",
		},
		"Support me and get unlimited": {
			"ru": "Поддержи меня и получи безлимит",
		},
	}

	if re, ok := storage[str][t.Code]; ok {
		return re
	}

	return str
}
