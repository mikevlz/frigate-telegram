package frigate

import (
	"bytes"
	"mime/multipart"
	"path/filepath"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mikevlz/frigate-telegram/internal/config"
	"github.com/mikevlz/frigate-telegram/internal/log"
	"github.com/mikevlz/frigate-telegram/internal/redis"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	baseURL    = "http://10.200.214.251:9090/api/v2" // Update with your Sharry URL
	//username   = os.Getenv("SHARRY_USER")
	//password   = os.Getenv("SHARRY_PASS")
	//shareID    = "your_share_id" // The private share ID you want to publish
)
username   = os.Getenv("SHARRY_USER")
password   = os.Getenv("SHARRY_PASS")
type LoginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Token   string `json:"token"`
	ValidMs int64  `json:"validMs"`
	User    string `json:"user"`
	Admin   bool   `json:"admin"`
	ID      string `json:"id"`
}

type PublishRequest struct {
	ReuseID bool `json:"reuseId"`
}

type ShareDetail struct {
	ID         string `json:"id"`
	PublishInfo struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	} `json:"publishInfo"`
}



type EventsStruct []struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            float64     `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	// SubLabel           []any       `json:"sub_label"`
	Thumbnail          string      `json:"thumbnail"`
	TopScore           interface{} `json:"top_score"`
	Zones              []any       `json:"zones"`
}

type SharryCreateApiResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
    ID      string `json:"id"` // Maps to the "$ident" string in the schema
}

type EventStruct struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            float64     `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	// SubLabel           []any       `json:"sub_label"`
	Thumbnail          string      `json:"thumbnail"`
	TopScore           interface{} `json:"top_score"`
	Zones              []any       `json:"zones"`
}

var Events EventsStruct
var Event EventStruct

func NormalizeTagText(text string) string {
	var alphabetCheck = regexp.MustCompile(`^[A-Za-z]+$`)
	var NormalizedText []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		wordString := fmt.Sprintf("%c", runes[i])
		if _, err := strconv.Atoi(wordString); err == nil {
			NormalizedText = append(NormalizedText, wordString)
		}
		if alphabetCheck.MatchString(wordString) {
			NormalizedText = append(NormalizedText, wordString)
		}
	}
	return strings.Join(NormalizedText, "")
}

func GetTagList(Tags []any) []string {
	var my_tags []string
	for _, zone := range Tags {
		if zone != nil {
			my_tags = append(my_tags, NormalizeTagText(zone.(string)))
		}
	}
	return my_tags
}

func ErrorSend(TextError string, bot *tgbotapi.BotAPI, EventID string) {
	//conf := config.New()
	TextError += "\nEventID: " + EventID
	///_, err := bot.Send(tgbotapi.NewMessage(conf.TelegramChatID, TextError))
	//if err != nil {
	//	log.Error.Println(err.Error())
	//}
	log.Error.Fatalln(TextError)
}

func SaveThumbnail(EventID string, Thumbnail string, bot *tgbotapi.BotAPI) string {
	// Decode string Thumbnail base64
	dec, err := base64.StdEncoding.DecodeString(Thumbnail)
	if err != nil {
		ErrorSend("Error when base64 string decode: "+err.Error(), bot, EventID)
	}

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".jpg"
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), bot, EventID)
	}
	defer f.Close()
	if _, err := f.Write(dec); err != nil {
		ErrorSend("Error when write file: "+err.Error(), bot, EventID)
	}
	if err := f.Sync(); err != nil {
		ErrorSend("Error when sync file: "+err.Error(), bot, EventID)
	}
	return filename
}

func GetEvents(FrigateURL string, bot *tgbotapi.BotAPI, SetBefore bool) EventsStruct {
	conf := config.New()

	FrigateURL = FrigateURL + "?limit=" + strconv.Itoa(conf.FrigateEventLimit)

	if SetBefore {
		timestamp := time.Now().UTC().Unix()
		timestamp = timestamp - int64(conf.EventBeforeSeconds)
		FrigateURL = FrigateURL + "&before=" + strconv.FormatInt(timestamp, 10)
	}

	log.Debug.Println("Geting events from Frigate via URL: " + FrigateURL)

	// Request to Frigate
	resp, err := http.Get(FrigateURL)
	if err != nil {
		ErrorSend("Error get events from Frigate, error: "+err.Error(), bot, "ALL")
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != 200 {
		ErrorSend("Response status != 200, when getting events from Frigate.\nExit.", bot, "ALL")
	}

	// Read data from response
	byteValue, err := io.ReadAll(resp.Body)
	if err != nil {
		ErrorSend("Can't read JSON: "+err.Error(), bot, "ALL")
	}

	// Parse data from JSON to struct
	err1 := json.Unmarshal(byteValue, &Events)
	if err1 != nil {
		ErrorSend("Error unmarshal json: "+err1.Error(), bot, "ALL")
		if e, ok := err.(*json.SyntaxError); ok {
			log.Info.Println("syntax error at byte offset " + strconv.Itoa(int(e.Offset)))
		}
		log.Info.Println("Exit.")
	}

	// Return Events
	return Events
}

func SaveClip(EventID string, bot *tgbotapi.BotAPI) string {
	// Get config
	conf := config.New()

	// Generate clip URL
	ClipURL := conf.FrigateURL + "/api/events/" + EventID + "/clip.mp4"

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".mp4"

	// Create clip file
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), bot, EventID)
	}
	defer f.Close()

	// Download clip file
	resp, err := http.Get(ClipURL)
	if err != nil {
		ErrorSend("Error clip download: "+err.Error(), bot, EventID)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		ErrorSend("Return bad status: "+resp.Status, bot, EventID)
	}

	// Writer the body to file
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		ErrorSend("Error clip write: "+err.Error(), bot, EventID)
	}
	return filename
}

func login() (string, error) {
	loginData := LoginRequest{
		Account:  username,
		Password: password,
	}

	body, _ := json.Marshal(loginData)
	resp, err := http.Post(baseURL+"/open/auth/login", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s", resp.Status)
	}

	var authResp AuthResponse
	err = json.NewDecoder(resp.Body).Decode(&authResp)
	if err != nil {
		return "", err
	}

	if !authResp.Success {
		return "", fmt.Errorf("login failed: %s", authResp.Message)
	}

	return authResp.Token, nil
}

func publishShare(token, shareID string) error {
	publishData := PublishRequest{ReuseID: true}
	body, _ := json.Marshal(publishData)

	req, _ := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/sec/share/%s/publish", baseURL, shareID),
		bytes.NewBuffer(body),
	)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sharry-Auth", token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("publish failed: %s", resp.Status)
	}

	return nil
}

func getPublicID(token, shareID string) (string, error) {
	req, _ := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/sec/share/%s", baseURL, shareID),
		nil,
	)
	req.Header.Set("Sharry-Auth", token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get share details: %s", resp.Status)
	}

	var shareDetail ShareDetail
	err = json.NewDecoder(resp.Body).Decode(&shareDetail)
	if err != nil {
		return "", err
	}

	if !shareDetail.PublishInfo.Enabled || shareDetail.PublishInfo.ID == "" {
		return "", fmt.Errorf("share is not properly published")
	}

	return shareDetail.PublishInfo.ID, nil
}

func SendMessageEvent(FrigateEvent EventStruct, bot *tgbotapi.BotAPI) {
	// Get config
	conf := config.New()

	redis.AddNewEvent(FrigateEvent.ID, "InWork", time.Duration(60)*time.Second)

	// Prepare text message
	text := "*Камера*\n"
	text += "┗ #" + NormalizeTagText(FrigateEvent.Camera) + "\n"
	//text += "┣*Label*\n┗ #" + NormalizeTagText(FrigateEvent.Label) + "\n"
	// if FrigateEvent.SubLabel != nil {
	// 	text += "┣*SubLabel*\n┗ #" + strings.Join(GetTagList(FrigateEvent.SubLabel), ", #") + "\n"
	// }
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Начало события*\n┗ `%s", t_start) + "`\n"
	if FrigateEvent.EndTime == 0 {
		text += "┣*конец события*\n┗ `Еще не закончилось`" + "\n"
	} else {
		t_end := time.Unix(int64(FrigateEvent.EndTime), 0)
		text += fmt.Sprintf("┣*конец события*\n┗ `%s", t_end) + "`\n"
	}
	//text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
	//text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
	
	text += "\n"
	text += "*Область события*\n┗ #" + strings.Join(GetTagList(FrigateEvent.Zones), ", #") + "\n"
	text += "\n"
	//text += "┣[Events](" + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",") + ")\n"
	//text += "┣[General](" + conf.FrigateExternalURL + ")\n"
	text += "[Оригинал записи](" + conf.FrigateExternalURL + "/api/events/" + FrigateEvent.ID + "/clip.mp4)\n"

	// Save thumbnail
	FilePathThumbnail := SaveThumbnail(FrigateEvent.ID, FrigateEvent.Thumbnail, bot)
	defer os.Remove(FilePathThumbnail)

	var medias []interface{}
	MediaThumbnail := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(FilePathThumbnail))
	MediaThumbnail.Caption = text
	MediaThumbnail.ParseMode = tgbotapi.ModeMarkdown
	medias = append(medias, MediaThumbnail)

	if FrigateEvent.HasClip && FrigateEvent.EndTime != 0 {
		// Save clip
		FilePathClip := SaveClip(FrigateEvent.ID, bot)
		defer os.Remove(FilePathClip)

		videoInfo, err := os.Stat(FilePathClip)
		if err != nil {
			ErrorSend("Error receiving information about the clip file: "+err.Error(), bot, FrigateEvent.ID)
		}
		form := new(bytes.Buffer)
		writer := multipart.NewWriter(form)
		//formField, err := writer.CreateFormField("meta")
		//if err != nil {
		//	log.Debug.Println(err)
		//}
		//_, err = formField.Write([]byte(`{"name":"string","validity":0,"description":"string","maxViews":0,"password":""}`))
		//if err != nil {
		//	log.Debug.Println(err)
		//}	
		fw, err := writer.CreateFormFile("file", filepath.Base(FilePathClip))
		if err != nil {
			log.Debug.Println(err)
		}
		fd, err := os.Open(FilePathClip)
		if err != nil {
			log.Debug.Println(err)
		}
		defer fd.Close()
		_, err = io.Copy(fw, fd)
		if err != nil {
			log.Debug.Println(err)
		}
	
		writer.Close()
	
		client := &http.Client{}
		req, err := http.NewRequest("POST", "http://10.200.214.251:9090/api/v2/alias/upload", form)
		if err != nil {
			log.Debug.Println(err)
		}
		req.Header.Set("Sharry-Alias", "6c6NFPiWbGB-RSWgRWWL9z2-uSrbFi7F6AW-SZxMKNUDLKy")
		req.Header.Set("accept", "application/json")
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := client.Do(req)
		if err != nil {
			log.Debug.Println(err)
		}
		defer resp.Body.Close()
		bodyText, err := io.ReadAll(resp.Body)
		if err != nil {
			ErrorSend("Error sending clip file to server: "+err.Error(), bot, FrigateEvent.ID)
		}
		// Unmarshal the JSON into the struct
		var response SharryCreateApiResponse
		err = json.Unmarshal(bodyText, &response)
		if err != nil {
			fmt.Println("Error parsing JSON:", err)
			//return
		}
		
			// Access the "id" field
		fmt.Println("sharry file ID:", response.ID)
		log.Debug.Println(bodyText)
			// 1. Login and get auth token
		token, err := login()
		if err != nil {
			panic(err)
		}

		// 2. Publish the share
		err = publishShare(token, response.ID)
		if err != nil {
			panic(err)
		}

		// 3. Get share details to find public ID
		publicID, err := getPublicID(token, response.ID)
		if err != nil {
			panic(err)
		}

		// 4. Construct public URL
		publicURL := fmt.Sprintf("%s/share/%s", baseURL, publicID)
		fmt.Printf("Public share URL: %s\n", publicURL)
		if videoInfo.Size() < 52428800 {
			// Telegram don't send large file see for more: https://github.com/mikevlz/frigate-telegram/issues/5
			// Add clip to media group
			MediaClip := tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(FilePathClip))
			medias = append(medias, MediaClip)
		}
	}

	// Create message
	msg := tgbotapi.MediaGroupConfig{
		ChatID: conf.TelegramChatID,
		Media:  medias,
	}
	messages, err := bot.SendMediaGroup(msg)
	if err != nil {
		ErrorSend("Error send media group message: "+err.Error(), bot, FrigateEvent.ID)
	}

	if messages == nil {
		ErrorSend("No received messages", bot, FrigateEvent.ID)
	}
	var State string
	State = "InProgress"
	if FrigateEvent.EndTime != 0 {
		State = "Finished"
	}
	redis.AddNewEvent(FrigateEvent.ID, State, time.Duration(conf.RedisTTL)*time.Second)
}

func StringsContains(MyStr string, MySlice []string) bool {
	for _, v := range MySlice {
		if v == MyStr {
			return true
		}
	}
	return false
}

func ParseEvents(FrigateEvents EventsStruct, bot *tgbotapi.BotAPI, WatchDog bool) {
	// Parse events
	conf := config.New()
	RedisKeyPrefix := ""
	if WatchDog {
		RedisKeyPrefix = "WatchDog_"
	}
	for Event := range FrigateEvents {
		if !(len(conf.FrigateExcludeCamera) == 1 && conf.FrigateExcludeCamera[0] == "None") {
			if StringsContains(FrigateEvents[Event].Camera, conf.FrigateExcludeCamera) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if !(len(conf.FrigateIncludeCamera) == 1 && conf.FrigateIncludeCamera[0] == "All") {
			if !(StringsContains(FrigateEvents[Event].Camera, conf.FrigateIncludeCamera)) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if redis.CheckEvent(RedisKeyPrefix + FrigateEvents[Event].ID) {
			if WatchDog {
				SendTextEvent(FrigateEvents[Event], bot)
			} else {
				go SendMessageEvent(FrigateEvents[Event], bot)
			}
		}
	}
}

func SendTextEvent(FrigateEvent EventStruct, bot *tgbotapi.BotAPI) {
	conf := config.New()
	text := "*New event*\n"
	text += "┣*Camera*\n┗ `" + FrigateEvent.Camera + "`\n"
	text += "┣*Label*\n┗ `" + FrigateEvent.Label + "`\n"
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s", t_start) + "`\n"
	text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
	text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
	text += "┣*Zones*\n┗ `" + strings.Join(GetTagList(FrigateEvent.Zones), ", ") + "`\n"
	text += "┣*Event URL*\n┗ " + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",")
	msg := tgbotapi.NewMessage(conf.TelegramChatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := bot.Send(msg)
	if err != nil {
		log.Error.Println(err.Error())
	}
	redis.AddNewEvent("WatchDog_"+FrigateEvent.ID, "Finished", time.Duration(conf.RedisTTL)*time.Second)
}

func NotifyEvents(bot *tgbotapi.BotAPI, FrigateEventsURL string) {
	conf := config.New()
	for {
		FrigateEvents := GetEvents(FrigateEventsURL, bot, false)
		ParseEvents(FrigateEvents, bot, true)
		time.Sleep(time.Duration(conf.WatchDogSleepTime) * time.Second)
	}
}
