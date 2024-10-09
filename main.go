package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// Config stores the API key and other configuration details
type Config struct {
	APIKey       string `yaml:"api_key"`
	Model        string `yaml:"model"`
	SystemPrompt string `yaml:"system_prompt"`
	ShellPrompt  string `yaml:"shell_prompt"`
}

// RequestPayload defines the structure of the JSON payload for the request
type RequestPayload struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	System    string    `json:"system"`
}

// Message defines the structure for messages within the payload
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ANSI color codes
const (
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	Reset  = "\033[0m"
)

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config path cannot be empty")
	}

	config := &Config{}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is missing in the config file")
	}

	return config, nil
}

// SaveConfig saves the API key to a YAML file
func SaveConfig(path string, config *Config) error {
	if path == "" {
		return fmt.Errorf("config path cannot be empty")
	}

	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	encoder := yaml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

func colorizeText(text string) string {
	re := regexp.MustCompile(`<(\w+)>(.*?)<\/\\1>`) // Corrected escape sequence
	return re.ReplaceAllStringFunc(text, func(m string) string {
		match := re.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		tag, content := match[1], match[2]
		switch tag {
		case "cmd":
			return Cyan + content + Reset
		case "error":
			return Red + content + Reset
		default:
			return content // No color if no specific case matched
		}
	})
}

// StreamResponse sends a prompt to the Anthropic API and prints the response text as it arrives
func StreamResponse(apiKey string, model string, systemPrompt string, messageHistory *[]Message) error {
	url := "https://api.anthropic.com/v1/messages"

	requestPayload := RequestPayload{
		Model:     model,
		MaxTokens: 256,
		Messages:  *messageHistory,
		Stream:    true,
		System:    systemPrompt,
	}

	reqBody, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal request payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "messages-2023-12-15")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	var content strings.Builder

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading response: %w", err)
		}

		lineStr := string(line)
		if strings.HasPrefix(lineStr, "data: ") {
			dataStr := strings.TrimPrefix(lineStr, "data: ")
			dataStr = strings.TrimSpace(dataStr)

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				fmt.Printf("Warning: Failed to unmarshal JSON: %v\n", err)
				continue
			}

			if eventType, ok := data["type"].(string); ok {
				switch eventType {
				case "content_block_delta":
					if delta, ok := data["delta"].(map[string]interface{}); ok {
						if text, ok := delta["text"].(string); ok {
							content.WriteString(text)
						}
					}
				case "message_stop":
					(*messageHistory) = append((*messageHistory), Message{Role: "assistant", Content: content.String()})
					colorizedText := colorizeText(content.String())
					fmt.Print(colorizedText)
					fmt.Print(" ")
					content.Reset()
				}
			}
		}
	}

	return nil
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	configPath := "config.yaml"
	config, err := LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		config = &Config{}
	}

	if config.Model == "" {
		config.Model = "claude-3-opus-20240229" // Default model if not specified in the config
	}

	if config.ShellPrompt == "" {
		config.ShellPrompt = "$> " // Default shell prompt if not specified in the config
	}

	if config.SystemPrompt == "" {
		config.SystemPrompt = "Assistant is in a CLI mood today." // Default system prompt if not specified in the config
	}

	if config.APIKey == "" {
		fmt.Print("Enter your API Key: ")
		scanner.Scan()
		apiKey := scanner.Text()
		if apiKey == "" {
			fmt.Println("Error: API Key cannot be empty")
			return
		}

		config.APIKey = apiKey
		if err := SaveConfig(configPath, config); err != nil {
			fmt.Printf("Failed to save API Key: %v\n", err)
			return
		}
	}

	var messageHistory []Message
	fmt.Println("Welcome to the a̷̡̧̭̹͉̤̘͍̒͌̆͛͘ͅn̵̛̻͂̓̀̓̇́̍͊̈́̂͒̀͠͝t̸̡̢̙͖̥͍̻͔͉̼̬̪̥̻́͊͂̂͊̍̈́̍̀̑̕͝͠ḩ̵̨̬́́̽͗̔̊́́͘͝r̶̡̛͈̳̭̯̯͕̱̐̒̆͗̋̇̈́͝͝o̷̧̬̤̮͉̬͍̖̍̍͊p̸̡͕̗͛̀̀i̵̧̡̛̞̳͉̞̤̼͋̔̍̿̈́̆͑̍̇͐͛́̒͆̕͠ͅç̴̢̢̥̮̜͉̹̜̣̱̱͓̙̘̮̤̅ quantum reality interface!")
	fmt.Println()
	fmt.Println("To get started type a command: help, ls, etc.")
	fmt.Println("Type 'exit' or 'quit' to end the session.")
	fmt.Println()

	for {
		fmt.Print(config.ShellPrompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Printf("Error reading input: %v\n", err)
			}
			break
		}

		prompt := scanner.Text()
		if prompt == "" {
			continue
		}

		if prompt == "exit" || prompt == "quit" {
			fmt.Println("Terminating s̷e̷s̵s̶i̴o̷n̸. Shutting d̸̖̍o̴̢͗w̵̺̋n̵̼͝.̶͙͑.̵̳́.̴̙̀....")
			break
		}

		prompt = "<cmd>" + prompt + "</cmd>"

		// Add the user's message to the message history
		messageHistory = append(messageHistory, Message{Role: "user", Content: prompt})

		err := StreamResponse(config.APIKey, config.Model, config.SystemPrompt, &messageHistory)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
	}
}
