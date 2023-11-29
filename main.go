package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"yaml"
)

type Config struct {
	Username      string   `yaml:"username"`
	ServerAddress string   `yaml:"serverAddress"`
	FilesToTrack  []string `yaml:"filesToTrack"`
	Seconds       int      `yaml:"seconds"`
}

type PastebinResponse struct {
	Data    PastebinData `json:"data"`
	Message string       `json:"message"`
	Success bool         `json:"success"`
	Code    int          `json:"code"`
}

type PastebinData struct {
	Content string `json:"content"`
	ID      string `json:"id"`
}

func main() {
	config, err := readConfig("config.yaml")
	if err != nil {
		fmt.Println("Error reading config file:", err)
		return
	}
	latestTemp := ""
	localEdit := 0
	lockedFiles := []string{}

	postClipboard(config.ServerAddress, "main.go 2006-01-02 15:04:05 -init\nmain.go 2006-01-02 15:04:05 -init\nmain.go 2006-01-02 15:04:05 -init\n")

	// loop starts
	for {
		fileTimes, err := GetFileTimes(config.FilesToTrack)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		currentTime := time.Now()
		localData := ""
		lockedFiles = []string{}
		remoteOld, err := getPastebinData(config.ServerAddress)
		if err != nil {
			fmt.Println("Get remote data error:", err)
			return
		}

		// update to the latest remote version
		if remoteOld.Data.Content != latestTemp {
			latestTemp = remoteOld.Data.Content
			fmt.Println("\nPolled at", time.Now().Format("2006-01-02 15:04:05"), "\nLatest modifications:")
			lockedFiles = timeAgo(latestTemp, currentTime, config.Username)
		}

		// check local files modified
		for _, fileTime := range fileTimes {
			localData += fileTime.Name() + " " + fileTime.ModTime().Format("2006-01-02 15:04:05")
			if isRecent(fileTime.ModTime(), currentTime, config.Seconds) {
				localData += " - " + config.Username
				localEdit = 2
			}
			localData += "\n"
		}

		// sync local modifications twice: 1.
		if localEdit > 0 {
			remoteNew := getLatest(localData, remoteOld.Data.Content)
			fmt.Println("\nSent at", time.Now().Format("2006-01-02 15:04:05"), "\nLatest modifications:")
			lockedFiles = timeAgo(remoteNew, currentTime, config.Username)
			postClipboard(config.ServerAddress, remoteNew)
			localEdit -= 1
		}

		// lock files
		for _, file := range lockedFiles {
			fmt.Println("Locking:", file)
		}

		time.Sleep(8 * time.Second)
	}
}

func readConfig(filename string) (Config, error) {
	var config Config

	// Read the YAML config file
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}

	// Unmarshal the YAML data into the Config struct
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func getPastebinData(id string) (PastebinResponse, error) {
	url := fmt.Sprintf("https://tools.xxxxxx.com/api/clipboard?id=%s", id)

	resp, err := http.Get(url)
	if err != nil {
		return PastebinResponse{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return PastebinResponse{}, err
	}

	var pastebinResp PastebinResponse
	err = json.Unmarshal(body, &pastebinResp)
	if err != nil {
		return PastebinResponse{}, err
	}

	return pastebinResp, nil
}

func postClipboard(id string, content string) error {
	targetUrl := "https://tools.xxxxxx.com/api/clipboard"
	// proxy, _ := url.Parse("http://127.0.0.1:8080")
	// transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	// client := &http.Client{Transport: transport}
	client := &http.Client{}

	// POST body
	body := []byte(`{"id":"` + id + `", "content":"` + strings.ReplaceAll(content, "\n", "\\n") + `"}`)

	req, err := http.NewRequest("POST", targetUrl, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authority", "tools.whatfa.com")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Dnt", "1")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func GetFileTimes(filesToTrack []string) ([]os.FileInfo, error) {
	var fileTimes []os.FileInfo

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && contains(filesToTrack, info.Name()) {
			fileTimes = append(fileTimes, info)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileTimes, nil
}

func isRecent(modTime time.Time, currentTime time.Time, seconds int) bool {
	threshold := time.Duration(seconds) * time.Second

	diff := currentTime.Sub(modTime)
	return diff < threshold
}

func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func timeAgo(input string, currentTime time.Time, username string) []string {
	lines := strings.Split(input, "\n")
	lockedFiles := []string{}

	_, offset := time.Now().Zone()
	for _, line := range lines {
		parts := strings.Split(line, " ")
		if len(parts) >= 3 {
			filename := parts[0]
			timestampStr := parts[1] + " " + parts[2]
			timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr)
			if err == nil {
				timeDiff := currentTime.Sub(timestamp)
				secondsDiff := int(timeDiff.Seconds()) + offset

				var timeAgo string
				if secondsDiff < 60 {
					timeAgo = fmt.Sprintf("%d seconds ago", secondsDiff)
				} else if secondsDiff < 7200 {
					minutesDiff := secondsDiff / 60
					timeAgo = fmt.Sprintf("%d minutes ago", minutesDiff)
				} else if secondsDiff < 86400 {
					hoursDiff := secondsDiff / 3600
					timeAgo = fmt.Sprintf("%d hours ago", hoursDiff)
				} else {
					daysDiff := secondsDiff / 86400
					timeAgo = fmt.Sprintf("%d days ago", daysDiff)
				}
				if len(parts) >= 5 {
					if username == parts[4] {
						timeAgo += " - \033[1;32m" + username + "\033[0m"
					} else {
						timeAgo += " - \033[1;31m" + parts[4] + "\033[0m"
						lockedFiles = append(lockedFiles, parts[0])
					}
				}
				fmt.Printf("%s    %s\n", filename, timeAgo)
				// if len(parts) >= 5 {
				// 	fmt.Printf(" - ", parts[4])
				// }
			}
		}
	}
	return lockedFiles
}

func getLatest(strA, strB string) string {
	linesA := strings.Split(strA, "\n")
	linesB := strings.Split(strB, "\n")

	latestTimes := make(map[string]time.Time)
	additionalText := make(map[string]string)

	// Compare lines and store the latest time and additional text for each filename
	for _, line := range linesA {
		parts := strings.Split(line, " ")
		if len(parts) >= 3 {
			filename := parts[0]
			timestampStr := parts[1] + " " + parts[2]
			timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr)
			if err == nil {
				if latestTime, ok := latestTimes[filename]; !ok || timestamp.After(latestTime) {
					latestTimes[filename] = timestamp
					additionalText[filename] = strings.Join(parts[3:], " ")
				}
			}
		}
	}

	for _, line := range linesB {
		parts := strings.Split(line, " ")
		if len(parts) >= 3 {
			filename := parts[0]
			timestampStr := parts[1] + " " + parts[2]
			timestamp, err := time.Parse("2006-01-02 15:04:05", timestampStr)
			if err == nil {
				if latestTime, ok := latestTimes[filename]; !ok || timestamp.After(latestTime) {
					latestTimes[filename] = timestamp
					additionalText[filename] = strings.Join(parts[3:], " ")
				}
			}
		}
	}

	// Sort the filenames alphabetically
	var sortedFilenames []string
	for filename := range latestTimes {
		sortedFilenames = append(sortedFilenames, filename)
	}
	sort.Strings(sortedFilenames)

	// Create the new string with lines that have the latest time and additional text
	var result strings.Builder
	for filename, latestTime := range latestTimes {
		line := fmt.Sprintf("%s %s", filename, latestTime.Format("2006-01-02 15:04:05"))
		if additionalText[filename] != "" {
			line += " " + additionalText[filename]
		}
		line += "\n"
		result.WriteString(line)
	}

	return result.String()
}
