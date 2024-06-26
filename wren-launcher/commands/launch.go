package commands

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Canner/WrenAI/wren-launcher/config"
	utils "github.com/Canner/WrenAI/wren-launcher/utils"
	"github.com/common-nighthawk/go-figure"
	"github.com/manifoldco/promptui"
	"github.com/pterm/pterm"
)

func prepareProjectDir() string {
	// create a project directory under ~/.wrenai
	homedir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	projectDir := path.Join(homedir, ".wrenai")

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		os.Mkdir(projectDir, 0755)
	}

	return projectDir
}

func evaluateTelemetryPreferences() (bool, error) {
	// let users know we're asking for telemetry consent
	disableTelemetry := config.IsTelemetryDisabled()
	if disableTelemetry {
		fmt.Println("You have disabled telemetry, Wren AI will not collect any data.")
		return false, nil
	}
	fmt.Println("Wren AI relies on anonymous usage statistics to continuously improve.")
	fmt.Println("You can opt out of sharing these statistics by manually adding flag `--disable-telemetry` as described at https://docs.getwren.ai/overview/telemetry")
	return true, nil
}

func askForLLMProvider() (string, error) {
	// let users know we're asking for a LLM provider
	fmt.Println("Please provide the LLM provider you want to use")
	fmt.Println("You can learn more about how to set up custom LLMs at https://docs.getwren.ai/installation/custom_llm#running-wren-ai-with-your-custom-llm-or-document-store")

	prompt := promptui.Select{
		Label: "Select an LLM provider",
		Items: []string{"OpenAI", "Custom"},
	}

	_, result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}

	return result, nil
}

func askForAPIKey() (string, error) {
	// let users know we're asking for an API key
	fmt.Println("Please provide your OpenAI API key")
	fmt.Println("Please use the key with full permission, more details at https://help.openai.com/en/articles/8867743-assign-api-key-permissions")

	validate := func(input string) error {
		// check if input is a valid API key
		// OpenAI API keys are starting with "sk-"
		if !strings.HasPrefix(input, "sk-") {
			return errors.New("invalid API key")
		}
		return nil
	}

	prompt := promptui.Prompt{
		Label:    "OpenAI API key",
		Validate: validate,
		Mask:     '*',
	}

	result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}

	return result, nil
}

func askForGenerationModel() (string, error) {
	// let users know we're asking for a generation model
	fmt.Println("Please provide the generation model you want to use")
	fmt.Println("You can learn more about OpenAI's generation models at https://platform.openai.com/docs/models/models")

	prompt := promptui.Select{
		Label: "Select an OpenAI's generation model",
		Items: []string{"gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo"},
	}

	_, result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", err
	}

	return result, nil
}

func isEnvFileValidForCustomLLM(projectDir string) error {
	// validate if .env.ai file exists in ~/.wrenai
	envFilePath := path.Join(projectDir, ".env.ai")

	if _, err := os.Stat(envFilePath); os.IsNotExist(err) {
		errMessage := fmt.Sprintf("Please create a .env.ai file in %s first, more details at https://docs.getwren.ai/installation/custom_llm#running-wren-ai-with-your-custom-llm-or-document-store", projectDir)
		return errors.New(errMessage)
	}

	return nil
}


func Launch() {
	// recover from panic
	defer func() {
		if r := recover(); r != nil {
			pterm.Error.Println("An error occurred:", r)
			fmt.Scanf("h")
		}
	}()

	// print Wren AI header
	fmt.Println(strings.Repeat("=", 55))
	myFigure := figure.NewFigure("WrenAI", "", true)
	myFigure.Print()
	fmt.Println(strings.Repeat("=", 55))

	// prepare a project directory
	pterm.Info.Println("Preparing project directory")
	projectDir := prepareProjectDir()

	// ask for LLM provider
	pterm.Print("\n")
	llmProvider, err := askForLLMProvider()
	openaiApiKey := ""
	openaiGenerationModel := ""
	if llmProvider == "OpenAI" {
		// ask for OpenAI API key
		pterm.Print("\n")
		openaiApiKey, _ = askForAPIKey()

		// ask for OpenAI generation model
		pterm.Print("\n")
		openaiGenerationModel, _ = askForGenerationModel()
	} else {
		// check if .env.ai file exists
		err = isEnvFileValidForCustomLLM(projectDir)
		if err != nil {
			panic(err)
		}
	}

	// ask for telemetry consent
	pterm.Print("\n")
	telemetryEnabled, err := evaluateTelemetryPreferences()

	if err != nil {
		pterm.Error.Println("Failed to get API key")
		panic(err)
	}

	// check if docker daemon is running, if not, open it and loop to check again
	pterm.Info.Println("Checking if Docker daemon is running")
	for {
		_, err = utils.CheckDockerDaemonRunning()
		if err == nil {
			break
		}

		pterm.Info.Println("Docker daemon is not running, opening Docker Desktop")
		err = utils.OpenDockerDaemon()
		if err != nil {
			panic(err)
		}

		time.Sleep(5 * time.Second)
	}

	// download docker-compose file and env file template for Wren AI
	pterm.Info.Println("Downloading docker-compose file and env file")
	// find an available port
	uiPort := utils.FindAvailablePort(3000)
	aiPort := utils.FindAvailablePort(5555)

	err = utils.PrepareDockerFiles(openaiApiKey, openaiGenerationModel, uiPort, aiPort, projectDir, telemetryEnabled)
	if err != nil {
		panic(err)
	}

	// launch Wren AI
	pterm.Info.Println("Launching Wren AI")
	const projectName string = "wrenai"
	err = utils.RunDockerCompose(projectName, projectDir, llmProvider)
	if err != nil {
		panic(err)
	}

	// wait for 10 seconds
	pterm.Info.Println("Wren AI is starting, please wait for a moment...")
	url := fmt.Sprintf("http://localhost:%d", uiPort)
	// wait until checking if CheckWrenAIStarted return without error
	// if timeout 2 minutes, panic
	timeoutTime := time.Now().Add(2 * time.Minute)
	for {
		if time.Now().After(timeoutTime) {
			panic("Timeout")
		}

		// check if ui is ready
		err := utils.CheckUIServiceStarted(url)
		if err == nil {
			pterm.Info.Println("UI Service is ready")
			break
		}
		time.Sleep(5 * time.Second)
	}

	for {
		if time.Now().After(timeoutTime) {
			panic("Timeout")
		}

		// check if ai service is ready
		err := utils.CheckAIServiceStarted(aiPort)
		if err == nil {
			pterm.Info.Println("AI Service is Ready")
			break
		}
		time.Sleep(5 * time.Second)
	}

	// open browser
	pterm.Info.Println("Opening browser")
	utils.Openbrowser(url)

	pterm.Info.Println("You can now safely close this terminal window")
	fmt.Scanf("h")
}
