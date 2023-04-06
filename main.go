package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/boltdb/bolt"
)

const (
	bucketName         = "questions_and_answers"
	apiKeyFile         = "openai_api_key.txt"
	openAIURL          = "https://api.openai.com/v1/chat/completions"
	modelID            = "gpt-3.5-turbo"
	defaultTemperature = 0.7
)

type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Config struct {
	APIKey string `json:"api_key"`
}

var showHelp = flag.Bool("help", false, "Show help")
var question = flag.String("question", "", "Question to ask")
var example_prompt = flag.String("example_prompt", "", "Example prompt")
var example_response = flag.String("example_response", "", "Example response")
var file_to_analyze = flag.String("file_to_analyze", "", "Text File to analyze")
var debugMode = flag.Bool("debug", false, "Print all Debug messages")

var logError *log.Logger
var logInfo *log.Logger
var logDebug *log.Logger

var config Config

func main() {
	flag.Parse()

	// Initialize loggers
	logError = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	logInfo = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)

	if *debugMode {
		logDebug = log.New(os.Stdout, "DEBUG\t", log.Ldate|log.Ltime)
	} else {
		logDebug = log.New(ioutil.Discard, "", 0)
	}

	// if parameter is empty display help
	if flag.NFlag() == 0 || *showHelp {
		flag.PrintDefaults()
		os.Exit(0)
	}

	configFile := getConfigDir() + "/config.json"

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		logError.Printf("Config file %s does not exist", configFile)
		os.Exit(1)
	}

	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		logError.Printf("Error reading API key file: %v", err)
	}

	err = json.Unmarshal(file, &config)
	if err != nil {
		logError.Printf("Error unmarshalling API key file: %v", err)
	}

	var text_to_analyze []byte

	if *file_to_analyze != "" {
		if _, err := os.Stat(*file_to_analyze); os.IsNotExist(err) {
			logError.Printf("File %s does not exist", *file_to_analyze)
			os.Exit(1)
		}

		text_to_analyze, err = ioutil.ReadFile(*file_to_analyze)
		if err != nil {
			logError.Printf("Error reading file: %v", err)
			os.Exit(1)
		}
	}

	apiKey := config.APIKey

	dbFile := getConfigDir() + "/qa.db"

	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Fatalf("Error opening BoltDB: %v", err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	})
	if err != nil {
		log.Fatalf("Error creating bucket: %v", err)
	}

	answer, err := getAnswer(apiKey, *question, *example_prompt, *example_response, string(text_to_analyze))
	if err != nil {
		log.Fatalf("Error getting answer: %v", err)
	}

	err = storeQA(db, *question, answer)
	if err != nil {
		log.Fatalf("Error storing question and answer: %v", err)
	}

	fmt.Printf("Answer: %s\n", answer)
}

func getAnswer(apiKey, question string, example_prompt string, example_response string, text_to_analyze string) (string, error) {
	chatReq := ChatCompletionRequest{
		Model: modelID,
		Messages: []Message{
			{Role: "system", Content: "You are a helpful assistant and you give answers in a list. People generally ask about text and books."},
			{Role: "user", Content: example_prompt},
			{Role: "assistant", Content: example_response},
			{Role: "user", Content: "Now, about this following text, " + question + ": " + text_to_analyze},
		},
		Temperature: defaultTemperature,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", openAIURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API request failed with status: %s", resp.Status)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp ChatCompletionResponse
	err = json.Unmarshal(respBody, &chatResp)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

func storeQA(db *bolt.DB, question, answer string) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		qa := &QA{Question: question, Answer: answer}
		data, err := json.Marshal(qa)
		if err != nil {
			return err
		}

		return b.Put([]byte(question), data)
	})
}

func getHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting home directory: %v", err)
	}

	return home
}

func getConfigDir() string {
	homeDir := getHomeDir()
	configDir := filepath.Join(homeDir, ".gogpt")

	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		log.Fatalf("Error creating config directory: %v", err)
	}

	return configDir
}
