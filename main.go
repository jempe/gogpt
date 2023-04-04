package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/boltdb/bolt"
)

const (
	bucketName    = "questions_and_answers"
	apiKeyFile    = "openai_api_key.txt"
	openAIURL     = "https://api.openai.com/v1/engines/davinci-codex/completions"
	modelEndpoint = "davinci-codex"
)

type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type CompletionRequest struct {
	Prompt           string  `json:"prompt"`
	MaxTokens        int     `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	TopP             float64 `json:"top_p"`
	FrequencyPenalty float64 `json:"frequency_penalty"`
	PresencePenalty  float64 `json:"presence_penalty"`
}

type CompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go \"Your question\"")
		return
	}

	question := strings.Join(os.Args[1:], " ")

	apiKey, err := ioutil.ReadFile(apiKeyFile)
	if err != nil {
		log.Fatalf("Error reading API key file: %v", err)
	}

	db, err := bolt.Open("qa.db", 0600, nil)
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

	answer, err := getAnswer(string(apiKey), question)
	if err != nil {
		log.Fatalf("Error getting answer: %v", err)
	}

	err = storeQA(db, question, answer)
	if err != nil {
		log.Fatalf("Error storing question and answer: %v", err)
	}

	fmt.Printf("Answer: %s\n", answer)
}

func getAnswer(apiKey, question string) (string, error) {
	prompt := fmt.Sprintf("Please answer the following question: %s", question)
	compReq := CompletionRequest{
		Prompt:           prompt,
		MaxTokens:        50,
		Temperature:      0.5,
		TopP:             1,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
	}

	reqBody, err := json.Marshal(compReq)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/completions", openAIURL, modelEndpoint), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
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

	var compResp CompletionResponse
	err = json.Unmarshal(respBody, &compResp)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(compResp.Choices[0].Text), nil
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
