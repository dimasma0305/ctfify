package gzcli

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gopkg.in/gomail.v2"

	"github.com/dimasma0305/ctfify/function/gzcli/gzapi"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/sethvargo/go-password/password"
)

// TeamCreds stores team credentials
type TeamCreds struct {
	Username           string `json:"username" yaml:"username"`
	Password           string `json:"password" yaml:"password"`
	Email              string `json:"email" yaml:"email"`
	TeamName           string `json:"team_name" yaml:"team_name"`
	IsEmailAlreadySent bool   `json:"is_email_already_sent" yaml:"is_email_already_sent"`
	IsTeamCreated      bool   `json:"is_team_created" yaml:"is_team_created"`
}

// CreteTeamAndUser creates a team and user, ensuring the team name is unique and within the specified length.
func (gz *GZ) CreteTeamAndUser(teamCreds *TeamCreds, config *Config, existingTeamNames, existingUserNames map[string]struct{}, credsCache []*TeamCreds, isSendEmail bool) (*TeamCreds, error) {
	var api *gzapi.GZAPI
	var currentCreds *TeamCreds
	password, err := password.Generate(24, 10, 10, false, false)
	if err != nil {
		return nil, err
	}

	// Generate a unique username
	username, err := generateUsername(teamCreds.Username, 15, existingUserNames)
	if err != nil {
		return nil, err
	}

	// Normalize the team name
	const maxTeamNameLength = 20
	teamName := normalizeTeamName(teamCreds.TeamName, maxTeamNameLength, existingTeamNames)

	alreadyLogin := false

	// If registration fails, attempt to initialize API with cached credentials
	for _, creds := range credsCache {
		if creds.Email == teamCreds.Email {
			currentCreds = creds
		}
	}

	if currentCreds != nil {
		api, err = gzapi.Init(config.Url, &gzapi.Creds{
			Username: currentCreds.Username,
			Password: currentCreds.Password,
		})
		if err == nil {
			alreadyLogin = true
		} else {
			log.Error("error login using: %v", currentCreds)
			return nil, err
		}

	} else {
		currentCreds = teamCreds
		currentCreds.Username = username
		currentCreds.Password = password
		currentCreds.TeamName = teamName
	}

	if !alreadyLogin {
		api, err = gzapi.Register(config.Url, &gzapi.RegisterForm{
			Email:    currentCreds.Email,
			Username: currentCreds.Username,
			Password: currentCreds.Password,
		})
		if err != nil {
			return nil, err
		}
	}

	// Create the team
	log.Info("Creating user %s with team %s", username, teamName)
	if !currentCreds.IsTeamCreated {
		err = api.CreateTeam(&gzapi.TeamForm{
			Bio:  "",
			Name: teamName,
		})
		if err != nil {
			log.ErrorH2("Team %s already exist", teamName)
		}
	} else {
		log.InfoH2("Team %s already created", teamName)
	}
	currentCreds.IsTeamCreated = true

	// Send credentials via email if enabled in the config
	if isSendEmail && !currentCreds.IsEmailAlreadySent {
		if err := sendEmail(teamCreds.Username, config.Url, currentCreds); err != nil {
			log.ErrorH2("Failed to send email to %s: %v", currentCreds.Email, err)
		}
		log.InfoH2("Successfully sending email to %s", currentCreds.Email)
		currentCreds.IsEmailAlreadySent = true
	} else {
		log.ErrorH2("Email to %s already sended before", currentCreds.Email)
	}

	return currentCreds, nil
}

func (gz *GZ) CreateTeams(csvURL string, isSendEmail bool) error {
	config, err := GetConfig(nil)
	if err != nil {
		return fmt.Errorf("failed to get config")
	}

	csvData, err := getData(csvURL)
	if err != nil {
		return fmt.Errorf("failed to get CSV data")
	}

	err = parseCSV(csvData, gz, config, isSendEmail)
	if err != nil {
		return err
	}

	return nil
}

func getData(source string) ([]byte, error) {
	var output []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, errors.New("failed to fetch data from URL")
		}

		output, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	} else if strings.HasPrefix(source, "file://") || !strings.Contains(source, "://") {
		filePath := strings.TrimPrefix(source, "file://")
		output, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("unsupported source prefix")
	}

	return output, nil
}

func getAppSettings() (map[string]interface{}, error) {
	filePath := "appsettings.json"
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return result, nil
}

// sendEmail sends the team credentials to the specified email address using gomail
func sendEmail(realName string, website string, creds *TeamCreds) error {
	appsettings, err := getAppSettings()
	if err != nil {
		return err
	}

	// Type assertion to check if EmailConfig exists and is of type map[string]interface{}
	emailConfig, ok := appsettings["EmailConfig"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to assert type map[string]interface{} for EmailConfig")
	}

	smtp, ok := emailConfig["Smtp"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("smtp is missing or not a dict")
	}

	// Extract the necessary fields from the emailConfig map
	smtpHost, ok := smtp["Host"].(string)
	if !ok {
		return fmt.Errorf("host is missing or not a string")
	}
	smtpPort, ok := smtp["Port"].(float64)
	if !ok {
		return fmt.Errorf("port is missing or not a number")
	}
	smtpUsername, ok := emailConfig["UserName"].(string)
	if !ok {
		return fmt.Errorf("smtpUsername is missing or not a string")
	}
	smtpPassword, ok := emailConfig["Password"].(string)
	if !ok {
		return fmt.Errorf("smtpPassword is missing or not a string")
	}

	m := gomail.NewMessage()
	m.SetHeader("From", smtpUsername)
	m.SetHeader("To", creds.Email)
	m.SetHeader("Subject", "Your Team Credentials")

	htmlBody := fmt.Sprintf(`
<html>
<head>
	<style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
        }
        .block {
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            border: 1px solid #eaeaea;
            border-radius: 5px;
            background-color: #f9f9f9;
        }
        h1 {
            color: #333;
        }
        .creds {
            margin-bottom: 20px;
        }
        .creds p {
            margin: 5px 0;
        }
        .cta {
            text-align: center;
            margin-top: 20px;
        }
        .cta a {
            display: inline-block;
            padding: 10px 20px;
            text-decoration: none;
            color: white;
            background-color: #007BFF;
            border-radius: 5px;
        }
        .cta a:hover {
            background-color: #0056b3;
        }
    </style>
</head>
<body>
	<div class="block">
	<h1>Hello %s,</h1>
	<div class="creds">
		<p>Here are your team credentials:</p>
		<p><strong>Username:</strong> %s</p>
		<p><strong>Password:</strong> %s</p>
		<p><strong>Team Name:</strong> %s</p>
		<p><strong>Website:</strong> <a href="%s">%s</a></p>
	</div>
	<p>After logging in with your credentials, you can copy your team invitation code from the /teams page, and then share it with your team members.</p>
	<p>Make sure to notify your team members to register first and then use the invitation code on the /team page.</p>
	<p>Once all your team members have joined, you can navigate to the /games page and request to join the game. The admin will verify your request, and you just need to wait for the CTF to start.</p>
	<div class="cta">
		<a href="%s">Go to Website</a>
	</div>
	</div>
</body>
</html>
`,
		realName, creds.Username, creds.Password, creds.TeamName, website, website, website,
	)

	// Set the email body as HTML
	m.SetBody("text/html", htmlBody)

	// Dial the SMTP server
	d := gomail.NewDialer(smtpHost, int(smtpPort), smtpUsername, smtpPassword)

	// Send the email
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	return nil
}

func parseCSV(data []byte, gz *GZ, config *Config, isSendEmail bool) error {
	reader := csv.NewReader(strings.NewReader(string(data)))

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV data: %v", err)
	}

	if len(records) == 0 {
		return errors.New("CSV is empty")
	}

	// Map to store the column indices for each header
	colIndices := make(map[string]int)

	// Assume the first row contains headers
	headers := records[0]
	for i, header := range headers {
		colIndices[header] = i
	}

	// Ensure that the required headers are present
	requiredHeaders := []string{"RealName", "Email", "TeamName"}
	for _, header := range requiredHeaders {
		if _, ok := colIndices[header]; !ok {
			return errors.New("missing required header: " + header)
		}
	}

	// Maps for storing unique usernames and existing team names
	uniqueUsernames := make(map[string]struct{})
	existingTeamNames := make(map[string]struct{})

	// Load existing team credentials from cache
	var teamsCredsCache []*TeamCreds
	if err := GetCache("teams_creds", &teamsCredsCache); err != nil {
		log.Error(err.Error())
	}

	// Create a map for quick lookup of existing credentials by email
	credsCacheMap := make(map[string]*TeamCreds)
	for _, creds := range teamsCredsCache {
		credsCacheMap[creds.Email] = creds
	}

	// List to hold the merged team credentials
	var teamsCreds []*TeamCreds

	for _, row := range records[1:] {
		realName := row[colIndices["RealName"]]
		email := row[colIndices["Email"]]
		teamName := row[colIndices["TeamName"]]

		// Create or update team and user based on the generated username
		creds, err := gz.CreteTeamAndUser(&TeamCreds{
			Username: realName,
			Email:    email,
			TeamName: teamName,
		}, config, existingTeamNames, uniqueUsernames, teamsCredsCache, isSendEmail)
		if err != nil {
			log.Error(err.Error())
			continue
		}

		if creds != nil {
			// Merge credentials if already exist in cache
			if existingCreds, exists := credsCacheMap[creds.Email]; exists {
				// Update the existing credentials with new information if necessary
				existingCreds.Username = creds.Username
				existingCreds.Password = creds.Password
				existingCreds.TeamName = creds.TeamName
			} else {
				// Add new credentials to the list
				teamsCreds = append(teamsCreds, creds)
			}
		}
	}

	// Add all credentials from the cache that were not updated
	for _, creds := range credsCacheMap {
		teamsCreds = append(teamsCreds, creds)
	}

	// Save the merged credentials to cache
	if err := setCache("teams_creds", teamsCreds); err != nil {
		return err
	}

	return nil
}
