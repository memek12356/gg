package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"context"
)

const (
	API_URL    = "https://api.xreverselabs.org/itsuka?apiKey=b2c442f36a149dde9a1542c2e4d1a3ea&ip=%s"
	RESET      = "\033[0m"
	RED        = "\033[31m"
	CYAN       = "\033[36m"
	GREEN      = "\033[32m"
	WHITE      = "\033[37m"
	RESULT_FILE = "Resultz.txt"
)

var (
	printMutex sync.Mutex
	fileMutex  sync.Mutex
)

type ApiResponse struct {
	Total  int      `json:"total"`
	Result []string `json:"result"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fmt.Print(WHITE + "Enter IP List Filename: " + RESET)
	var inputFile string
	fmt.Scanln(&inputFile)

	// Read base IPs from file
	baseIPs, err := readLines(inputFile)
	if err != nil {
		exitWithError("Error reading input file: " + err.Error())
	}

	// Get thread count
	fmt.Print(WHITE + "Enter Thread Count: " + RESET)
	var threadCount int
	fmt.Scanln(&threadCount)

	// Generate all target IPs
	ipList := generateAllIPs(baseIPs)

	// Create worker pool
	var wg sync.WaitGroup
	ipChan := make(chan string)

	for i := 0; i < threadCount; i++ {
		wg.Add(1)
		go worker(ipChan, &wg, ctx, cancel)
	}

	// Feed IPs to workers
	for _, ip := range ipList {
		ipChan <- ip
	}

	close(ipChan)
	wg.Wait()

	fmt.Println(GREEN + "Finished! Results saved in " + RESULT_FILE + RESET)
}

func readLines(filename string) ([]string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n"), nil
}

func generateAllIPs(baseIPs []string) []string {
	var ips []string
	for _, baseIP := range baseIPs {
		parsedIP := net.ParseIP(baseIP)
		if parsedIP == nil || parsedIP.To4() == nil {
			printColored(RED+"Invalid IP format: "+baseIP+"\n", RED)
			continue
		}

		ipParts := strings.Split(baseIP, ".")
		if len(ipParts) != 4 {
			printColored(RED+"Invalid IP format: "+baseIP+"\n", RED)
			continue
		}

		prefix := strings.Join(ipParts[:3], ".")
		for i := 1; i < 256; i++ {
			ips = append(ips, fmt.Sprintf("%s.%d", prefix, i))
		}
	}
	return ips
}

func worker(ipChan <-chan string, wg *sync.WaitGroup, ctx context.Context, cancel context.CancelFunc) {
    defer wg.Done()
    client := &http.Client{Timeout: 10 * time.Second}

    for {
        select {
        case ip, ok := <-ipChan:
            if !ok {
                return
            }
            if critical := reverseLookup(ip, client); critical {
                cancel()
                return
            }
        case <-ctx.Done():
            return
        }
    }
}

func reverseLookup(ip string, client *http.Client) bool {
	url := fmt.Sprintf(API_URL, ip)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		printStatus(ip, RED+"Error: "+err.Error())
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		printStatus(ip, RED+"Error: "+err.Error())
		return false
	}
	defer resp.Body.Close()

	// Check if the HTTP response is OK
	if resp.StatusCode != http.StatusOK {
		printStatus(ip, RED+"HTTP Error: "+resp.Status)
		return false
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		printStatus(ip, RED+"Error: "+err.Error())
		return false
	}

	var apiResponse ApiResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		printStatus(ip, RED+"Error parsing JSON: "+err.Error())
		return true // Critical error, stop processing
	}

	if apiResponse.Total == 0 {
		printStatus(ip, RED+"NO")
	} else {
		printStatus(ip, GREEN+strconv.Itoa(apiResponse.Total)+" DOMAINS")
		saveResults(apiResponse.Result)
	}

	return false
}

func printStatus(ip, message string) {
	printMutex.Lock()
	defer printMutex.Unlock()
	fmt.Printf("%sChecking ==> %s%s%s => %s%s\n", WHITE, CYAN, ip, WHITE, message, RESET)
}

func saveResults(domains []string) {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	file, err := os.OpenFile(RESULT_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("%sError saving results: %s%s\n", RED, err.Error(), RESET)
		return
	}
	defer file.Close()

	for _, domain := range domains {
		if _, err := file.WriteString(domain + "\n"); err != nil {
			fmt.Printf("%sError writing domain: %s%s\n", RED, err.Error(), RESET)
		}
	}
}

func printColored(message, color string) {
	printMutex.Lock()
	defer printMutex.Unlock()
	fmt.Print(color + message + RESET)
}

func exitWithError(message string) {
	printColored(message+"\n", RED)
	os.Exit(1)
}
