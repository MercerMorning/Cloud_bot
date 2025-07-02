package main

import (
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	bot          *tgbotapi.BotAPI
	chatIDs      = make(map[int64]bool) // Хранит ID чатов, куда нужно отправлять уведомления
	chatIDsMutex = &sync.Mutex{}        // Мьютекс для безопасного доступа к chatIDs
)

const (
	apiURL         = "https://4cloud.pro/api.php?method=get-consoles-status"
	errorResponse  = `[{"Status": "Error"}]`
	checkInterval  = 10 * time.Second
	configFileName = "chat_ids.json" // Файл для сохранения chat IDs
)

func main() {
	var err error
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable not set")
	}

	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Загружаем сохранённые chat IDs
	loadChatIDs()

	// Запускаем проверку статуса в фоне
	go checkStatusPeriodically()

	// Настраиваем обработчик сообщений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		msgText := update.Message.Text

		if msgText == "/start" {
			// Добавляем чат в список для уведомлений
			addChatID(chatID)
			saveChatIDs()

			msg := tgbotapi.NewMessage(chatID, "Теперь вы будете получать уведомления о статусе консолей.")
			bot.Send(msg)
		} else if msgText == "/stop" {
			// Удаляем чат из списка для уведомлений
			removeChatID(chatID)
			saveChatIDs()

			msg := tgbotapi.NewMessage(chatID, "Вы больше не будете получать уведомления о статусе консолей.")
			bot.Send(msg)
		}
	}
}

func checkStatusPeriodically() {
	for {
		status, err := getAPIStatus()
		if err != nil {
			log.Printf("Error getting status: %v", err)
			time.Sleep(checkInterval)
			continue
		}

		if status != errorResponse {
			notifyChats(status)
		}

		time.Sleep(checkInterval)
	}
}

func getAPIStatus() (string, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Нормализуем JSON для сравнения
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	normalized, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(normalized), nil
}

func notifyChats(status string) {
	chatIDsMutex.Lock()
	defer chatIDsMutex.Unlock()

	for chatID := range chatIDs {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Статус изменился:\n%s", status))
		_, err := bot.Send(msg)
		if err != nil {
			log.Printf("Error sending message to chat %d: %v", chatID, err)
		}
	}
}

func addChatID(chatID int64) {
	chatIDsMutex.Lock()
	defer chatIDsMutex.Unlock()
	chatIDs[chatID] = true
}

func removeChatID(chatID int64) {
	chatIDsMutex.Lock()
	defer chatIDsMutex.Unlock()
	delete(chatIDs, chatID)
}

func saveChatIDs() {
	chatIDsMutex.Lock()
	defer chatIDsMutex.Unlock()

	data, err := json.Marshal(chatIDs)
	if err != nil {
		log.Printf("Error marshaling chat IDs: %v", err)
		return
	}

	err = ioutil.WriteFile(configFileName, data, 0644)
	if err != nil {
		log.Printf("Error saving chat IDs to file: %v", err)
	}
}

func loadChatIDs() {
	data, err := ioutil.ReadFile(configFileName)
	if err != nil {
		if os.IsNotExist(err) {
			return // Файл ещё не создан
		}
		log.Printf("Error reading chat IDs file: %v", err)
		return
	}

	var loadedChatIDs map[int64]bool
	err = json.Unmarshal(data, &loadedChatIDs)
	if err != nil {
		log.Printf("Error unmarshaling chat IDs: %v", err)
		return
	}

	chatIDsMutex.Lock()
	defer chatIDsMutex.Unlock()
	for id, val := range loadedChatIDs {
		chatIDs[id] = val
	}
}
