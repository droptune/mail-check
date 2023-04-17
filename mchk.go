package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/smtp"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var Reset = "\033[0m"
var Red = "\033[31m"
var Green = "\033[32m"
var Yellow = "\033[33m"
var Blue = "\033[34m"
var Purple = "\033[35m"
var Cyan = "\033[36m"
var Gray = "\033[37m"
var White = "\033[97m"

func init() {
	fileInfo, _ := os.Stdout.Stat()
	if runtime.GOOS == "windows" || (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		Reset = ""
		Red = ""
		Green = ""
		Yellow = ""
		Blue = ""
		Purple = ""
		Cyan = ""
		Gray = ""
		White = ""
	}
}

const appName string = "mchk"

type AppConfig struct {
	Debug            bool         `yaml:"debug"`
	ContinueOnErrors bool         `yaml:"continue_on_errors"`
	Test             []TestConfig `yaml:"tests,flow"`
}

type TestConfig struct {
	Name           string `yaml:"name"`
	ShouldSend     bool   `yaml:"should_send"`
	SMTPServer     string `yaml:"smtp_server"`
	SMTPPort       string `yaml:"smtp_port"`
	Sender         string `yaml:"send_from"`
	Recipient      string `yaml:"send_to"`
	SenderLogin    string `yaml:"sender_login"`
	SenderPassword string `yaml:"sender_password"`
	WaitFor        int    `yaml:"wait_for"`
	ShouldReceive  bool   `yaml:"should_receive"`
	IMAPServer     string `yaml:"imap_server"`
	IMAPPort       string `yaml:"imap_port"`
	IMAPLogin      string `yaml:"imap_login"`
	IMAPPassword   string `yaml:"imap_password"`
	LeaveMessage   bool   `yaml:"leave_message"`
}

func (c *AppConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

func checkTestConfig(c *TestConfig) error {
	var configErrors []error
	haveError := false

	if c.SMTPServer == "" {
		haveError = true
		configErrors = append(configErrors, errors.New("Config file: SMTP server is not specified"))
	}
	if c.SMTPPort == "" {
		c.SMTPPort = "25"
	}
	if c.Sender == "" {
		haveError = true
		configErrors = append(configErrors, errors.New("Config file: SMTP sender is not specified"))
	}
	if c.Recipient == "" {
		haveError = true
		configErrors = append(configErrors, errors.New("Config file: SMTP recipient is not specified"))
	}
	if c.SenderLogin == "" {
		haveError = true
		configErrors = append(configErrors, errors.New("Config file: SMTP sender login is not specified"))
	}
	if c.IMAPServer == "" {
		haveError = true
		configErrors = append(configErrors, errors.New("Config file: IMAP server is not specified"))
	}
	if c.IMAPPort == "" {
		c.IMAPPort = "993"
	}
	if c.SenderPassword == "" {
		fmt.Printf("Enter SMTP password for " + c.SenderLogin + ": ")
		byteSenderPassword, err := term.ReadPassword(0)
		if err != nil {
			log.Fatal(err)
		}
		c.SenderPassword = string(byteSenderPassword)
	}
	if c.IMAPPassword == "" {
		fmt.Printf("Enter SMTP password for " + c.IMAPLogin + ": ")
		byteIMAPPassword, err := term.ReadPassword(0)
		if err != nil {
			log.Fatal(err)
		}
		c.IMAPPassword = string(byteIMAPPassword)
	}
	if haveError {
		err := errors.Join(configErrors...)
		return err
	}
	return nil
}

func getSubjectHash(smtpServer string) string {
	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic(err)
	}
	hash := md5.Sum([]byte(smtpServer + string(randomBytes)))
	return hex.EncodeToString(hash[:])
}

func sendMessage(config TestConfig, subject string) error {
	smtpServer := config.SMTPServer + ":" + config.SMTPPort
	fmt.Printf("Sending message through " + smtpServer + " from " + config.Sender + " to: " + config.Recipient + "... ")

	message := "From: " + config.Sender + "\n" +
		"Subject: " + subject + "\n" +
		"To: " + config.Recipient

	auth := smtp.PlainAuth("", config.SenderLogin, config.SenderPassword, config.SMTPServer)

	err := smtp.SendMail(smtpServer, auth, config.Sender, []string{config.Recipient}, []byte(message))
	if err != nil {
		return err
	} else {
		return nil
	}
}

func getMessageByIMAP(cfg TestConfig, s string) error {
	fmt.Printf("Connecting to IMAP server " + cfg.IMAPServer + ":" + cfg.IMAPPort + "... ")

	c, err := client.DialTLS(cfg.IMAPServer+":"+cfg.IMAPPort, nil)
	if err != nil {
		fmt.Printf(Red + "✖" + Reset + "\n")
		return err
	}
	fmt.Printf(Green + "✔" + Reset + "\n")
	defer c.Logout()

	if err := c.Login(cfg.IMAPLogin, cfg.IMAPPassword); err != nil {
		return err
	}

	_, err = c.Select("INBOX", false)

	if err != nil {
		log.Fatal(err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Header = map[string][]string{"Subject": []string{s}}
	ids, err := c.Search(criteria)
	if err != nil {
		return err
	}

	if len(ids) > 1 {
		return errors.New("Found more than one ID for subject " + s)
	}
	if len(ids) == 0 {
		return errors.New("Sent message not found on " + cfg.IMAPServer)
	}

	fmt.Println("Message successfully recieved " + Green + "✔" + Reset)

	if !cfg.LeaveMessage {
		// Mark found message as deleted
		seqset := new(imap.SeqSet)
		seqset.AddNum(ids...)
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		flags := []interface{}{imap.DeletedFlag}
		if err = c.Store(seqset, item, flags, nil); err != nil {
			return err
		}
		// Delete marked message
		if err := c.Expunge(nil); err != nil {
			return err
		}
	}
	return nil
}

func addHomeDir(dir string) string {
	if strings.HasPrefix(dir, "~/") {
		userdir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal("Can't get current home directory for config path. You may have to specify path explicitly with --config <path_to_config>")
		}
		return path.Join(path.Dir(userdir+"/"), path.Dir(strings.TrimPrefix(dir, "~/")))
	} else {
		return dir
	}
}

func createDefaultConfig(configPath string) error {
	defaultConfig := `---
	continue_on_errors: no
	tests:
	  - name: test example
	    should_send: yes
		smtp_server: smtp.example.com
		smtp_port: 25
		send_from: user@example.com
		send_to: user@example.com
		sender_login: user@example.com
		sender_password: password
		wait_for: 2
		should_receive: yes
		imap_server: imap.example.com
		imap_port: 993
		imap_login: user@example.com
		imap_password: password
		leave_message: no`

	configDirectory := filepath.Dir(configPath)

	_, err := os.Stat(configDirectory)
	if err != nil || os.IsNotExist(err) {
		err := os.MkdirAll(configDirectory, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	finalConfigPath := path.Join(configDirectory, filepath.Base(configPath))
	f, err := os.Create(finalConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	_, err = f.WriteString(defaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Example config file created at " + finalConfigPath)
	return nil
}

func waitFor(t int) {
	quantifier := "seconds"
	if t == 1 {
		quantifier = "second"
	}
	// Check if we are on real terminal
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		spinner := [8]string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
		i := 0
		ch := make(chan int)

		go func() {
			time.Sleep(time.Duration(t) * time.Second)
			ch <- 1
			close(ch)
		}()

	out:
		for {
			select {
			case <-ch:
				fmt.Printf("\r%d %s passed. %s✔%s             \n", t, quantifier, Green, Reset)
				break out
			default:
				i = i % len(spinner)
				time.Sleep(100 * time.Millisecond)
				fmt.Printf("\rWaiting for %d %s... %s ", t, quantifier, spinner[i])
				i += 1
			}
		}
	} else {
		fmt.Printf("Waiting for %d %s... ", t, quantifier)
		time.Sleep(time.Duration(t) * time.Second)
		fmt.Println(Green + "✔" + Reset)
	}
}

func main() {
	configArgPtr := flag.String("config", "", "path to configuration file")

	flag.Parse()

	var configPaths []string

	if *configArgPtr == "" {
		configPaths = append(configPaths, addHomeDir("~/.config/"+appName+"/"+appName+".yml"),
			addHomeDir("~/."+appName+".yml"),
			"./mchk.yml")
	} else {
		configPaths = append(configPaths, *configArgPtr)
	}

	currentConfig := ""

	// Search for config file
	for _, path := range configPaths {
		_, err := os.Stat(path)
		if err == nil {
			currentConfig = path
			break
		}
	}

	if currentConfig == "" {
		fmt.Println("No config file found.")
		defaultConfigPath := addHomeDir("~/.config/" + appName + "/" + appName + ".yml")
		_, err := os.Stat(defaultConfigPath)
		if err != nil || os.IsNotExist(err) {
			createDefaultConfig(defaultConfigPath)
		}
		os.Exit(1)
	}

	// Load configuration from file
	configData, err := ioutil.ReadFile(currentConfig)
	if err != nil {
		log.Fatal(err)
	}
	var config AppConfig
	if err := config.Parse(configData); err != nil {
		log.Fatal(err)
	}

	for _, test := range config.Test {
		fmt.Println("Running test \"" + test.Name + "\"...")

		err = checkTestConfig(&test)
		if err != nil {
			log.Fatal(err)
		}

		subject := getSubjectHash(test.SMTPServer)
		err := sendMessage(test, subject)
		if err != nil {
			if test.ShouldSend {
				if config.ContinueOnErrors {
					fmt.Println(err)
					continue
				} else {
					log.Fatal(err)
				}
			} else {
				fmt.Println(err)
				fmt.Println("Sending failed as expected " + Green + "✔" + Reset)
			}
		} else {
			fmt.Println(Green + "✔" + Reset)
		}

		waitFor(test.WaitFor)

		err = getMessageByIMAP(test, subject)
		if err != nil {
			if test.ShouldReceive {
				if config.ContinueOnErrors {
					fmt.Printf("%s %v✖\nTest '%s' failed%v\n", err, Red, test.Name, Reset)
					continue
				} else {
					fmt.Printf("%v%s ✖\nTest '%s' failed%v\n", Red, err, test.Name, Reset)
					os.Exit(1)
				}
			} else {
				fmt.Println(err)
				fmt.Println("Test message not found in INBOX as expected " + Green + "✔" + Reset)
			}
		}
	}
}
