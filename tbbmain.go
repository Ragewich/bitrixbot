package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	//"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var birtixWebhook = "http:***************************"
var awaitingTaskID map[int64]bool
var awaitingComment map[int64]int

func getStorageList(bitrixWebhook string) {
	bitrixURL := fmt.Sprintf("%s/disk.storage.getlist.json", bitrixWebhook)

	resp, err := http.Get(bitrixURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.Reader(resp.Body))
	bodyString := string(bodyBytes)
	log.Println(bodyString)

	var result map[string]interface{}
	json.Unmarshal(bodyBytes, &result)

	if result == nil {
		log.Panic("Received nil response from Bitrix24")
	}

	// Выводим список хранилищ
	for _, storage := range result["result"].([]interface{}) {
		storageMap := storage.(map[string]interface{})
		log.Printf("ID: %s, NAME: %s", storageMap["ID"], storageMap["NAME"])
	}
}
func uploadFileToBitrix(fileContent, bitrixWebhook string) string {
	// Проверяем размер файла
	maxSize := 10 * 1024 * 1024 // 10 MB
	if len(fileContent) > maxSize {
		log.Printf("File is too large: %d bytes (max size is %d bytes)", len(fileContent), maxSize)
		return ""
	}

	bitrixURL := fmt.Sprintf("%s/disk.storage.uploadfile.json", bitrixWebhook)
	storageID := 15 // Замените на ваш folderId

	data := url.Values{}
	data.Set("id", strconv.Itoa(storageID))
	data.Set("data", `{"NAME": "file_11.jpg"}`)
	data.Set("fileContent", fileContent)

	client := &http.Client{}
	r, _ := http.NewRequest("POST", bitrixURL, strings.NewReader(data.Encode())) // URL-encoded payload
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(r)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Читаем ответ сервера и извлекаем fileId
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)
	log.Println(bodyString)

	var result map[string]interface{}
	json.Unmarshal(bodyBytes, &result)

	if result == nil {
		log.Panic("Received nil response from Bitrix24")
	}
	fileId := result["result"].(map[string]interface{})["ID"].(string)

	log.Println(fileId)
	return fileId
}

func sendBitrixRequest(taskID int, message string, fileContent string, bitrixWebhook string) {
	bitrixURL := fmt.Sprintf("%s/task.commentitem.add.json?TASKID=%d", bitrixWebhook, taskID)

	data := url.Values{}
	data.Set("FIELDS[POST_MESSAGE]", message)

	// Если fileContent не пустой, добавьте его в данные запроса
	if fileContent != "" {
		data.Set("FIELDS[UF_TASK_WEBDAV_FILES][0]", "photo00000.jpg")
		data.Set("FIELDS[UF_TASK_WEBDAV_FILES][1]", fileContent)
	}

	client := &http.Client{}
	r, err := http.NewRequest("POST", bitrixURL, strings.NewReader(data.Encode())) // URL-encoded payload
	if err != nil {
		log.Printf("Error creating new request: %v", err)
		return
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(r)
	if err != nil {
		log.Printf("Error sending request to Bitrix: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Received non-OK response from Bitrix: %s", resp.Status)
		return
	}

	// Выводим ответ сервера
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}
	log.Printf("Response from Bitrix: %s", string(body))
}

// Define the missing function
func downloadPhoto(bot *tgbotapi.BotAPI, photo *tgbotapi.PhotoSize) []byte {
	// Получаем URL файла
	fileURL, err := bot.GetFileDirectURL(photo.FileID)
	if err != nil {
		log.Panic(err)
	}

	// Делаем HTTP GET запрос к этому URL
	resp, err := http.Get(fileURL)
	if err != nil {
		log.Panic(err)
	}
	defer resp.Body.Close()

	// Читаем ответ в байты
	fileBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Panic(err)
	}

	return fileBytes
}
func main() {
	awaitingTaskID = make(map[int64]bool)
	awaitingComment = make(map[int64]int)

	bot, err := tgbotapi.NewBotAPI("*********************************")
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, _ := bot.GetUpdatesChan(u)

	helloButton := tgbotapi.NewInlineKeyboardButtonData("Hello", "hello")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			helloButton,
		),
	)
	for update := range updates {
		if update.Message != nil {
			var base64Photo string
			if update.Message.Photo != nil {
				// Если сообщение содержит фотографию, загрузите фотографию и преобразуйте ее в base64
				photo := (*update.Message.Photo)[len(*update.Message.Photo)-1]
				fileBytes := downloadPhoto(bot, &photo)
				base64Photo = base64.StdEncoding.EncodeToString(fileBytes)
			}

			// Если пользователь уже ввел taskId, отправьте комментарий2
			if taskID, ok := awaitingComment[update.Message.Chat.ID]; ok {
				if base64Photo != "" {
					// Загружаем фотографию в Bitrix24 и получаем fileId
					fileId := uploadFileToBitrix(base64Photo, birtixWebhook)

					// Используем fileId в параметре UF_TASK_WEBDAV_FILES
					sendBitrixRequest(taskID, update.Message.Text, fileId, birtixWebhook)
				} else {
					sendBitrixRequest(taskID, update.Message.Text, "", birtixWebhook)
				}
				delete(awaitingComment, update.Message.Chat.ID)
			} else if update.Message.Text == "/menu" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Menu:")
				msg.ReplyMarkup = keyboard
				bot.Send(msg)
				getStorageList(birtixWebhook)
			} else if awaitingTaskID[update.Message.Chat.ID] {
				// This user is awaiting a taskId, so treat their message as a taskId
				taskID, err := strconv.Atoi(update.Message.Text)
				if err != nil {
					// The user did not enter a valid number
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "That's not a valid taskId. Please enter a number.")
					bot.Send(msg)
				} else {
					// We have a valid taskId, so ask for the comment text
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Please enter the comment text.")
					bot.Send(msg)

					// Store the taskId and set this user as awaiting a comment
					awaitingComment[update.Message.Chat.ID] = taskID
					delete(awaitingTaskID, update.Message.Chat.ID)
				}
			}
		} else if update.CallbackQuery != nil && update.CallbackQuery.Data == "hello" {
			// Send a message asking for the taskId
			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Please enter the taskId.")
			resp, err := bot.Send(msg)
			if err != nil {
				log.Printf("Error sending message to user: %v", err)
			} else {
				log.Printf("Message sent successfully, response: %v", resp)
			}
			awaitingTaskID[update.CallbackQuery.Message.Chat.ID] = true
		}
	}
}
