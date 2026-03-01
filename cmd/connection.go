package cmd

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var connectionCmd = &cobra.Command{
	Use:   "connection",
	Short: "Connection commands",
}

var connectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connections",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, connectionListNative)
	},
}

var connectionAddCmd = &cobra.Command{
	Use:   "add <domain> <domain-token> <captaincore-token>",
	Short: "Add a connection",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <domain> <domain-token> <captaincore-token> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, connectionAddNative)
	},
}

// connectionListNative implements `captaincore connection list` natively in Go.
func connectionListNative(cmd *cobra.Command, args []string) {
	connections, err := models.GetAllConnections()
	if err != nil {
		fmt.Println("Error fetching connections:", err)
		return
	}

	if connections == nil {
		connections = []models.Connection{}
	}

	// Build output matching PHP format (only created_at, domain, token)
	type connectionOutput struct {
		CreatedAt string `json:"created_at"`
		Domain    string `json:"domain"`
		Token     string `json:"token"`
	}

	var output []connectionOutput
	for _, c := range connections {
		output = append(output, connectionOutput{
			CreatedAt: c.CreatedAt,
			Domain:    c.Domain,
			Token:     c.Token,
		})
	}

	if output == nil {
		output = []connectionOutput{}
	}

	result, _ := json.MarshalIndent(output, "", "    ")
	fmt.Println(string(result))
}

// connectionAddNative implements `captaincore connection add` natively in Go.
func connectionAddNative(cmd *cobra.Command, args []string) {
	domain := args[0]
	domainToken := args[1]
	captaincoreToken := args[2]

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Build HTTP client with optional TLS skip
	client := &http.Client{}
	if system.CaptainCoreDev != "" && system.CaptainCoreDev != "false" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// Post to remote CaptainCore API
	postBody, _ := json.Marshal(map[string]string{
		"token":             domainToken,
		"captaincore_token": captaincoreToken,
	})

	resp, err := client.Post("https://"+domain+"/wp-json/captaincore/v1/connect", "application/json", bytes.NewReader(postBody))
	if err != nil {
		fmt.Printf("Something went wrong: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	responseStr := string(body)
	fmt.Println(responseStr)

	if responseStr != "Sucessfully connected." {
		return
	}

	// Upsert connection to database
	existing, _ := models.GetConnectionByDomain(domain)
	if existing != nil {
		fmt.Println("Updated existing connection.")
	} else {
		fmt.Println("Adding new connection.")
	}

	models.UpsertConnection(models.Connection{
		CreatedAt: strconv.FormatInt(time.Now().Unix(), 10),
		Domain:    domain,
		Token:     captaincoreToken,
	})
}

func init() {
	rootCmd.AddCommand(connectionCmd)
	connectionCmd.AddCommand(connectionAddCmd)
	connectionCmd.AddCommand(connectionListCmd)
}
