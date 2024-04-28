package main

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "regexp"
    "net/http"
    "os"
    "strings"

    "github.com/charmbracelet/ssh"
    "github.com/charmbracelet/wish"
    "github.com/charmbracelet/wish/logging"
    "gopkg.in/yaml.v2"
)
// Config stores the API key and other configuration details
type Config struct {
    APIKey       string `yaml:"api_key"`
    Model        string `yaml:"model"`
    SystemPrompt string `yaml:"system_prompt"`
    ShellPrompt string `yaml:"shell_prompt"`
}

// RequestPayload defines the structure of the JSON payload for the request
type RequestPayload struct {
    Model     string    `json:"model"`
    MaxTokens int       `json:"max_tokens"`
    Messages  []Message `json:"messages"`
    Stream    bool      `json:"stream"`
    System    string      `json:"system"`
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
    config := &Config{}
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    decoder := yaml.NewDecoder(file)
    if err := decoder.Decode(config); err != nil {
        return nil, err
    }

    return config, nil
}

// SaveConfig saves the API key to a YAML file
func SaveConfig(path string, config *Config) error {
    file, err := os.Create(path)
    if err != nil {
        return err
    }
    defer file.Close()

    encoder := yaml.NewEncoder(file)
    if err := encoder.Encode(config); err != nil {
        return err
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
func StreamResponse(apiKey string, model string, systemPrompt string, messageHistory *[]Message, s ssh.Session) error {
    url := "https://api.anthropic.com/v1/messages"

    requestPayload := RequestPayload{
        Model:     model,
        MaxTokens: 256,
        Messages:  *messageHistory,
        Stream:    true,
        System:    systemPrompt,
    }

    reqBody, _ := json.Marshal(requestPayload)

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
    if err != nil {
        return err
    }

    req.Header.Set("x-api-key", apiKey)
    req.Header.Set("anthropic-version", "2023-06-01")
    req.Header.Set("anthropic-beta", "messages-2023-12-15")
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    reader := bufio.NewReader(resp.Body)
    var content strings.Builder

    for {
        line, err := reader.ReadBytes('\n')
        if err != nil {
            if err == io.EOF {
                break
            }
            return err
        }

        lineStr := string(line)
        if strings.HasPrefix(lineStr, "data: ") {
            dataStr := strings.TrimPrefix(lineStr, "data: ")
            dataStr = strings.TrimSpace(dataStr)

            var data map[string]interface{}
            if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
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
                    fmt.Fprint(s, colorizedText)
                    fmt.Fprint(s, " ")
                    content.Reset()
                }
            }
        }
    }

    return nil
}

func sessionHandler(s ssh.Session) {
    // Load the configuration
    configPath := "config.yaml"
    config, err := LoadConfig(configPath)
    if err != nil {
        fmt.Fprintln(s, "Failed to load configuration:", err)
        return
    }

    // Set default values if not specified in the config
    if config.Model == "" {
        config.Model = "claude-3-opus-20240229"
    }
    if config.ShellPrompt == "" {
        config.ShellPrompt = "$> "
    }
    if config.SystemPrompt == "" {
        config.SystemPrompt = "Assistant is in a CLI mood today."
    }

    // Initialize the message history
    var messageHistory []Message

    // Print the welcome message and prompt
    fmt.Fprintln(s, "Welcome to the a̷̡̧̭̹͉̤̘͍̒͌̆͛͘ͅn̵̛̻͂̓̀̓̇́̍͊̈́̂͒̀͠͝t̸̡̢̙͖̥͍̻͔͉̼̬̪̥̻́͊͂̂͊̍̈́̍̀̑̕͝͠ḩ̵̨̬́́̽͗̔̊́́͘͝r̶̡̛͈̳̭̯̯͕̱̐̒̆͗̋̇̈́͝͝o̷̧̬̤̮͉̬͍̖̍̍͊p̸̡͕̗͛̀̀i̵̧̡̛̞̳͉̞̤̼͋̔̍̿̈́̆͑̍̇͐͛́̒͆̕͠ͅç̴̢̢̥̮̜͉̹̜̣̱̱͓̙̘̮̤̅ quantum reality interface!")
    fmt.Fprintln(s)
    fmt.Fprintln(s)
    fmt.Fprintln(s, "To get started type a command: help, ls, etc.")
    fmt.Fprintln(s, "Type 'exit' or 'quit' to end the session.")
    fmt.Fprintln(s)
    fmt.Fprint(s, config.ShellPrompt)

    // Start the interactive session
    scanner := bufio.NewScanner(s)
    for {
        // Read the user input
        scanner.Scan()
        prompt := scanner.Text()

        // Handle the exit command
        if prompt == "exit" || prompt == "quit" {
            fmt.Fprintln(s, "Terminating s̷e̷s̵s̶i̴o̷n̸. Shutting d̸̖̍o̴̢͗w̵̺̋n̵̼͝.̶͙͑.̵̳́.̴̙̀....")
            break
        }

        prompt = "<cmd>" + prompt + "</cmd>"

        // Add the user's message to the message history
        messageHistory = append(messageHistory, Message{Role: "user", Content: prompt})

        // Stream the response
        err := StreamResponse(config.APIKey, config.Model, config.SystemPrompt, &messageHistory, s)
        if err != nil {
            fmt.Fprintln(s, "Error:", err)
            continue
        }

        // Print the shell prompt
        fmt.Fprint(s, config.ShellPrompt)
    }
}

func main() {
    srv, err := wish.NewServer(
        wish.WithAddress("localhost:22020"),
        wish.WithHostKeyPath(".ssh/term_info_ed25519"),
        wish.WithMiddleware(
            func(next ssh.Handler) ssh.Handler {
                return func(s ssh.Session) {
                    sessionHandler(s)
                    next(s)
                }
            },
            logging.Middleware(),
        ),
    )
    if err != nil {
        log.Fatalln(err)
    }

    log.Println("Starting SSH server on localhost:22")
    if err := srv.ListenAndServe(); err != nil {
        log.Fatalln(err)
    }
}
